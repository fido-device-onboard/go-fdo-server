# Configuration File Reference

This document describes all configuration options available for the FDO server. Configuration files can use TOML or YAML format.

Command-line arguments take precedence over configuration file values. The server address can be specified either as a command-line argument or in the configuration file under the appropriate section.

## Configuration File Location

The configuration file can be specified via the `--config` server command line parameter, for example:

```bash
# Using TOML configuration file:
go-fdo-server manufacturing --config /etc/config.toml

# Using YAML configuration file in the local directory with listening address override:
go-fdo-server owner --config config.yaml 127.0.0.1:8080

# Using TOML, enable debug logging
go-fdo-server rendezvous --log-level=debug --config /home/fdo/config.toml
```

If `--config` is not provided the server will search the following directories in order until a configuration file is found:

- `$HOME/.config/go-fdo-server/`
- `/etc/go-fdo-server/`
- `/usr/share/go-fdo-server/`

The name of the configuration file is based on the server's role, with the file name suffix corresponding to the file format:

| Role | Filename | Examples |
|------|----------|----------|
| Manufacturer | `manufacturing.<suffix>` | `manufacturing.yaml`, `manufacturing.toml` |
| Owner | `owner.<suffix>` | `owner.yaml`, `owner.toml` |
| Rendezvous | `rendezvous.<suffix>` | `rendezvous.yaml`, `rendezvous.toml` |


## Configuration Structure

The configuration file uses a hierarchical structure that defines the following sections:

- `log` - Logging level configuration
- `db` - Database configuration
- `http` - HTTP server configuration
- `device_ca` - Device Certificate Authority configuration
- `manufacturing` - Manufacturing server-specific configuration
- `owner` - Owner server-specific configuration
- `rendezvous` - Rendezvous server-specific configuration

## Logging Configuration

| Key | Type | Description | Default |
|-----|------|-------------|---------|
| `level` | string | Set the logging level. Allowed values: "debug", "info", "warn", or "error" | info |

## Database Configuration

A database is used to persist server state and is required for all
server roles. The database configuration is provided under the `[db]`
section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `type` | string | Database type (e.g., "sqlite", "postgres") | Yes |
| `dsn` | string | Database connection string (e.g., `file:database.db` for SQLite, `host=localhost port=5432 user=postgres password=secret dbname=mydb` for PostgreSQL) | Yes |

## HTTP Server Configuration

All servers provide an HTTP endpoint. The HTTP server configuration is
provided under the `[http]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `ip` | string | HTTP server IP address or hostname | Yes |
| `port` | string | HTTP server port | Yes |
| `cert` | string | Path to server certificate file | No |
| `key` | string | Path to server private key file | No |

**Note**: HTTPS (TLS) is automatically enabled when both `cert` and `key` are provided.

## Device CA Configuration

The Device Certificate Authority configuration is under the `[device_ca]` section. This section is required for both manufacturing and owner servers:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `cert` | string | Device CA certificate file path | Yes |
| `key` | string | Device CA private key file path | Yes (for manufacturing server) |

**Note**: For the owner server, only the `cert` field is required. The `key` field is only needed for the manufacturing server.

## Manufacturing Server Configuration

The manufacturing server configuration is under the `[manufacturing]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `key` | string | Manufacturing private key file path | Yes |

The manufacturing server also requires:
- `[device_ca]` section with both `cert` and `key` (see Device CA Configuration above)
- `[owner]` section with `cert` field (see Owner Configuration below)

## Owner Server Configuration

