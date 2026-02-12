# Certificate Setup Guide

This guide explains how to set up certificates for the go-fdo-server in both production and development environments.

## Table of Contents

- [Overview](#overview)
- [Certificate Types](#certificate-types)
- [Quick Start: Single-Host Testing](#quick-start-single-host-testing)
- [Production Deployment (Multi-Host)](#production-deployment-multi-host)
- [Troubleshooting](#troubleshooting)

## Overview

**Platform Note**: This guide uses file paths and commands for Fedora, RHEL, and CentOS. If you are using a different operating system or distribution, adjust the certificate and configuration file paths accordingly.

The go-fdo-server requires certificates for FDO protocol operation:

1. **Manufacturer Certificate** (manufacturer.key/crt)
   - Private key: Local to manufacturer server only
   - Certificate: Local to manufacturer server only

2. **Owner Certificate** (owner.key/crt)
   - Private key: Local to owner server only
   - Certificate: Generated on owner server, then copied to manufacturer server

3. **Device CA Certificate** (device-ca.key/crt)
   - Shared: MUST be identical on both manufacturer and owner servers

## Certificate Types

### Local Certificates

**Manufacturer Certificate** (`/etc/pki/go-fdo-server/manufacturer.key/crt`)
- Used only by the manufacturer server
- Signs vouchers during device initialization (DI protocol)
- Must be created manually or using the helper script
- Default filenames in configs: `manufacturer-example.key/crt`

**Owner Certificate** (`/etc/pki/go-fdo-server/owner.key/crt`)
- Private key (.key): Used only by owner server for TO2 protocol
- Certificate (.crt): Generated on owner server, then copied to manufacturer server
- Manufacturer needs owner certificate to extend vouchers to the correct owner
- Must be created manually or using the helper script
- Default filenames in configs: `owner-example.key/crt`

### Shared Certificates

**Device CA Certificate** (`/etc/pki/go-fdo-server/device-ca.key/crt`)
- MUST be identical on both manufacturer and owner servers
- Signs device certificates during DI protocol (manufacturer)
- Verifies device certificates during TO2 protocol (owner)
- Must be created once and distributed to both servers

**NOTE**: The device-ca certificate chain must be intact across both servers for FDO to function correctly. Independent generation breaks the signing/verification chain.

### HTTPS/TLS Certificates

FDO protocol certificates are separate from HTTPS server certificates:

- **FDO protocol**: Uses manufacturer/owner/device-ca certificates (this guide)
- **HTTPS server**: Configured via `--http-cert` and `--http-key` flags or reverse proxy

For production, use proper TLS certificates from a trusted CA (Let's Encrypt, internal CA, etc.) for the HTTPS server.

## Quick Start: Single-Host Testing

For development or testing with all FDO services on one host, use the provided helper script:

```bash
sudo /usr/libexec/go-fdo-server/generate-go-fdo-server-certs.sh
```

This script generates ALL certificates (manufacturer, owner, and device-ca) in `/etc/pki/go-fdo-server/` and sets appropriate permissions.

**Generated files:**
- `device-ca-example.key` / `device-ca-example.crt` (shared certificate)
- `manufacturer-example.key` / `manufacturer-example.crt` (manufacturer local)
- `owner-example.key` / `owner-example.crt` (owner local)

After running the script, you can start the services:

```bash
sudo systemctl start go-fdo-server-manufacturer.service
sudo systemctl start go-fdo-server-rendezvous.service
sudo systemctl start go-fdo-server-owner.service
```

**Important**: These are self-signed test certificates. Never use them in production.

## Production Deployment (Multi-Host)

For production deployments with manufacturer, rendezvous, and owner on separate hosts:

### Step 1: Generate Device CA (Once, Secure Host)

Generate the shared device CA on a secure administration host:

```bash
# Generate device CA private key (DER format)
openssl ecparam -name prime256v1 -genkey -out device-ca.key -outform der

# Generate self-signed device CA certificate (valid 10 years)
openssl req -x509 -key device-ca.key -keyform der -out device-ca.crt \
  -days 3650 -subj "/C=US/O=YourOrg/CN=FDO Device CA"

# Secure the private key
chmod 600 device-ca.key
```

**IMPORTANT**: Keep the device-ca.key secure. This is the trust anchor for all devices.

### Step 2: Distribute Device CA

Securely copy device CA files to both servers:

**To Manufacturer Server:**
```bash
# Copy both key and certificate to manufacturer
scp device-ca.key device-ca.crt manufacturer-host:/tmp/

# Set ownership and move to correct location (on manufacturer host)
ssh manufacturer-host
sudo mv /tmp/device-ca.key /tmp/device-ca.crt /etc/pki/go-fdo-server/
sudo chown go-fdo-server-manufacturer:go-fdo-server /etc/pki/go-fdo-server/device-ca.*
sudo chmod 640 /etc/pki/go-fdo-server/device-ca.key
sudo chmod 644 /etc/pki/go-fdo-server/device-ca.crt
```

**To Owner Server:**
```bash
# Copy only certificate to owner (key not needed)
scp device-ca.crt owner-host:/tmp/

# Set ownership and move to correct location (on owner host)
ssh owner-host
sudo mv /tmp/device-ca.crt /etc/pki/go-fdo-server/
sudo chown go-fdo-server-owner:go-fdo-server /etc/pki/go-fdo-server/device-ca.crt
sudo chmod 644 /etc/pki/go-fdo-server/device-ca.crt
```

**Note**: The owner server only needs device-ca.crt for verification, not the private key.

### Step 3: Generate Local Certificates

**On Manufacturer Server:**
```bash
# Generate manufacturer private key (DER format)
openssl ecparam -name prime256v1 -genkey -out /etc/pki/go-fdo-server/manufacturer.key -outform der

# Generate manufacturer certificate
openssl req -x509 -key /etc/pki/go-fdo-server/manufacturer.key -keyform der \
  -out /etc/pki/go-fdo-server/manufacturer.crt \
  -days 3650 -subj "/C=US/O=YourOrg/CN=FDO Manufacturer"

# Set permissions
sudo chown go-fdo-server-manufacturer:go-fdo-server /etc/pki/go-fdo-server/manufacturer.*
sudo chmod 640 /etc/pki/go-fdo-server/manufacturer.key
sudo chmod 644 /etc/pki/go-fdo-server/manufacturer.crt
```

**On Owner Server:**
```bash
# Generate owner private key (DER format)
openssl ecparam -name prime256v1 -genkey -out /etc/pki/go-fdo-server/owner.key -outform der

# Generate owner certificate
openssl req -x509 -key /etc/pki/go-fdo-server/owner.key -keyform der \
  -out /etc/pki/go-fdo-server/owner.crt \
  -days 3650 -subj "/C=US/O=YourOrg/CN=FDO Owner"

# Set permissions
sudo chown go-fdo-server-owner:go-fdo-server /etc/pki/go-fdo-server/owner.*
sudo chmod 640 /etc/pki/go-fdo-server/owner.key
sudo chmod 644 /etc/pki/go-fdo-server/owner.crt
```

### Step 4: Exchange Owner Certificate

The manufacturer server needs a copy of the owner's public certificate to extend ownership vouchers during device initialization.

**From Owner Server, copy certificate to manufacturer:**
```bash
scp /etc/pki/go-fdo-server/owner.crt manufacturer-host:/tmp/
```

**On Manufacturer Server:**
```bash
# Move to correct location
sudo mv /tmp/owner.crt /etc/pki/go-fdo-server/

# Set ownership and permissions
sudo chown go-fdo-server-manufacturer:go-fdo-server /etc/pki/go-fdo-server/owner.crt
sudo chmod 644 /etc/pki/go-fdo-server/owner.crt
```

### Step 5: Update Configuration Files

Edit `/etc/go-fdo-server/manufacturing.yaml` and `/etc/go-fdo-server/owner.yaml` to update certificate paths from the default `-example` suffix to the plain filenames created above (e.g., change `manufacturer-example.key` to `manufacturer.key`).

### Step 6: Start Services

```bash
# Manufacturer host
sudo systemctl enable --now go-fdo-server-manufacturer.service

# Rendezvous host (no FDO protocol certificates needed)
sudo systemctl enable --now go-fdo-server-rendezvous.service

# Owner host
sudo systemctl enable --now go-fdo-server-owner.service
```
## Additional Resources

- FDO Specification: https://fidoalliance.org/specs/FDO/
- Project Documentation: https://github.com/fido-device-onboard/go-fdo-server
