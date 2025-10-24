# Dockerfile Usage Guide

This guide explains how to build and use the go-fdo-server Docker container image for local development and testing.

## Overview

The Dockerfile in this repository creates a minimal, secure container image for running the go-fdo-server. The image uses a multi-stage build process to produce a small final image based on a distroless base.

### Image Characteristics

- **Base Image**: `gcr.io/distroless/static-debian12:nonroot` (minimal, security-hardened)
- **Build Stage**: `golang:1.25-alpine` (compact build environment)
- **Included Binaries**:
  - `go-fdo-server` - main server binary
  - `curl` - static curl binary for health checks

## Prerequisites

- Docker or Podman installed (rootless Podman recommended for enhanced security)
- `openssl` for generating keys and certificates
- The Dockerfile from this repository
- Sufficient disk space for build (~500MB including layers)

### Container Runtime Notes

This guide provides examples for both Docker and rootless Podman. Rootless Podman offers additional security benefits by running containers without root privileges on the host system.

**Rootless Podman advantages**:
- No daemon running as root
- User namespaces provide additional isolation
- Reduced attack surface

## Building the Image

From the repository root directory:

**With Docker:**
```bash
docker build -t go-fdo-server:latest .
```

**With rootless Podman:**
```bash
podman build -t go-fdo-server:latest .
```

Verify the image was created:

**With Docker:**
```bash
docker images | grep go-fdo-server
```

**With rootless Podman:**
```bash
podman images | grep go-fdo-server
```

## Preparing the Host Environment

Before running containers, prepare the necessary certificates and directories.

**Important**: This guide uses `/tmp/fdo` for simplicity in testing. For production deployments, use a persistent directory outside of `/tmp` (e.g., `/var/lib/fdo` or `/opt/fdo`) to ensure data is not lost on system reboot.

### Create Directory Structure

```bash
mkdir -p /tmp/fdo/certs /tmp/fdo/db /tmp/fdo/files
```

### Generate Test Certificates

```bash
# Manufacturer key (DER format)
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/certs/manufacturer.key -outform der

# Manufacturer certificate (PEM format)
openssl req -x509 -key /tmp/fdo/certs/manufacturer.key -keyform der \
  -out /tmp/fdo/certs/manufacturer.crt -days 365 \
  -subj "/C=US/O=Example/CN=Manufacturer"

# Device CA key (DER format)
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/certs/device_ca.key -outform der

# Device CA certificate (PEM format)
openssl req -x509 -key /tmp/fdo/certs/device_ca.key -keyform der \
  -out /tmp/fdo/certs/device_ca.crt -days 365 \
  -subj "/C=US/O=Example/CN=Device CA"

# Owner key (DER format)
openssl ecparam -name prime256v1 -genkey -out /tmp/fdo/certs/owner.key -outform der

# Owner certificate (PEM format)
openssl req -x509 -key /tmp/fdo/certs/owner.key -keyform der \
  -out /tmp/fdo/certs/owner.crt -days 365 \
  -subj "/C=US/O=Example/CN=Owner"
```

### Set Permissions

**For Docker:**
```bash
# Set ownership to container UID and use restrictive permissions
sudo chown -R 65532:65532 /tmp/fdo
chmod -R u+rwX /tmp/fdo
```

**For rootless Podman:**
```bash
# Make files readable and writable by your user
chmod -R u+rwX /tmp/fdo
```

## Running Containers

### Create Container Network

For containers to communicate with each other, create a shared network first:

**With Docker:**
```bash
docker network create fdo-network
```

**With rootless Podman:**
```bash
podman network create fdo-network
```

### Important Notes

- The distroless base image provides no shell (`sh`, `bash`), so interactive shell sessions via `docker exec -it ... sh` are not possible
- You can still use `docker exec` / `podman exec` to run specific commands that exist in the image (e.g., `curl`, `go-fdo-server`)
- All configuration must be provided via command-line arguments
- Volumes must be mounted for persistent data (databases, keys, certificates)
- Containers must be on the same network to communicate with each other

### Running Individual Services

**Important**: The examples below use `P@ssw0rd1!` as the database password for demonstration purposes. **Always use a strong, unique password in production environments.**

#### Rendezvous Server

**With Docker:**
```bash
docker run -d \
  --name fdo-rendezvous \
  --network fdo-network \
  -p 8041:8041 \
  -v /tmp/fdo:/tmp/fdo \
  go-fdo-server:latest \
  --debug rendezvous 0.0.0.0:8041 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/rendezvous.db"
```

**With rootless Podman:**
```bash
podman run -d \
  --name fdo-rendezvous \
  --network fdo-network \
  --user 0:0 \
  -p 8041:8041 \
  -v /tmp/fdo:/tmp/fdo:z \
  go-fdo-server:latest \
  --debug rendezvous 0.0.0.0:8041 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/rendezvous.db"
```

**Note for rootless Podman**: The `--user 0:0` flag tells the container to run as UID 0, which with user namespace mapping maps to your host UID. This allows the container to access files you own. The `:z` suffix on the volume mount automatically sets the correct SELinux context on SELinux-enabled systems.