The owner server configuration is under the `[owner]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `cert` | string | Owner certificate file path | Yes (for manufacturing server) |
| `key` | string | Owner private key file path | Yes (for owner server) |
| `reuse_credentials` | boolean | Perform the Credential Reuse Protocol in TO2 | No (default: false) |
| `to0_insecure_tls` | boolean | Skip TLS certificate verification for TO0 | No (default: false) |
| `service_info` | map | ServiceInfo Modules to execute on device onboarding (See below) | No |

The owner server also requires:
- `[device_ca]` section with `cert` field (see Device CA Configuration above)

**Note**: The `owner.cert` field is used by the manufacturing server to specify the owner certificate. The `owner.key` field is used by the owner server to specify its private key.

### Service Info Configuration (FSIM Operations)

The owner server can be configured to execute FSIM (FDO Service Info Module) operations during device onboarding. FSIM operations are defined as an ordered list `fsims` under the `service_info` field within the `[owner]` section. Each list entry contains the name of the FSIM operation to perform and parameters to pass to the operation. FSIM Operations may be listed in any order but will be executed on the device in the order they appear in the list.

### Supported FSIM Modules

The following FSIM modules are supported:

1. **fdo.command** - Execute commands on the device
2. **fdo.download** - Download files from the owner server to the device
3. **fdo.upload** - Upload files from the device to the owner server
4. **fdo.wget** - Instruct the device to download files from specified URLs

### Service Info Operation Structure

Each operation in the `service_info.fsims` list has the following structure:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `fsim` | string | The FSIM module type (one of: "fdo.command", "fdo.download", "fdo.upload", "fdo.wget") | Yes |
| `params` | object | Parameters for the FSIM module (structure depends on the fsim type) | Yes |

### Service Info Defaults

The `service_info` configuration supports an optional `defaults` section that allows you to specify default directory values for FSIM operations. This reduces repetition when multiple operations use the same directories.

The `defaults` field is a list of default entries with the following structure:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `fsim` | string | The FSIM module type (one of: "fdo.download", "fdo.upload", "fdo.wget") | Yes |
| `dir` | string | Default directory path (must be absolute) | Yes |

**Important notes:**
- Each `fsim` value can appear only once in the defaults list (maximum of 3 entries)
- The `dir` path must be absolute
- For `fdo.download` and `fdo.upload`, the directory must exist on the owner server at startup
- For `fdo.wget`, the directory is on the device (existence is not checked at startup)
- Defaults can be overridden by specifying `params.dir` in individual FSIM operations
- If neither a default nor `params.dir` is specified, the current working directory is used

#### Defaults Example

```yaml
service_info:
  defaults:
    - fsim: "fdo.download"
      dir: "/var/lib/go-fdo-server-owner/downloads"
    - fsim: "fdo.upload"
      dir: "/var/lib/go-fdo-server-owner/uploads"
    - fsim: "fdo.wget"
      dir: "/var/lib/device/wget/files"
  fsims:
    - fsim: "fdo.download"
      params:
        # dir not specified - uses default from above
        files:
          - src: "app.tar.gz"
            dst: "/tmp/app.tar.gz"
    - fsim: "fdo.upload"
      params:
        dir: "/custom/upload/path"  # Override default
        files:
          - src: "/var/log/syslog"
            dst: "device-syslog.log"
```

### fdo.command Parameters

Execute commands on the device.

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `cmd` | string | The command processor to execute (e.g., "sh", "bash", "cmd"). | Yes |
| `args` | array of strings | Command arguments | No |
| `may_fail` | boolean | If true, allow the command to fail without aborting onboarding | No (default: false) |
| `return_stdout` | boolean | If true, the device's stdout stream from the command will be sent to the owner server and written to the logs | No (default: false) |
| `return_stderr` | boolean | If true, the device's stderr stream from the command will be sent to the owner server and written to the logs | No (default: false) |

### fdo.command Example

```yaml
fsim: "fdo.command"
params:
  may_fail: false
  return_stdout: true
  cmd: "bash"
  args:
    - "-c"
    - |
      #! /bin/bash
      set -xeuo pipefail
      echo "Current Date:"
      date
      dmidecode --quiet --dump-bin /var/lib/fdo/upload/dmidecode
```

### fdo.download Parameters

Download files from the owner server to the device.

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `dir` | string | Base directory path on the owner server where source files are located (used when `files.src` is relative). If not specified, uses the default from `service_info.defaults` or the owner server's current working directory. | No |
| `files` | array of objects | List of files to download | Yes |

Each file object in the `files` array has:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `src` | string | Path to the file on the owner server. Can be absolute (ignores `params.dir`) or relative (appended to `params.dir`). | Yes |
| `dst` | string | Destination path on the device. Can be absolute or relative (to device working directory). | Yes |
| `may_fail` | boolean | If true, allow the download to fail without aborting onboarding | No (default: false) |

### fdo.download Example

```yaml
fsim: "fdo.download"
params:
  dir: "/var/lib/fdo/downloads"
  files:
    - src: "configs/app-config.json"  # relative to dir, file at /var/lib/fdo/downloads/configs/app-config.json
      dst: "/etc/myapp/config.json"  # absolute path on device
    - src: "/opt/scripts/setup.sh"  # absolute path, ignores dir
      dst: "setup.sh"  # relative to device working directory
      may_fail: true  # this file download is optional
