# FIDO Device Onboard - Golang Server

`go-fdo-server` is a Golang implementation of the device onboarding services defined in the FIDO Device Onboard specification. It can operate as a Manufacturing, Rendezvous, or Owner server. It can be used with the `go-fdo-client` device application for a complete FDO onboarding solution.

[fdo]: https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/FIDO-Device-Onboard-PS-v1.1-20220419.html
[go-fdo-client]: https://github.com/fido-device-onboard/go-fdo-client
[cbor]: https://www.rfc-editor.org/rfc/rfc8949.html
[cose]: https://datatracker.ietf.org/doc/html/rfc8152

## Prerequisites

- Go 1.25.0 or later
- `openssl` and `curl` available
- `jq` to parse JSON output (optional)
- `npx` to run `openapi-format`
- `make` to build the `go-fdo-server` application and artifacts

## Build

To build the `go-fdo-server` run `make` in the top level directory:

```bash
$ make
```

## Documentation

New to FDO? The [User Guide](docs/user-guide/README.md) provides an overview of the FDO
onboarding process and explains how to install, configure, and run the FDO servers.

Want to try it quickly? The [Quickstart Guide](docs/user-guide/quick-start.md) walks through
a complete onboarding session from the command line using a minimal test setup.