#### Manufacturing Server

**With Docker:**
```bash
docker run -d \
  --name fdo-manufacturer \
  --network fdo-network \
  -p 8038:8038 \
  -v /tmp/fdo:/tmp/fdo \
  go-fdo-server:latest \
  --debug manufacturing 0.0.0.0:8038 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/manufacturer.db" \
  --manufacturing-key /tmp/fdo/certs/manufacturer.key \
  --owner-cert /tmp/fdo/certs/owner.crt \
  --device-ca-cert /tmp/fdo/certs/device_ca.crt \
  --device-ca-key /tmp/fdo/certs/device_ca.key
```

**With rootless Podman:**
```bash
podman run -d \
  --name fdo-manufacturer \
  --network fdo-network \
  --user 0:0 \
  -p 8038:8038 \
  -v /tmp/fdo:/tmp/fdo:z \
  go-fdo-server:latest \
  --debug manufacturing 0.0.0.0:8038 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/manufacturer.db" \
  --manufacturing-key /tmp/fdo/certs/manufacturer.key \
  --owner-cert /tmp/fdo/certs/owner.crt \
  --device-ca-cert /tmp/fdo/certs/device_ca.crt \
  --device-ca-key /tmp/fdo/certs/device_ca.key
```

#### Owner Server

**With Docker:**
```bash
docker run -d \
  --name fdo-owner \
  --network fdo-network \
  -p 8043:8043 \
  -v /tmp/fdo:/tmp/fdo \
  go-fdo-server:latest \
  --debug owner 0.0.0.0:8043 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/owner.db" \
  --owner-key /tmp/fdo/certs/owner.key \
  --device-ca-cert /tmp/fdo/certs/device_ca.crt
```

**With rootless Podman:**
```bash
podman run -d \
  --name fdo-owner \
  --network fdo-network \
  --user 0:0 \
  -p 8043:8043 \
  -v /tmp/fdo:/tmp/fdo:z \
  go-fdo-server:latest \
  --debug owner 0.0.0.0:8043 \
  --db-type sqlite --db-dsn "file:/tmp/fdo/db/owner.db" \
  --owner-key /tmp/fdo/certs/owner.key \
  --device-ca-cert /tmp/fdo/certs/device_ca.crt
```

## Health Checks

### From Host System

The simplest approach is to check from your host:

```bash
curl http://localhost:8041/health  # Rendezvous
curl http://localhost:8038/health  # Manufacturing
curl http://localhost:8043/health  # Owner
```

### From Inside Container

The image includes a static curl binary, so you can also run health checks from inside the container:

**With Docker:**
```bash
docker exec fdo-rendezvous curl -f http://localhost:8041/health
docker exec fdo-manufacturer curl -f http://localhost:8038/health
docker exec fdo-owner curl -f http://localhost:8043/health
```

**With rootless Podman:**
```bash
podman exec fdo-rendezvous curl -f http://localhost:8041/health
podman exec fdo-manufacturer curl -f http://localhost:8038/health
podman exec fdo-owner curl -f http://localhost:8043/health
```

**Note**: While curl is available, the distroless image has no shell, so you cannot run `docker exec -it <container> sh` for interactive debugging.

## Troubleshooting

### Container Fails to Start

Check logs for error messages:

**With Docker:**
```bash
docker logs fdo-rendezvous
docker logs fdo-manufacturer
docker logs fdo-owner
```

**With rootless Podman:**
```bash
podman logs fdo-rendezvous
podman logs fdo-manufacturer
podman logs fdo-owner
```

Common issues:
- **"unable to open database file"**: Permission or SELinux issue - see sections below
- **Address already in use**: Another service is using the port, stop it or use a different port mapping
- **Invalid database password**: Ensure password meets requirements (8+ chars, number, uppercase, special character)
- **Port binding errors (rootless Podman)**: Ports below 1024 require special setup or port mapping

### Permission Errors / SQLite "unable to open database file"

If containers fail with SQLite errors like "unable to open database file", this is likely a permission issue.

**For rootless Podman**:

1. **Ensure you're using `--user 0:0` and `:z` flag**:
   ```bash
   podman run --user 0:0 -v /tmp/fdo:/tmp/fdo:z ...
   ```
   The `--user 0:0` flag maps container UID 0 to your host UID, allowing access to files you own. The `:z` suffix automatically sets the correct SELinux context on SELinux-enabled systems.

2. **Verify file permissions**:
   ```bash
   chmod -R u+rwX /tmp/fdo
   ```

**For Docker**:

1. **Ensure files are owned by UID 65532**:
   ```bash
   sudo chown -R 65532:65532 /tmp/fdo
   chmod -R u+rwX /tmp/fdo
   ```

## Additional Resources

- **FDO Specification**: [FIDO Device Onboard](https://fidoalliance.org/specs/FDO/)
- **Main README**: See [README.md](README.md) for non-containerized usage
- **FSIM Guide**: See [FSIM_USAGE.md](FSIM_USAGE.md) for Service Info Module details
- **Client Usage**: See go-fdo-client repository for device-side container usage