```

### fdo.upload Parameters

Upload files from the device to the owner server. Files are uploaded to a per-device directory on the owner server. The name of the directory is the device's replacement GUID (the GUID that is set after onboarding completes). This prevents files with the same name from being overwritten as devices are onboarded.

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `dir` | string | Absolute path to a directory on the owner server where uploaded files will be stored. A per-device subdirectory is created in this directory for each device that uploads files during onboarding. If not set the directory from the `fdo.upload` entry in `service_info.defaults` is used. | No |
| `files` | array of objects | List of files to request from the device | Yes |

Each file object in the `files` array has:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `src` | string | Path to the file on the device to upload. Can be absolute or relative (to device working directory). | Yes |
| `dst` | string | Destination path on the owner server. If omitted the basename of `src` will be used. Must be a relative path (appended to `params.dir`/$GUID/). | No |

### fdo.upload Example

```yaml
fsim: "fdo.upload"
params:
  dir: "/var/lib/fdo/uploads"
  files:
    - src: "/etc/hostname"
      dst: "device-hostname.txt"  # saved to /var/lib/fdo/uploads/$GUID/device-hostname.txt
    - src: "/var/log/device.log"
      dst: "logs/device-12345.log"  # saved to /var/lib/fdo/uploads/$GUID/logs/device-12345.log
    - src: "/sys/class/dmi/id/product_uuid"
      dst: "system-info/uuid"  # saved to /var/lib/fdo/uploads/$GUID/system-info/uuid
    - src: "/etc/machine-id"
      # dst omitted - saved to /var/lib/fdo/uploads/$GUID/machine-id
```

### fdo.wget Parameters

Instruct the device to download content from an HTTP server.

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `dir` | string | Absolute path to a directory on the device where files will be downloaded. If not specified, uses the default from `service_info.defaults` or the device's current working directory. Used as base directory for relative `files.dst` paths. | No |
| `files` | array of objects | List of URLs that the device will retrieve content from and the file paths where the content will be stored. | Yes |

Each file object in the `files` array has:

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `url` | string | URL to download from (scheme must be `http` or `https`) | Yes |
| `dst` | string | Destination filename on the device for the retrieved content. If omitted the basename of the URL path is used. Can be an absolute path or relative (joined with `dir` if specified).  | No |
| `length` | integer | For validation: expected size of downloaded content in bytes | No |
| `checksum` | string | For validation: Expected SHA-384 checksum of the file (96 hexadecimal characters) | No |

### fdo.wget Example

```yaml
fsim: "fdo.wget"
params:
  dir: "/root/downloads"
  files:
    - url: "https://example.com/packages/app-v1.2.3.rpm"
      dst: "/tmp/app.rpm"
      length: 2048576
      checksum: "a1b2c3d4e5f..."
    - url: "https://cdn.example.com/updates/firmware.bin"
    - url: "https://cdn.example.com/updates/license.txt"
      dst: "license.txt"
```

For the example above the first download will be saved to `/tmp/app.rpm`, the second to `/root/downloads/firmware.bin` and the last to `/root/downloads/license.txt`.

## Rendezvous Server Configuration

The rendezvous server configuration is under the `[rendezvous]` section:

No specific configuration options are required for the rendezvous server beyond the common HTTP and database configurations.

## Configuration File Examples

### Manufacturing Server Configuration

```toml
[log]
level = "debug"

[http]
ip = "127.0.0.1"
port = "8038"
cert = "/path/to/manufacturing.crt"
key = "/path/to/manufacturing.key"

[db]
type = "sqlite"
dsn = "file:manufacturing.db"

[manufacturing]
key = "/path/to/manufacturing.key"

