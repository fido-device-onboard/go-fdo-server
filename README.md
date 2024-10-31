# FIDO Device Onboard - Go Server

`go-fdo-server` is a server implementation of FIDO Device Onboard specification in Go.

[fdo]: https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/FIDO-Device-Onboard-PS-v1.1-20220419.html
[cbor]: https://www.rfc-editor.org/rfc/rfc8949.html
[cose]: https://datatracker.ietf.org/doc/html/rfc8152

## Prerequisites

- Go 1.23.0 or later
- A Go module initialized with `go mod init`


The `update-deps.sh` script updates all dependencies in your Go module to their latest versions and cleans up the `go.mod` and `go.sum` files.

To update your dependencies, simply run the script:
```sh
./update-deps.sh
```

## Building the Example Server Application

The example server application can be built with `go build` directly

```console
$ go build -o fdo_server ./cmd/fdo_server/
$ ./fdo_server

Usage:
  fdo_server [--] [options]

Server options:
  -command-date
        Use fdo.command FSIM to have device run "date --utc"
  -db string
        SQLite database file path
  -db-pass string
        SQLite database encryption-at-rest passphrase
  -debug
        Print HTTP contents
  -download file
        Use fdo.download FSIM for each file (flag may be used multiple times)
  -ext-http addr
        External address devices should connect to (default "127.0.0.1:${LISTEN_PORT}")
  -http addr
        The address to listen on (default "localhost:8080")
  -import-voucher path
        Import a PEM encoded voucher file at path
  -insecure-tls
        Listen with a self-signed TLS certificate
  -print-owner-public type
        Print owner public key of type and exit
  -resale-guid guid
        Voucher guid to extend for resale
  -resale-key path
        The path to a PEM-encoded x.509 public key for the next owner
  -reuse-cred
        Perform the Credential Reuse Protocol in TO2
  -upload file
        Use fdo.upload FSIM for each file (flag may be used multiple times)
  -upload-dir path
        The directory path to put file uploads (default "uploads")
  -wget url
        Use fdo.wget FSIM for each url (flag may be used multiple times)

Key types:
  - RSA2048RESTR
  - RSAPKCS
  - RSAPSS
  - SECP256R1
  - SECP384R1

Encryption suites:
  - A128GCM
  - A192GCM
  - A256GCM
  - AES-CCM-64-128-128 (not implemented)
  - AES-CCM-64-128-256 (not implemented)
  - COSEAES128CBC
  - COSEAES128CTR
  - COSEAES256CBC
  - COSEAES256CTR

Key exchange suites:
  - DHKEXid14
  - DHKEXid15
  - ASYMKEX2048
  - ASYMKEX3072
  - ECDH256
  - ECDH384
```

## Starting the FDO Server
This guide provides instructions to set up and run the FDO server and client instances for different roles: Manufacturer, Rendezvous (RV), and Owner.
### Manufacturer Instance
Start the FDO server with the test database:
```sh
./fdo_server -http 127.0.0.1:8038 -db ./mfg.db -db-pass <db-password> -debug
```
This server instance acts as the Manufacturer.

### RV Instance
Start another instance of the FDO server on a different port with a different database:
```
./fdo_server -http 127.0.0.1:8041 -db ./rv.db -db-pass <db-password> -debug
```
This server instance acts as the RV.

### Owner Instance
Start another instance of the FDO server on a different port with a different database:
```
./fdo_server -http 127.0.0.1:8043 -db ./own.db -db-pass <db-password> -debug
```
This server instance acts as the Owner.

## Managing RV Info Data
### Create New RV Info Data
Send a POST request to create new RV info data, which is stored in the Manufacturer’s database:
```
curl --location --request POST 'http://localhost:8038/api/v1/rvinfo' \
--header 'Content-Type: text/plain' \
--data-raw '[[[5,"127.0.0.1"],[3,8041],[12,1],[2,"127.0.0.1"],[4,8041]]]'
```
To bypass the TO1 protocol set RVBypass using
```
curl --location --request POST 'http://localhost:8038/api/v1/rvinfo' \
--header 'Content-Type: text/plain' \
--data-raw '[[[5,"127.0.0.1"],[3,8041],[14],[12,1],[2,"127.0.0.1"],[4,8041]]]'
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
--data-raw '[[[5,"127.0.0.1"],[3,8041],[14,false],[12,1],[2,"127.0.0.1"],[4,8041]]]'
```

