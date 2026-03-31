# Quickstart: Install, Configure and Run FDO

This document is a guide to running FDO from the command line in a test environment.

This guide will explain how to:
* install the FDO server and client applications
* generate test certificate and key files
* configure the FDO servers
* run all three FDO servers
* perform a basic device onboarding

This is a brief guide to these operations. Refer to the [User Guide](README.md) for detailed documentation.

## Installation

Install the `go-fdo-server` server application. From the top level directory:

```bash
go install
```

Install the `go-fdo-client` application from its source repository:

```bash
go install github.com/fido-device-onboard/go-fdo-client@latest
```

**Note**: Add `export PATH=$HOME/go/bin:$PATH` to your shell configuration file to run Go binaries without the `./` prefix.

## Certificate Generation

Generate test keys/certs (under `/tmp/fdo/keys`):

```bash
mkdir -p /tmp/fdo/keys

# Manufacturer EC key + self-signed cert
openssl ecparam \
  -name prime256v1 \
  -genkey \
  -out /tmp/fdo/keys/manufacturer_key.der \
  -outform der
openssl req \
  -x509 \
  -key /tmp/fdo/keys/manufacturer_key.der \
  -keyform der \
  -out /tmp/fdo/keys/manufacturer_cert.pem \
  -days 365 \
  -subj "/C=US/O=Example/CN=Manufacturer"

# Device CA EC key + self-signed cert
openssl ecparam \
  -name prime256v1 \
  -genkey \
  -out /tmp/fdo/keys/device_ca_key.der \
  -outform der
openssl req \
  -x509 \
  -key /tmp/fdo/keys/device_ca_key.der \
  -keyform der \
  -out /tmp/fdo/keys/device_ca_cert.pem \
  -days 365 \
  -subj "/C=US/O=Example/CN=Device"

# Owner EC key + self-signed cert
openssl ecparam \
  -name prime256v1 \
  -genkey \
  -out /tmp/fdo/keys/owner_key.der \
  -outform der
openssl req \
  -x509 \
  -key /tmp/fdo/keys/owner_key.der \
  -keyform der \
  -out /tmp/fdo/keys/owner_cert.pem \
  -days 365 \
  -subj "/C=US/O=Example/CN=Owner"

```

**Note**: Certificates are NOT auto-generated. For single-host testing with RPM-based installations, a helper script is provided (location may vary by distribution). For production deployments and detailed certificate setup information, see the [User Guide](README.md).

## Running the FDO Servers
Start the services in three terminals (or background them). Use distinct databases under `/tmp/fdo/db` and a strong DB passphrase.

```bash
mkdir -p /tmp/fdo/db /tmp/fdo/keys /tmp/fdo/ov

# Rendezvous (127.0.0.1:8041)
go-fdo-server rendezvous 127.0.0.1:8041 \
  --log-level=debug \
  --db-type sqlite \
  --db-dsn "file:/tmp/fdo/db/rv.db"

# Manufacturing (127.0.0.1:8038)
go-fdo-server manufacturing 127.0.0.1:8038 \
  --log-level=debug \
  --db-type sqlite \
  --db-dsn "file:/tmp/fdo/db/mfg.db" \
  --manufacturing-key /tmp/fdo/keys/manufacturer_key.der \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --device-ca-key /tmp/fdo/keys/device_ca_key.der \
  --owner-cert /tmp/fdo/keys/owner_cert.pem

# Owner (127.0.0.1:8043)
go-fdo-server owner 127.0.0.1:8043 \
  --log-level=debug \
  --db-type sqlite \
  --db-dsn "file:/tmp/fdo/db/own.db" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der
```

Verify that the server's network endpoints are accessible:

```bash
curl -fsS http://127.0.0.1:8041/health
curl -fsS http://127.0.0.1:8038/health
curl -fsS http://127.0.0.1:8043/health
```

## Configuring the FDO Servers

A minimal configuration must be set prior to performing onboarding. This involves:
* setting the `RVInfo` configuration on the Manufacturing server
* setting the `RVTO2Addr` configuration on the Owner server

### Managing the Manufacturing Server `RVInfo` Configuration

The `RVInfo` configuration is used to determine the network address of the Rendezvous server. The configuration must include the Rendezvous server's IP address or DNS name, and TCP port.

> **Note**: The V2 API (`/api/v2/rvinfo`) is recommended for new deployments. It uses OpenAPI specification format with strict typing (integer ports). The V1 API (`/api/v1/rvinfo`) is deprecated and will be removed in a future release, but is still supported for backward compatibility.

