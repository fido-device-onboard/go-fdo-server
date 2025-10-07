# Using the go-fdo-server containers (Compose)

This document shows how to run the three FDO roles—**Rendezvous**, **Manufacturing**, and **Owner**—using the Compose files in this directory.

The instructions work with **Docker Compose** (`docker compose …`) and **Podman Compose** (`podman-compose …`).

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Clone repository and create data directories](#clone-repository-and-create-data-directories)
- [Set container_user environment variable](#set-container_user-environment-variable)
- [Generate certificates and keys](#generate-certificates-and-keys)
- [Important Note for SELinux Systems](#important-note-for-selinux-systems)
- [Start the services](#start-the-services)
- [Health checks](#health-checks)
- [End-to-end onboarding test (DI → voucher → TO0 → TO2)](#end-to-end-onboarding-test-di--voucher--to0--to2)
- [Device resale test (Owner → New Owner)](#device-resale-test-owner--new-owner)
- [Stop and remove services](#stop-and-remove-services)
- [Using FSIM (FIDO Service Info Modules)](#using-fsim-fido-service-info-modules)
- [API Format Notes](#api-format-notes)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- Podman 4+ (and `podman-compose`) **or** Docker 24+ (Compose v2)
- `openssl` for generating keys and certificates
- `curl` for API calls and health checks
- `go-fdo-client` - Install via package manager(Fedora) or from [go-fdo-client](https://github.com/fido-device-onboard/go-fdo-client)

On Fedora:
```bash
sudo dnf install podman-compose openssl curl go-fdo-client
```

## Clone repository and create data directories

Clone the repository:

```bash
git clone https://github.com/fido-device-onboard/go-fdo-server.git
cd go-fdo-server/deployments/compose/
```

Create folders for databases, certificates, and owner vouchers:

```bash
mkdir -p /tmp/go-fdo/{certs,ov}
```

**Note**: Database files will be created automatically in `/tmp/go-fdo/` when the containers start.

## Set container_user environment variable

The compose files use `${container_user}` to set the user ID for running containers.

**For rootful Docker** (default on Ubuntu):
```bash
export container_user=$(id -u):$(id -g)
```

**For rootless Podman** (default on Fedora, RHEL):

Rootless Podman uses user namespace mapping where container UIDs are mapped to host UIDs. The `${container_user}` variable needs to be set to `0:0` so that container UID 0 maps to your host UID:

```bash
export container_user="0:0"
```

**Why this is necessary**: When using rootless Podman, setting `user: ${container_user}` to your actual host UID (e.g., `1000:1000`) causes permission issues because:
- Container UID 1000 maps to host UID 100999 (due to user namespace mapping)
- Files in `/tmp/go-fdo/` are owned by your host UID (e.g., 1000)
- The container process running as host UID 100999 cannot access files owned by host UID 1000

By setting `container_user="0:0"`:
- Container UID 0 maps to your host UID through the user namespace
- The container can access files you own
- This is secure because container root is not actual root on the host

Add this to your `.bashrc` or `.zshrc` to persist across sessions:

```bash
# For rootful Docker
echo 'export container_user=$(id -u):$(id -g)' >> ~/.bashrc

# For rootless Podman
echo 'export container_user="0:0"' >> ~/.bashrc
```

## Generate certificates and keys

Generate keys and certificates under `/tmp/go-fdo/certs`. Keys must be in DER format; certificates in PEM.

```bash
# Manufacturer (EC P-256 key + self-signed cert)
openssl ecparam -name prime256v1 -genkey -out /tmp/go-fdo/certs/manufacturer.key -outform der
openssl req -x509 -key /tmp/go-fdo/certs/manufacturer.key -keyform der \
  -out /tmp/go-fdo/certs/manufacturer.crt -days 365 -subj "/C=US/O=Example/CN=Manufacturer"

# Device CA (EC P-256 key + self-signed cert)
openssl ecparam -name prime256v1 -genkey -out /tmp/go-fdo/certs/device_ca.key -outform der
openssl req -x509 -key /tmp/go-fdo/certs/device_ca.key -keyform der \
  -out /tmp/go-fdo/certs/device_ca.crt -days 365 -subj "/C=US/O=Example/CN=Device CA"

# Owner (EC P-256 key + self-signed cert)
openssl ecparam -name prime256v1 -genkey -out /tmp/go-fdo/certs/owner.key -outform der
openssl req -x509 -key /tmp/go-fdo/certs/owner.key -keyform der \
  -out /tmp/go-fdo/certs/owner.crt -days 365 -subj "/C=US/O=Example/CN=Owner"

# (Optional) New Owner for resale scenarios
openssl ecparam -name prime256v1 -genkey -out /tmp/go-fdo/certs/new_owner.key -outform der
openssl req -x509 -key /tmp/go-fdo/certs/new_owner.key -keyform der \
  -out /tmp/go-fdo/certs/new_owner.crt -days 365 -subj "/C=US/O=Example/CN=New Owner"
```

**Set proper permissions for container access**:

```bash
# Make files readable and writable by your user
chmod -R u+rwX /tmp/go-fdo
```

**Note**: The compose files use `/tmp:/tmp:z` by default. On SELinux systems, this causes an error and requires a workaround (see next section).

## Important Note for SELinux Systems

**If you are using rootless Podman on a SELinux-enabled system (Fedora, RHEL)**, the default `/tmp:/tmp:z` volume mount will fail with:

```
Error: SELinux relabeling of /tmp is not allowed
```

**Required workaround** - Remove the `:z` flag from volume mounts and manually set SELinux context:

1. Edit the compose files to remove the `:z` flag:
   ```bash
   sed -i 's|/tmp:/tmp:z|/tmp:/tmp|g' server/*.yaml
   ```

2. After creating directories and certificates, set the SELinux context:
   ```bash
   sudo chcon -R -t container_file_t /tmp/go-fdo
   ```

3. Verify the context was set correctly:
   ```bash
   ls -laZ /tmp/go-fdo/
   ```

   You should see `container_file_t` in the output for all files and directories.

**Why this is necessary**:
- SELinux blocks relabeling of system directories like `/tmp` for security reasons
- SQLite requires a writable `/tmp` directory for temporary files (journals, WAL)
- Without the `:z` flag, we must manually set the SELinux context to `container_file_t`
- This allows containers to access `/tmp/go-fdo/` while maintaining security

## Start the services

### Start the three FDO services

From the `deployments/compose/` directory:

**Podman**:
```bash
podman-compose -f server/fdo-onboarding-servers.yaml up -d
```

**Docker**:
```bash
docker compose -f server/fdo-onboarding-servers.yaml up -d
```

This starts three services:
- `manufacturer` - Manufacturing server (port 8038)
- `rendezvous` - Rendezvous server (port 8041)
- `owner` - Owner server (port 8043)

Verify the services are running:

```bash
podman ps 
CONTAINER ID  IMAGE                          COMMAND               CREATED         STATUS                    PORTS                   NAMES
dfa7c8d828bf  localhost/manufacturer:latest  --db=/tmp/go-fdo/...  25 seconds ago  Up 25 seconds (starting)  0.0.0.0:8038->8038/tcp  manufacturer
177aebe1a14d  localhost/rendezvous:latest    --db=/tmp/go-fdo/...  25 seconds ago  Up 25 seconds (starting)  0.0.0.0:8041->8041/tcp  rendezvous
6f4a66db5c64  localhost/owner:latest         --db=/tmp/go-fdo/...  25 seconds ago  Up 25 seconds (starting)  0.0.0.0:8043->8043/tcp  owner
```

View the logs for the services:

```bash
# view logs for all fdo services
podman-compose -f server/fdo-onboarding-servers.yaml logs

# view logs for a single fdo service
podman-compose -f server/fdo-onboarding-servers.yaml logs manufacturer
```

## Health checks

Verify all services are running:

```bash
curl -fsS http://127.0.0.1:8038/health  # Manufacturer
curl -fsS http://127.0.0.1:8041/health  # Rendezvous
curl -fsS http://127.0.0.1:8043/health  # Owner
```

Each should return: `{"version":"1.1","status":"OK"}`

## End-to-end onboarding test (DI → voucher → TO0 → TO2)

This demonstrates the complete FDO flow using containerized services.

### 1. Configure RV info and Owner redirect

**Store Rendezvous info** (JSON format) on Manufacturing server:

```bash
curl -fsS -X POST 'http://127.0.0.1:8038/api/v1/rvinfo' \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"dns":"rendezvous","device_port":"8041","owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]'

# Verify
curl -fsS 'http://127.0.0.1:8038/api/v1/rvinfo'
```

**To enable RVBypass** (device skips TO1, goes directly to owner):

```bash
curl -fsS -X POST 'http://127.0.0.1:8038/api/v1/rvinfo' \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"dns":"owner","device_port":"8043","rv_bypass":true,"owner_port":"8043","protocol":"http","ip":"127.0.0.1"}]'
```

**Set Owner redirect** (JSON format) on Owner server:

```bash
curl -fsS -X POST 'http://127.0.0.1:8043/api/v1/owner/redirect' \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"dns":"owner","port":"8043","protocol":"http","ip":"127.0.0.1"}]'

# Verify
curl -fsS 'http://127.0.0.1:8043/api/v1/owner/redirect'
```

**JSON field reference**:
- RV info: `dns`, `ip`, `device_port`, `owner_port`, `protocol` (http/https/tcp/tls), `rv_bypass` (boolean), `medium`, `delay_seconds`, etc.
- Owner redirect: `dns`, `ip`, `port`, `protocol` (tcp/tls/http/https/coap/coaps)

### 2. Device Initialization (DI)

Initialize a device at the Manufacturing server:

```bash
cd /tmp/go-fdo
go-fdo-client device-init 'http://127.0.0.1:8038' \
  --device-info localtest \
  --key ec256 \
  --blob cred.bin
```

### 3. Extract device GUID

```bash
GUID=$(go-fdo-client print --blob cred.bin | grep -oE '[0-9a-fA-F]{32}' | head -n1)
echo "Device GUID: ${GUID}"
```

### 4. Transfer voucher to Owner

Download voucher from Manufacturing (returns PEM format):

```bash
curl -fsS "http://127.0.0.1:8038/api/v1/vouchers/${GUID}" > /tmp/go-fdo/ov/ownervoucher.pem
```

Upload voucher to Owner:

```bash
curl -fsS -X POST 'http://127.0.0.1:8043/api/v1/owner/vouchers' \
  --data-binary @/tmp/go-fdo/ov/ownervoucher.pem
```

### 5. Trigger TO0 (Owner registers with Rendezvous)

```bash
curl -s "http://127.0.0.1:8043/api/v1/to0/${GUID}"
```

### 6. Onboard device (TO1 + TO2)

```bash
go-fdo-client onboard \
  --key ec256 \
  --kex ECDH256 \
  --debug \
  --blob cred.bin
```

**Success**: Output should end with `FIDO Device Onboard Complete`

## Device resale test (Owner → New Owner)

This demonstrates transferring device ownership from the original owner to a new owner, simulating a device resale scenario.

**Important**: The resale scenario requires a fresh start with the resale server configuration, which includes the `--reuse-credentials` flag on the owner services. This flag allows vouchers to be resold after the initial onboarding.

### 1. Clean up and restart from the beginning

Stop the basic onboarding services, clean up databases, and start fresh with the resale configuration:

**Podman**:
```bash
podman-compose -f server/fdo-onboarding-servers.yaml down -v
rm -f /tmp/go-fdo/*.db
podman-compose -f server/fdo-resale-servers.yaml up -d
```

**Docker**:
```bash
docker compose -f server/fdo-onboarding-servers.yaml down -v
rm -f /tmp/go-fdo/*.db
docker compose -f server/fdo-resale-servers.yaml up -d
```

The `fdo-resale-servers.yaml` includes all three onboarding servers plus:
- `new_owner` - Additional owner service for resale (port 8045)

**Note**: Removing the databases (`rm -f /tmp/go-fdo/*.db`) ensures a clean start.

**Required workaround for resale** - The compose files need to be updated to add the `--reuse-credentials` flag to both owner services:

```bash
sed -i '/^      - owner:8043$/a\      - --reuse-credentials' server/fdo-onboarding-servers.yaml
sed -i '/^      - new_owner:8045$/a\      - --reuse-credentials' server/fdo-resale-servers.yaml
```

**Why this is necessary**:
- The Credential Reuse Protocol keeps the device GUID unchanged across onboardings
- Without this flag, the device gets a new GUID after each onboarding, breaking the resale voucher chain
- Both the first owner and new owner need this flag to allow multiple onboardings with the same GUID

After applying the workaround, restart the services:

**Podman**:
```bash
podman-compose -f server/fdo-resale-servers.yaml down
podman-compose -f server/fdo-resale-servers.yaml up -d
```

**Docker**:
```bash
docker compose -f server/fdo-resale-servers.yaml down
docker compose -f server/fdo-resale-servers.yaml up -d
```

Verify all services are running:

```bash
curl -fsS http://127.0.0.1:8038/health  # Manufacturer
curl -fsS http://127.0.0.1:8041/health  # Rendezvous
curl -fsS http://127.0.0.1:8043/health  # Owner
curl -fsS http://127.0.0.1:8045/health  # New Owner
```

### 2. Repeat the initial onboarding flow

Since we started fresh, repeat steps from the [End-to-end onboarding test](#end-to-end-onboarding-test-di--voucher--to0--to2):

1. Configure RV info on the Manufacturing server
2. Configure Owner redirect on the Owner server
3. Run device initialization (DI)
4. Extract device GUID
5. Transfer voucher from Manufacturing to Owner
6. Trigger TO0
7. Run device onboarding (TO2)

Once the initial onboarding is complete, proceed with the resale steps below.

### 3. Configure new_owner redirect

Set the owner redirect for new_owner:

```bash
curl -fsS -X POST 'http://127.0.0.1:8045/api/v1/owner/redirect' \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"dns":"new_owner","port":"8045","protocol":"http","ip":"127.0.0.1"}]'

# Verify
curl -fsS 'http://127.0.0.1:8045/api/v1/owner/redirect'
```

### 4. Transfer voucher from owner to new_owner

Create a resale voucher from the original owner by extending the ownership chain to the new owner's public key.

First, extract the new owner's public key from the certificate:

```bash
openssl x509 -in /tmp/go-fdo/certs/new_owner.crt -pubkey -noout > /tmp/go-fdo/certs/new_owner_pubkey.pem
```

Then create the resale voucher:

```bash
curl -fsS -X POST "http://127.0.0.1:8043/api/v1/owner/resell/${GUID}" \
  --data-binary @/tmp/go-fdo/certs/new_owner_pubkey.pem > /tmp/go-fdo/ov/resale-voucher.pem
```

Upload to new_owner:

```bash
curl -fsS -X POST 'http://127.0.0.1:8045/api/v1/owner/vouchers' \
  --data-binary @/tmp/go-fdo/ov/resale-voucher.pem
```

### 5. Trigger TO0 with new_owner

Register the new_owner with the Rendezvous server:

```bash
curl -s "http://127.0.0.1:8045/api/v1/to0/${GUID}"
```

### 6. Re-onboard device to new_owner

Onboard the device again - it will now connect to new_owner:

```bash
go-fdo-client onboard \
  --key ec256 \
  --kex ECDH256 \
  --debug \
  --blob cred.bin
```

**Success**: Device now onboards to new_owner at port 8045. Output should end with `FIDO Device Onboard Complete`.

**What happened**: The device's ownership was transferred from the original owner (port 8043) to the new owner (port 8045) through voucher transfer. This simulates a real-world device resale where the manufacturer's voucher is passed from one owner to another.

## Stop and remove services

Stop and remove all services:

**Podman**:
```bash
podman-compose -f server/fdo-onboarding-servers.yaml down -v
# or for resale setup:
podman-compose -f server/fdo-resale-servers.yaml down -v
```

**Docker**:
```bash
docker compose -f server/fdo-onboarding-servers.yaml down -v
# or for resale setup:
docker compose -f server/fdo-resale-servers.yaml down -v
```

To stop specific services only:
```bash
podman-compose -f server/fdo-onboarding-servers.yaml stop manufacturer rendezvous owner
# or
docker compose -f server/fdo-onboarding-servers.yaml stop manufacturer rendezvous owner
```

## Using FSIM (FIDO Service Info Modules)

FSIM modules enable file transfers and command execution during onboarding.

**Available FSIM compose files**:
- `fsim-fdo-download-servers.yaml` + `fsim-fdo-download-override.yaml` - Download files to device
- `fsim-fdo-upload-servers.yaml` + `fsim-fdo-upload-override.yaml` - Upload files from device
- `fsim-fdo-wget-servers.yaml` + `fsim-fdo-wget-override.yaml` - Device downloads from URLs

Example using FSIM download:

```bash
podman-compose -f server/fsim-fdo-download-servers.yaml \
  -f server/fsim-fdo-download-override.yaml up -d
```

Additional documentation on using FSIMs can be found [here](https://github.com/fido-device-onboard/go-fdo-server/blob/main/FSIM_USAGE.md).

## API Format Notes

The REST APIs accept **human-readable JSON** which is internally parsed and converted to CBOR (Concise Binary Object Representation) for the FDO protocol. 

### RV Info JSON Format

The Manufacturing server's `/api/v1/rvinfo` endpoint accepts JSON arrays with objects containing:

**Required fields** (at least one of):
- `dns` - DNS hostname (string)
- `ip` - IP address (string)

**Optional fields**:
- `device_port` - Port for device connection (string or number)
- `owner_port` - Port for owner connection (string or number)
- `protocol` - Protocol type: `"http"`, `"https"`, `"tcp"`, `"tls"`, `"rest"`, `"coap"`, `"coap+tcp"`
- `rv_bypass` - Boolean, if true device skips TO1 and connects directly to owner
- `medium` - `"eth_all"`, `"wifi_all"`, or numeric value
- `wifi_ssid`, `wifi_pw` - WiFi credentials
- `dev_only`, `owner_only` - Boolean flags
- `delay_seconds` - Delay in seconds (number)
- `sv_cert_hash`, `cl_cert_hash` - Certificate hashes (hex strings)
- `user_input`, `ext_rv` - Additional string fields

**Example**:
```json
[{
  "dns": "manufacturer.example.com",
  "ip": "192.168.1.100",
  "device_port": "8041",
  "owner_port": "8041",
  "protocol": "http",
  "rv_bypass": false
}]
```

### Owner Redirect JSON Format

The Owner server's `/api/v1/owner/redirect` endpoint accepts JSON arrays with objects containing:

**Required fields** (at least one of):
- `dns` - DNS hostname (string)
- `ip` - IP address (string)

**Optional fields**:
- `port` - Port number (string or number)
- `protocol` - Protocol: `"tcp"`, `"tls"`, `"http"`, `"https"`, `"coap"`, `"coaps"`

**Example**:
```json
[{
  "dns": "owner.example.com",
  "ip": "192.168.1.200",
  "port": "8043",
  "protocol": "http"
}]
```

The server validates the JSON and converts it to the CBOR format required by the FDO protocol specification.

## Troubleshooting

### container_user not set

If you get errors about `container_user` variable not set, set it according to your container runtime:

```bash
# For rootful Docker
export container_user=$(id -u):$(id -g)

# For rootless Podman
export container_user="0:0"
```

### Permission errors / SQLite "unable to open database file"

If containers fail with SQLite errors like "unable to open database file", this is likely a user namespace mapping issue with rootless Podman.

**Solution**: Set `container_user="0:0"` instead of your actual UID:

```bash
export container_user="0:0"
podman-compose -f server/fdo-onboarding-servers.yaml restart
```

**Why**: Rootless Podman maps container UIDs to host UIDs. Container UID 0 maps to your host UID, allowing access to files you own. Setting `container_user` to your actual UID (e.g., `1000:1000`) causes the container process to run as a different host UID that cannot access your files.

Ensure the files are owned by your user and writable:

```bash
chmod -R u+rwX /tmp/go-fdo
```

### SELinux permission denied errors

If you see "permission denied" or "unable to open database file" errors on SELinux-enabled systems (Fedora, RHEL, CentOS), the SELinux context is likely incorrect.

**Required for all SELinux systems**: You must manually set the SELinux context on `/tmp/go-fdo/` after creating directories and before starting containers:

```bash
# Check SELinux status
getenforce

# Set the correct SELinux context
sudo chcon -R -t container_file_t /tmp/go-fdo

# Verify the SELinux context
ls -laZ /tmp/go-fdo/
```

The context should show `container_file_t` for all files and directories. If the containers fail to start or SQLite reports database errors, verify this context is set correctly.

### Database files not found

The database files are created automatically in `/tmp/go-fdo/` when containers start:
- `/tmp/go-fdo/manufacturer.db`
- `/tmp/go-fdo/rendezvous.db`
- `/tmp/go-fdo/owner.db`
- `/tmp/go-fdo/new_owner.db` (if using resale setup)

Ensure `/tmp/go-fdo/` exists and is writable by your user.

### Port conflicts

If ports 8038, 8041, 8043, or 8045 are already in use, modify the port mappings in the compose files:

```yaml
ports:
  - "9038:8038"  # Map host port 9038 to container port 8038
```

### Invalid RV info or Owner redirect JSON

If you get errors when setting RV info or owner redirect, check:

1. **JSON syntax** - Must be valid JSON array of objects
2. **Required fields** - At least one of `dns` or `ip` must be present
3. **Port values** - Must be valid integers between 1-65535
4. **Protocol values** - Must be one of the supported protocol strings (see API Format Notes)
5. **IP addresses** - Must be valid IPv4 or IPv6 addresses

Example error: `"error parsing rvinfo data: invalid ip"` means the IP address format is incorrect.

The server performs validation before storing the data. Use the GET endpoints to verify your configuration was accepted:
```bash
curl -fsS 'http://127.0.0.1:8038/api/v1/rvinfo'
curl -fsS 'http://127.0.0.1:8043/api/v1/owner/redirect'
```
