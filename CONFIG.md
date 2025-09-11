# Configuration File Reference

This document describes all configuration options available for the FDO server. Configuration files can use any format supported by Viper (YAML, JSON, TOML, HCL, Java properties) and the format is automatically detected based on the file extension.

Command-line arguments take precedence over configuration file values. The server address can be specified either as a command-line argument or in the configuration file under the `address` key.

Configuration files are loaded using the `--config` flag, for example:

```bash
# Using YAML
go-fdo-server manufacturing --config config.yaml

# Using JSON, over ride address given in configuration file
go-fdo-server owner --config config.json 127.0.0.1:8080

# Using TOML, enable debug logging
go-fdo-server rendezvous  --debug --config config.toml
```

## Global Configuration Options

These options are available for all server types (manufacturing, owner, rendezvous):

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `db` | string | SQLite database file path | Yes |
| `db-pass` | string | SQLite database encryption-at-rest passphrase (minimum 8 characters, must include number, uppercase letter, and special character) | Yes |
| `debug` | boolean | Print debug contents | No (default: false) |
| `insecure-tls` | boolean | Listen with a self-signed TLS certificate | No (default: false) |
| `server-cert-path` | string | Path to server certificate | No |
| `server-key-path` | string | Path to server private key | No |
| `address` | string | HTTP server address (e.g., "127.0.0.1:8080") | Yes |

## Manufacturing Server Configuration

Additional options specific to the manufacturing server:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `manufacturing-key` | string | Manufacturing private key path | Yes |
| `device-ca-cert` | string | Device certificate path | Yes |
| `device-ca-key` | string | Device CA private key path | Yes |
| `owner-cert` | string | Owner certificate path | Yes |

## Owner Server Configuration

Additional options specific to the owner server:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `device-ca-cert` | string | Device CA certificate path | Yes |
| `owner-key` | string | Owner private key path | Yes |
| `external-address` | string | External address devices should connect to (default: "127.0.0.1:${LISTEN_PORT}") | No |
| `command-date` | boolean | Use fdo.command FSIM to have device run "date --utc" | No (default: false) |
| `command-wget` | array of strings | Use fdo.wget FSIM for each URL (can be specified multiple times) | No |
| `command-upload` | array of strings | Use fdo.upload FSIM for each file (can be specified multiple times) | No |
| `upload-directory` | string | The directory path to put file uploads | No |
| `command-download` | array of strings | Use fdo.download FSIM for each file (can be specified multiple times) | No |
| `reuse-credentials` | boolean | Perform the Credential Reuse Protocol in TO2 | No (default: false) |

## Rendezvous Server Configuration

The rendezvous server only uses the global configuration options listed above.

## Configuration File Examples

### YAML Format

```yaml
# Global settings
db: "fdo-server.db"
db-pass: "MySecurePassword123!"
debug: true
insecure-tls: false
server-cert-path: "/path/to/server.crt"
server-key-path: "/path/to/server.key"
address: "127.0.0.1:8080"

# Manufacturing server specific settings
manufacturing-key: "/path/to/manufacturing.key"
device-ca-cert: "/path/to/device.ca"
device-ca-key: "/path/to/device.key"
owner-cert: "/path/to/owner.crt"

# Owner server specific settings
external-address: "0.0.0.0:8443"
command-date: true
command-wget: 
  - "https://example.com/file1"
  - "https://example.com/file2"
command-upload: 
  - "upload1.txt"
  - "upload2.txt"
upload-directory: "/tmp/uploads"
command-download: 
  - "download1.txt"
reuse-credentials: true
owner-key: "/path/to/owner.key"
```

### JSON Format

```json
{
  "db": "fdo-server.db",
  "db-pass": "MySecurePassword123!",
  "debug": true,
  "insecure-tls": false,
  "server-cert-path": "/path/to/server.crt",
  "server-key-path": "/path/to/server.key",
  "address": "127.0.0.1:8080",
  "manufacturing-key": "/path/to/manufacturing.key",
  "device-ca-cert": "/path/to/device.ca",
  "device-ca-key": "/path/to/device.key",
  "owner-cert": "/path/to/owner.crt",
  "external-address": "0.0.0.0:8443",
  "command-date": true,
  "command-wget": [
    "https://example.com/file1",
    "https://example.com/file2"
  ],
  "command-upload": [
    "upload1.txt",
    "upload2.txt"
  ],
  "upload-directory": "/tmp/uploads",
  "command-download": [
    "download1.txt"
  ],
  "reuse-credentials": true,
  "owner-key": "/path/to/owner.key"
}
```

### TOML Format

```toml
# Global settings
db = "fdo-server.db"
db-pass = "MySecurePassword123!"
debug = true
insecure-tls = false
server-cert-path = "/path/to/server.crt"
server-key-path = "/path/to/server.key"
address = "127.0.0.1:8080"

# Manufacturing server specific settings
manufacturing-key = "/path/to/manufacturing.key"
device-ca-cert = "/path/to/device.ca"
device-ca-key = "/path/to/device.key"
owner-cert = "/path/to/owner.crt"

# Owner server specific settings
external-address = "0.0.0.0:8443"
command-date = true
command-wget = [
  "https://example.com/file1",
  "https://example.com/file2"
]
command-upload = [
  "upload1.txt",
  "upload2.txt"
]
upload-directory = "/tmp/uploads"
command-download = [
  "download1.txt"
]
reuse-credentials = true
owner-key = "/path/to/owner.key"
```

## Usage


## Notes

- All file paths in the configuration should be absolute paths or paths relative to the current working directory
- The `db-pass` field has strict requirements: minimum 8 characters, must include at least one number, one uppercase letter, and one special character
- Array values (like `command-wget`, `command-upload`, `command-download`) can be specified multiple times in the configuration file
- Boolean values can be specified as `true`/`false` (YAML/JSON) or `true`/`false` (TOML)