## Managing Owner Redirect Data
### Create New Owner Redirect Data
Send a POST request to create new owner redirect data, which is stored in the Owner’s database:
```
curl --location --request POST 'http://localhost:8043/api/v1/owner/redirect' \
--header 'Content-Type: text/plain' \
--data-raw '[["127.0.0.1","127.0.0.1",8043,3]]'
```

### View and Update Existing Owner Redirect Data
Use GET and PUT requests to view and update existing owner redirect data.


## Fetch and Post Voucher
Fetch a Voucher
Fetch a voucher using curl and save it to a file named ownervoucher:
```
curl --location --request GET 'http://localhost:8038/api/v1/vouchers?guid=<guid>' -o ownervoucher
```
Post the Voucher to RV and Owner Server
Post the fetched voucher to the RV and Owner server using curl:
```
curl -X POST 'http://localhost:8041/api/v1/owner/vouchers' -d @ownervoucher
curl -X POST 'http://localhost:8043/api/v1/owner/vouchers' -d @ownervoucher
```
## Execute DI from the FDO GO Client.
For Running the FDO GO Client setup, please refer to the FDO Go Client README.
## Execute TO0
Execute the TO0 by providing DI GUID from FDO GO Client:
```
curl --location --request GET 'http://localhost:8043/api/v1/to0/<guid>'
```
TO0 will be completed in the respective Owner and RV.
## Execute TO1 and TO2 from the FDO GO Client.
## Building and Running the Example Server Application using Containers

### Prerequisites

- Container runtime (Docker or Podman)
- Make

### Makefile Targets

- `build`: Builds the Server image.
- `run`: Runs the Server container.
- `clean`: Removes the Server image.
- `all`: Builds the Server image and then runs the container.

### Variables

The following variables can be set to customize the behavior of the `make run` command:

- `CONTAINER_RUNTIME`: The container runtime executable to use (default: `docker`).
- `IMAGE_NAME`: The name of the container image (default: `fdo_server`).
- `CONTAINER_NAME`: The name of the container (default: `fdo_server`).
- `DB_PATH`: The path to the SQLite database file (default: `./test.db`).
- `DB_PASS`: The SQLite database encryption-at-rest passphrase. **NOTE**:This field should be populated before starting the container.
- `NETWORK`: The container network setting (default: `host`).
- `DEBUG`: Debug flag to print HTTP contents (default: `--debug`).
- `HTTP_ADDR`: The address to listen on (default: `localhost:8080`).
- `EXT_HTTP_ADDR`: The external address devices should connect to (default: `127.0.0.1:8080`).
- `UPLOAD_DIR`: The directory path to put file uploads (default: `uploads`).
- `DOWNLOAD_FILES`: Files to use with `fdo.download` FSIM (can be multiple files).
- `UPLOAD_FILES`: Files to use with `fdo.upload` FSIM (can be multiple files).
- `IMPORT_VOUCHER`: Path to import a PEM encoded voucher file.
- `INSECURE_TLS`: Flag to listen with a self-signed TLS certificate.
- `PRINT_OWNER_PUBLIC`: Type of owner public key to print and exit.
- `RESALE_GUID`: Voucher GUID to extend for resale.
- `RESALE_KEY`: Path to a PEM-encoded x.509 public key for the next owner.
- `REUSE_CRED`: Flag to perform the Credential Reuse Protocol in TO2.
- `WGET_URLS`: URLs to use with `fdo.wget` FSIM (can be multiple URLs).

## Usage

### Building the container image

To build the container image, run:

```console
make build
```

### Running the container
To start the FDO Go service as a container, run:

```console
make run
```
This will start the container with the specified network settings and database path.

```console
make copy
```
This will copy the files uploaded using fdo.upload FSIM module to `app-data` folder present in host system.

### Stoping the container
To stop the container, run:
```console
make stop
```

### Default Target
To build and run the container in one step, run:
```console
make all
```

***NOTE:***
Supports all server parameters specified in the building section. Use network mode based on the host machine and requirements.
