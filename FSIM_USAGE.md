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
**As of now only one command is available. This is for testing purposes, and the list of commands will be expanded once testing is complete.**
- `--command-date`: Executes `date --utc` on the device to display current UTC time

### Device-Side Requirements
The device must be started with the `--echo-commands` option to enable command execution:
```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin --echo-commands
```

### Example Output
When a device onboards with the command FSIM enabled, the device will execute the specified command and the output will be displayed in the owner service logs.

### Security Considerations
- Commands run with the privileges of the device onboard process
- Only predefined safe commands are supported
- Command output is captured and logged
- Device must explicitly enable command execution with `--echo-commands` for security

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

### Device-Side Requirements
The device must specify a directory for downloaded files using the `--download` option:
```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin --download /tmp/downloads
```

### File Transfer Details
- Files are read from the owner server's local filesystem
- Downloaded files are named using the same name given in the `--command-download` filepath (e.g., "file1.txt", "file2.conf")
- Transfer is mandatory (MustDownload: true)

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

### Security Considerations
- Download directory permissions should be appropriately configured
- Validate downloaded file contents and integrity after transfer

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

### Device-Side Requirements
The device must specify the relative paths to directories containing uploadable files using the `--upload` parameter:
```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin --upload /var/log,/etc/device
```

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

### Security Considerations
- Upload directory permissions should be restrictive
- Validate uploaded file contents before processing
- Monitor disk space in upload directory
- Device controls which directories are available for upload via `--upload` parameter

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
- URLs must be valid and include a filename in the path

### Device-Side Requirements
The device must specify a download directory using the `--wget-dir` option:
```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin --wget-dir /tmp/downloads
```

### Network Connection Details
- **Device opens a new TCP connection** to the HTTP server for each download
- Device performs HTTP(S) GET requests to specified URLs
- Files retain the filenames given in the `--command-wget` argument (basename from URL path)

### Example
```bash
go-fdo-server owner 127.0.0.1:8043 \
  --db /tmp/fdo/db/own.db --db-pass "$DB_PASS" \
  --device-ca-cert /tmp/fdo/keys/device_ca_cert.pem \
  --owner-key /tmp/fdo/keys/owner_key.der \
  --command-wget https://config.example.com/production/app.conf \
  --command-wget https://releases.example.com/v2.1/app-binary
```

### Security Considerations
- **Network Security**: Device opens new TCP connections to external servers, ensure firewall rules allow this
- Device must have network access to specified URLs
- URLs should use HTTPS for secure transfer
- Verify URL sources are trusted before configuration
- **Download Location**: Device controls download directory via `--wget-dir` parameter
- Monitor network traffic and validate downloaded file integrity

## Combining Multiple FSIMs

**This section is under development. While some of the following information about how to combine FSIMs may be useful, there are no guarantees about its accuracy. This section will be updated as further changes to simultaneous FSIM usage are implemented.**

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

### Device Configuration for Combined FSIMs
When using multiple FSIMs, the device must be configured with all required parameters:
```bash
go-fdo-client onboard --key ec256 --kex ECDH256 --debug --blob /tmp/fdo/cred.bin \
  --echo-commands \
  --download /tmp/downloads \
  --upload /var/log,/etc/device \
  --wget-dir /tmp/downloads
```