To set the `RVInfo` configuration, send a `POST` HTTP request containing the Rendezvous server information to the Manufacturing server:
```bash
curl -X POST 'http://localhost:8038/api/v1/rvinfo' \
  -H 'Content-Type: text/plain' \
  -d '[{"dns":"fdo.example.com","device_port":"8041","owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]'
```

Verify the configuration has been stored on the Manufacturing server by sending a `GET` request to fetch the current `RVInfo` data:
```bash
curl -X GET 'http://localhost:8038/api/v1/rvinfo'
```

### RV Info V2 API Examples (Recommended)

```bash
# Create or update RV info (V2 format: array of arrays with single-key objects, integer ports)
curl -X PUT 'http://localhost:8038/api/v2/rvinfo' \
  -H 'Content-Type: application/json' \
  -d '[[{"dns":"fdo.example.com"},{"device_port":8041},{"owner_port":8041},{"protocol":"http"},{"ip":"127.0.0.1"}]]'

# Create or update with RV bypass (skip rendezvous, connect directly to owner)
curl -X PUT 'http://localhost:8038/api/v2/rvinfo' \
  -H 'Content-Type: application/json' \
  -d '[[{"dns":"fdo.example.com"},{"device_port":8043},{"owner_port":8043},{"protocol":"http"},{"ip":"127.0.0.1"},{"rv_bypass":true}]]'

# Fetch RV info
curl -X GET 'http://localhost:8038/api/v2/rvinfo'

# Delete RV info
curl -X DELETE 'http://localhost:8038/api/v2/rvinfo'
```

### Managing the Owner Server Redirect Configuration (`RVTO2Addr`)

The Owner server sends its `RVTO2Addr` configuration to the Rendezvous server prior to onboarding a device.  The configuration contains the network address of the Owner server, which the Rendezvous server will pass to the device during onboarding.  The device uses this network address to access the Owner server.

To set the `RVTO2Addr` configuration, send a `POST` HTTP request containing the Owner server's network address to the Owner server:
```bash
curl -X POST 'http://localhost:8043/api/v1/owner/redirect' \
  -H 'Content-Type: text/plain' \
  -d '[{"dns":"fdo.example.com","port":"8043","protocol":"http","ip":"127.0.0.1"}]'
```

Verify the configuration has been stored on the Owner server by sending a `GET` request to fetch the current redirect data:
```bash
curl -X GET 'http://localhost:8043/api/v1/owner/redirect'
```

## Basic Onboarding Flow (Device DI → voucher → TO0 → TO2)

After the FDO servers are properly configured, the `go-fdo-client` can be used to run the device onboarding process. This involves:
* creating credentials on the device
* generating an Ownership Voucher for the device
* installing the Ownership Voucher on the Owner server
* onboarding the device

1. Perform Device Initialization (DI) with `go-fdo-client`. This will create and store the device credentials in `/tmp/fdo/cred.bin`:

```bash
go-fdo-client device-init 'http://localhost:8038' \
  --device-info gotest \
  --key ec256 \
  --debug \
  --blob /tmp/fdo/cred.bin
```

2. Use the `go-fdo-client` to extract the Device GUID from the device credentials:

```bash
GUID=$(go-fdo-client print --blob /tmp/fdo/cred.bin | grep -oE '[0-9a-fA-F]{32}' | head -n1)
echo "GUID=${GUID}"
```

3. Using the Device GUID, download the device's Ownership Voucher from the Manufacturing server and upload it to the Owner server:

```bash
curl -v "http://localhost:8038/api/v1/vouchers/${GUID}" > /tmp/fdo/ov/ownervoucher
curl -X POST 'http://localhost:8043/api/v1/owner/vouchers' \
  --data-binary @/tmp/fdo/ov/ownervoucher
```

Uploading the voucher will cause the Owner server to run the TO0 protocol with the Rendezvous server. On completion of TO0, both the Rendezvous and Owner servers are ready to onboard the device.

4. Use the `go-fdo-client` to run the onboarding protocols (TO1 and TO2). Verify that onboarding completed successfully:

```bash
go-fdo-client onboard \
  --key ec256 \
  --kex ECDH256 \
  --debug \
  --blob /tmp/fdo/cred.bin | tee /tmp/fdo/client-onboard.log
grep -F 'FIDO Device Onboard Complete' /tmp/fdo/client-onboard.log >/dev/null && echo 'Onboarding OK'
```

This completes the FDO onboarding process.

5. To cleanup:

```bash
rm -rf /tmp/fdo
```

