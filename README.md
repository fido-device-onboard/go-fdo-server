# FIDO Device Onboard - Go Server

`go-fdo-server` is a server implementation of FIDO Device Onboard specification in Go.

[fdo]: https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/FIDO-Device-Onboard-PS-v1.1-20220419.html
[cbor]: https://www.rfc-editor.org/rfc/rfc8949.html
[cose]: https://datatracker.ietf.org/doc/html/rfc8152

## Prerequisites

- Go 1.25.0 or later
- `openssl` and `curl` available

## Quickstart: Run the three services locally (no TLS)
This project exposes separate subcommands for each role: `rendezvous`, `manufacturing`, and `owner`.

Install the server binary:

```bash
go install github.com/fido-device-onboard/go-fdo-server@latest
```

Install the client binary:

```bash
go install github.com/fido-device-onboard/go-fdo-client@latest
```

Generate test keys/certs (under /tmp/fdo/keys):

```bash
mkdir -p /tmp/fdo/keys

# Manufacturer EC key + self-signed cert
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/keys/manufacturer_key.der -outform der
openssl req -x509 -key /tmp/fdo/keys/manufacturer_key.der -keyform der -out /tmp/fdo/keys/manufacturer_cert.pem -days 365 -subj "/C=US/O=Example/CN=Manufacturer"

# Device CA EC key + self-signed cert
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/keys/device_ca_key.der -outform der
openssl req -x509 -key /tmp/fdo/keys/device_ca_key.der -keyform der -out /tmp/fdo/keys/device_ca_cert.pem -days 365 -subj "/C=US/O=Example/CN=Device"

# Owner EC key + self-signed cert
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/keys/owner_key.der -outform der
openssl req -x509 -key /tmp/fdo/keys/owner_key.der -keyform der -out /tmp/fdo/keys/owner_cert.pem -days 365 -subj "/C=US/O=Example/CN=Owner"

```

Start the services in three terminals (or background them). Use distinct databases under /tmp/fdo/db and a strong DB passphrase.

```bash
mkdir -p /tmp/fdo/db /tmp/fdo/keys /tmp/fdo/ov

# Rendezvous (127.0.0.1:8041)
go-fdo-server --log-level=debug rendezvous 127.0.0.1:8041 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/rv.db"

# Manufacturing (127.0.0.1:8038)
go-fdo-server --log-level=debug manufacturing 127.0.0.1:8038 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/mfg.db" \
  --manufacturing-key /tmp/fdo/keys/manufacturer_key.der \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --device-ca-key  /tmp/fdo/keys/device_ca_key.der \
  --owner-cert     /tmp/fdo/keys/owner_cert.pem

# Owner (127.0.0.1:8043)
go-fdo-server --log-level=debug owner 127.0.0.1:8043 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/own.db" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key      /tmp/fdo/keys/owner_key.der
```

Health checks:

```bash
curl -fsS http://127.0.0.1:8041/health
curl -fsS http://127.0.0.1:8038/health
curl -fsS http://127.0.0.1:8043/health
```

## Managing RV Info Data
### Create New RV Info Data
Send a POST request to create new RV info data, which is stored in the Manufacturer’s database:
```
curl --location --request POST 'http://localhost:8038/api/v1/rvinfo' \                      
--header 'Content-Type: text/plain' \
--data-raw '[{"dns":"fdo.example.com","device_port":"8041","owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]'
```
To bypass the TO1 protocol set RVBypass using
```
curl --location --request POST 'http://localhost:8038/api/v1/rvinfo' \                      
--header 'Content-Type: text/plain' \
--data-raw '[{"dns":"fdo.example.com","device_port":"8043","rv_bypass": "true", "owner_port":"8043","protocol":"http","ip":"127.0.0.1"}]'
```
### Fetch Current RV Info Data
Send a GET request to fetch the current RV info data:
```
curl --location --request GET 'http://localhost:8038/api/v1/rvinfo'
```

### Update Existing RV Info Data
Send a PUT request to update the existing RV info data:
```
curl --location --request PUT 'http://localhost:8038/api/v1/rvinfo' \
--header 'Content-Type: text/plain' \
--data-raw '[{"dns":"fdo.example.com","device_port":"8041","rv_bypass": "false", "owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]'
```

## Managing Owner Redirect Data
### Create New Owner Redirect Data
Send a POST request to create new owner redirect data, which is stored in the Owner’s database:
```
curl --location --request POST 'http://localhost:8043/api/v1/owner/redirect' \
--header 'Content-Type: text/plain' \
--data-raw '[{"dns":"fdo.example.com","port":"8043","protocol":"http","ip":"127.0.0.1"}]'
```

### View and Update Existing Owner Redirect Data
Use GET and PUT requests to view and update existing owner redirect data.
```
curl --location --request GET 'http://localhost:8043/api/v1/owner/redirect'

curl --location --request PUT 'http://localhost:8043/api/v1/owner/redirect' \
--header 'Content-Type: text/plain' \
--data-raw '[{"dns":"fdo.test.com","port":"8085","protocol":"http","ip":"127.0.0.1"}]'
```


## Basic onboarding flow (device DI → voucher → TO0 → TO2)

1. Device Initialization (DI) with `go-fdo-client` (stores `/tmp/fdo/cred.bin`):

```bash
go-fdo-client device-init 'http://localhost:8038' \
  --device-info gotest \
  --key ec256 \
  --debug \
  --blob /tmp/fdo/cred.bin
```

2. Extract the device GUID:

```bash
GUID=$(go-fdo-client print --blob /tmp/fdo/cred.bin | grep -oE '[0-9a-fA-F]{32}' | head -n1)
echo "GUID=${GUID}"
```

3. Download voucher from Manufacturing and upload to Owner:

```bash
curl -v "http://localhost:8038/api/v1/vouchers?guid=${GUID}" > /tmp/fdo/ov/ownervoucher
curl -X POST 'http://localhost:8043/api/v1/owner/vouchers' --data-binary @/tmp/fdo/ov/ownervoucher
```

4. TO0 on Owner server:

When importing a voucher, the owner automatically starts to0 with the rendezvous server and tries until the onboard is successful.

5. Run onboarding (TO2) and verify success:

```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin | tee /tmp/fdo/client-onboard.log
grep -F 'FIDO Device Onboard Complete' /tmp/fdo/client-onboard.log >/dev/null && echo 'Onboarding OK'
```

Cleanup:

```bash
rm -rf /tmp/fdo
```


## TLS configuration

1. Generate key and certificate for the server

```bash
openssl ecparam -genkey -name prime256v1 -out server.key
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650 -subj "/C=US/O=Example/CN=example"
```

2. Run go-fdo-server

```bash
go-fdo-server --http-cert <cert-path> --http-key <key-path> ...
```


## Configuration File Support

The FDO server supports configuration files for all three subcommands: `manufacturing`, `owner`, and `rendezvous`. Configuration files can be used to specify all command-line options, making it easier to manage complex configurations.

Each subcommand supports a `--config` flag that accepts a path to a TOML or YAML configuration file.

For a complete reference of all available configuration options, see [CONFIG.md](CONFIG.md).

