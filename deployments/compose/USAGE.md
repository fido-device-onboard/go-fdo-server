# Using the go-fdo-server containers (Compose)

This document will show how to run the three FDO roles—**Rendezvous**, **Manufacturing**, and **Owner**—using the Compose files in this directory.

The instructions work with **Docker Compose** (`docker-compose …`) and **Podman Compose** (`podman-compose …`).

---

## Prerequisites

- Podman 4+ (and `podman-compose`) **or** Docker 24+ (Compose v2).
- `openssl` for generating keys.
- `curl` for retrieving files and health checks.
- Other common packages that may not be installed on minimal systems `make` and `git`

## Clone your fork and create data directories

Clone your fork (replace $USER with your Github username):

```bash
USER=(your github username here)
git clone https://github.com/$USER/go-fdo-server.git
cd go-fdo-server/deployments/compose/
```

Some values are hard coded in the `servers.yaml` such as database passwords and file paths. Update as needed.

Create folders to be used for storing the database, certs and owner vouchers:

```bash
mkdir -p /tmp/go-fdo/{db,certs,ov}
```

## Generate certs

Generate the certs to be used with the FDO Servers, placing files under `/tmp/go-fdo/certs`.

```bash

# Manufacturer
openssl ecparam -name prime256v1 -genkey -out "/tmp/go-fdo/certs/manufacturer.key" -outform der
openssl req -x509 -key "/tmp/go-fdo/certs/manufacturer.key" -keyform der \
  -out "/tmp/go-fdo/certs/manufacturer.crt" -days 365 -subj "/C=US/O=Example/CN=Manufacturer"

# Device CA (key + cert)
openssl ecparam -name prime256v1 -genkey -out "/tmp/go-fdo/certs/device-ca.key" -outform der
openssl req -x509 -key "/tmp/go-fdo/certs/device-ca.key" -keyform der \
  -out "/tmp/go-fdo/certs/device-ca.crt" -days 365 -subj "/C=US/O=Example/CN=Device CA"

# Owner (key + cert)
openssl ecparam -name prime256v1 -genkey -out "/tmp/go-fdo/certs/owner.key" -outform der
openssl req -x509 -key "/tmp/go-fdo/certs/owner.key" -keyform der \
  -out "/tmp/go-fdo/certs/owner.crt" -days 365 -subj "/C=US/O=Example/CN=Owner"

```
Relabel the files

```bash
# SELinux: update path for containers
sudo chcon -R -t container_file_t /tmp/go-fdo

# make keys and certs readable to the container user
sudo chmod -R a+rX /tmp/go-fdo
```

## Start the stack

If you would like to run the containers rootless, rootless Podman needs subuid/subgid ranges in /etc/subuid and /etc/subgid for the user running containers. Add them for your user:

```bash
USER=(your local username)
sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 $USER
```

After which, **log out and back in again** for the change to take effect.

```bash
podman system migrate
```
From the `go-fdo-server/deployments/compose/` directory, run the following commands:

Podman:

```bash
podman-compose -f servers.yaml up -d manufacturer owner rendezvous
```

Docker:

```bash
docker-compose -f servers.yaml up -d manufacturer owner rendezvous
```

## Health checks

```bash
curl -fsS http://127.0.0.1:8041/health
curl -fsS http://127.0.0.1:8038/health
curl -fsS http://127.0.0.1:8043/health
```

All three should return `{"version":"1.1","status":"OK"}`.

## Minimal end-to-end test (DI → voucher → TO0 → TO2)

This mirrors the main README’s flow, pointing at the containerized services and the same endpoints.

```bash
# Store Rendezvous info (on Manufacturer)
curl -fsS -X POST "http://127.0.0.1:8038/api/v1/rvinfo" \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"ip":"127.0.0.1","protocol":"http","device_port":"8041","owner_port":"8041","dns":"rendezvous"}]'

# Verify its been saved:
curl -fsS "http://127.0.0.1:8038/api/v1/rvinfo"

# (Recommended) Set Owner redirect (on Owner)
curl -fsS -X POST "http://127.0.0.1:8043/api/v1/owner/redirect" \
  -H 'Content-Type: text/plain' \
  --data-raw '[{"ip":"127.0.0.1","protocol":"http","port":"8043","dns":"owner"}]'

# Verify its been saved:
curl -fsS "http://127.0.0.1:8043/api/v1/owner/redirect"

# Device Initialization (DI) at Manufacturing
go-fdo-client device-init "http://127.0.0.1:8038"   --device-info localtest --key ec256 --debug --blob "cred.bin"

# Extract GUID from the device credential blob
GUID=$(go-fdo-client print --blob "cred.bin" | grep -oE '[0-9a-fA-F]{32}' | head -n1)
echo "GUID=${GUID}"

# Download voucher from Manufacturing
curl -fsS "http://127.0.0.1:8038/api/v1/vouchers/${GUID}"   > /tmp/go-fdo/ov/ownervoucher.pem

# Upload to Owner
curl -fsS -X POST "http://127.0.0.1:8043/api/v1/owner/vouchers"   --data-binary @/tmp/go-fdo/ov/ownervoucher.pem

# Trigger TO0 (Owner → Rendezvous)
curl -s "http://127.0.0.1:8043/api/v1/to0/${GUID}"

# Run TO2 (onboarding)
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob "cred.bin"
```

After onboarding you should see a message that ends with: `FIDO Device Onboard Complete`.

## To stop and remove

```bash

podman-compose -f servers.yaml down -v manufacturer owner rendezvous

# or:

docker-compose -f servers.yaml down -v manufacturer owner rendezvous

```