[device_ca]
cert = "/path/to/device.ca"
key = "/path/to/device.key"

[owner]
cert = "/path/to/owner.crt"
```

### Owner Server Configuration

```toml
[log]
level = "debug"

[http]
ip = "127.0.0.1"
port = "8043"
cert = "/path/to/owner.crt"
key = "/path/to/owner.key"

[db]
type = "postgres"
dsn = "host=localhost user=owner password=Passw0rd dbname=owner port=5432 sslmode=disable TimeZone=Europe/Madrid"

[device_ca]
cert = "/path/to/device.ca"

[owner]
key = "/path/to/owner.key"
reuse_credentials = true
to0_insecure_tls = false

# Example FSIM operations configuration
[owner.service_info]
[[owner.service_info.fsims]]
fsim = "fdo.command"
[owner.service_info.fsims.params]
cmd = "sh"
args = ["-c", "echo Current date: ; date"]
return_stdout = true

[[owner.service_info.fsims]]
fsim = "fdo.download"
[owner.service_info.fsims.params]
dir = "/var/lib/fdo/downloads"
[[owner.service_info.fsims.params.files]]
src = "/path/to/file1.txt"
dst = "config.txt"
may_fail = false

[[owner.service_info.fsims]]
fsim = "fdo.upload"
[owner.service_info.fsims.params]
dir = "/var/lib/fdo/uploads"
[[owner.service_info.fsims.params.files]]
src = "/etc/device-info.txt"
dst = "info/device-info.txt"

[[owner.service_info.fsims]]
fsim = "fdo.wget"
[[owner.service_info.fsims.params.files]]
url = "https://example.com/package.tar.gz"
dst = "package.tar.gz"
checksum = "abc123..."
```

### Rendezvous Server Configuration

```toml
[log]
level = "debug"

[http]
ip = "127.0.0.1"
port = "8041"
cert = "/path/to/rendezvous.crt"
key = "/path/to/rendezvous.key"

[db]
type = "sqlite"
dsn = "file:rendezvous.db"

[rendezvous]
```

### YAML Configuration Example

```yaml
log:
  level: "debug"

http:
  ip: "127.0.0.1"
  port: "8038"
  cert: "/path/to/manufacturing.crt"
  key: "/path/to/manufacturing.key"

db:
  type: "sqlite"
  dsn: "file:manufacturing.db"

manufacturing:
  key: "/path/to/manufacturing.key"

device_ca:
  cert: "/path/to/device.ca"
  key: "/path/to/device.key"

owner:
  cert: "/path/to/owner.crt"
```

### Owner Server with FSIM Configuration (YAML)

```yaml
log:
  level: "debug"

http:
  ip: "127.0.0.1"
  port: "8043"

db:
  type: "sqlite"
  dsn: "file:owner.db"

device_ca:
  cert: "/path/to/device.ca"

owner:
  key: "/path/to/owner.key"
  reuse_credentials: true
  service_info:
    fsims:
      - fsim: "fdo.command"
        params:
          cmd: "sh"
          args: ["-c", "echo Current date: ; date"]
          return_stdout: true

      - fsim: "fdo.download"
        params:
          dir: "/var/lib/fdo/downloads"
          files:
            - src: "/path/to/file1.txt"
              dst: "config.txt"
              may_fail: false
            - src: "/path/to/file2.txt"
              dst: "data.txt"

      - fsim: "fdo.upload"
        params:
          dir: "/var/lib/fdo/uploads"
          files:
            - src: "/etc/device-info.txt"
              dst: "info/device-info.txt"

      - fsim: "fdo.wget"
        params:
          files:
            - url: "https://example.com/package.tar.gz"
              dst: "package.tar.gz"
```

## Notes

- All file paths in the configuration should be absolute paths or paths relative to the current working directory
- Boolean values can be specified as `true`/`false` in TOML or `true`/`false` in YAML
- The configuration file uses a hierarchical structure where each server type has its own section
- Command-line arguments take precedence over configuration file values
- The HTTP server listen address can be overridden by providing it as a positional argument to the command (e.g., `go-fdo-server owner 127.0.0.1:8080`)
- Both `http.cert` and `http.key` MUST be provided in order to enable HTTP over TLS (HTTPS).
