# FIDO Service Info Module (FSIM) Usage Guide

This guide explains how to use the standard FIDO Service Info Modules (FSIMs) with the go-fdo-server owner service. FSIMs enable device onboarding with automated file transfers and command execution during the TO2 protocol phase.

## Overview

The go-fdo-server owner service supports four standard FSIMs:

- **fdo.command** - Execute commands on the device
- **fdo.download** - Download files from owner to device  
- **fdo.upload** - Upload files from device to owner
- **fdo.wget** - Have device download files from URLs

FSIMs are activated by adding the corresponding command-line flags when starting the owner service.

## Prerequisites

- FDO server setup completed (see main README.md)
- Owner service configured with proper certificates and keys
- Device successfully initialized and voucher transferred to owner

## fdo.command FSIM

### Purpose
Execute shell commands on the device during onboarding.

### Usage
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-date
```

### Available Commands
- `--command-date`: Executes `date --utc` on the device to display current UTC time

### Example Output
When a device onboards with the command FSIM enabled, the device will execute the specified command and the output will be displayed in the owner service logs.

## fdo.download FSIM

### Purpose
Transfer files from the owner server to the device during onboarding.

### Usage
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-download /path/to/local/file1.txt \
  --command-download /path/to/local/file2.conf
```

### Parameters
- `--command-download <file_path>`: Specify a local file path to transfer to the device
- Flag can be used multiple times to transfer multiple files

### Example
```bash
# Prepare files to download
echo "Device configuration data" > /tmp/device-config.txt
echo "Application settings" > /tmp/app-settings.json

# Start owner with download FSIM
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-download /tmp/device-config.txt \
  --command-download /tmp/app-settings.json
```

### Error Handling
- If a specified file cannot be opened, the owner service will log a fatal error
- File permissions on the owner server must allow reading by the service process

## fdo.upload FSIM

### Purpose
Transfer files from the device to the owner server during onboarding.

### Usage
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --upload-directory /tmp/uploads \
  --command-upload device-logs.txt \
  --command-upload system-info.json
```

### Parameters
- `--upload-directory <dir_path>`: Directory on owner server where uploaded files will be stored
- `--command-upload <filename>`: Name of file to request from device
- Upload flag can be used multiple times for multiple files

### Example
```bash
# Create upload directory
mkdir -p /tmp/fdo-uploads

# Start owner with upload FSIM
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --upload-directory /tmp/fdo-uploads \
  --command-upload /var/log/device.log \
  --command-upload /etc/device-id.txt
```

## fdo.wget FSIM

### Purpose
Instruct the device to download files from external URLs during onboarding.

### Usage
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-wget https://example.com/config/device.conf \
  --command-wget https://updates.example.com/firmware.bin
```

### Parameters
- `--command-wget <url>`: URL for device to download
- Flag can be used multiple times for multiple downloads

### Download Details
- Device performs HTTP(S) GET requests to specified URLs
- Files are saved with the basename from the URL path
- Download occurs during TO2 protocol phase on the device

### Example
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-wget https://config.example.com/production/app.conf \
  --command-wget https://releases.example.com/v2.1/app-binary
```

## Combining Multiple FSIMs

Multiple FSIMs can be used together in a single onboarding session:

```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-date \
  --command-download /tmp/device-config.json \
  --upload-directory /tmp/device-reports \
  --command-upload system-status.log \
  --command-wget https://updates.example.com/latest.pkg
```

This configuration will:
1. Execute `date --utc` on device
2. Download `device-config.json` to device
3. Upload `system-status.log` from device
4. Have device download `latest.pkg` from external URL
