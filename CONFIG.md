# Configuration File Reference

This document describes all configuration options available for the FDO server. Configuration files can use TOML or YAML format.

Command-line arguments take precedence over configuration file values. The server address can be specified either as a command-line argument or in the configuration file under the appropriate section.

Configuration files are loaded using the `--config` flag, for example:

```bash
# Using TOML configuration file
go-fdo-server manufacturing --config config.toml

# Using YAML configuration file
go-fdo-server owner --config config.yaml 127.0.0.1:8080

# Using TOML, enable debug logging
go-fdo-server rendezvous --debug --config config.toml
```

## Configuration Structure

The configuration file uses a hierarchical structure with separate sections for each server type:

- `debug` - Global debug setting
- `manufacturing` - Manufacturing server configuration
- `owner` - Owner server configuration  
- `rendezvous` - Rendezvous server configuration


## Common Configuration Sub-Types

### HTTP Configuration

HTTP configuration is used by all server types under the `[http]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `listen` | string | HTTP server listen address (e.g., "127.0.0.1:8080") | Yes |
| `ssl` | boolean | Enable SSL/TLS | No (default: false) |
| `insecure-tls` | boolean | Use self-signed TLS certificate | No (default: false) |
| `cert` | string | Path to server certificate file | No |
| `key` | string | Path to server private key file | No |

### Database Configuration

Database configuration is used by all server types under the `[database]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `path` | string | SQLite database file path | Yes |
| `password` | string | Database encryption passphrase (min 8 chars, must include number, uppercase, special char) | Yes |


## Global Configuration Options

| Key | Type | Description | Default |
|-----|------|-------------|---------|
| `debug` | boolean | Enable debug logging | false |


## Manufacturing Server Configuration

The manufacturing server configuration is under the `[manufacturing]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `private-key` | string | Manufacturing private key file path | Yes |
| `owner-cert` | string | Owner certificate file path | Yes |
| `http` | map | HTTP Server configuration | Yes |
| `database` | map | Database configuration | Yes |
| `device-ca` | map | Device CA certificate configuration | Yes |

### Device CA Configuration (`[manufacturing.device-ca]`)

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `cert` | string | Device CA certificate file path | Yes |
| `key` | string | Device CA private key file path | Yes |


## Owner Server Configuration

The owner server configuration is under the `[owner]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `external-address` | string | External address devices should connect to | No |
| `device-ca-cert` | string | Device CA certificate file path | Yes |
| `owner-key` | string | Owner private key file path | Yes |
| `reuse-credentials` | boolean | Perform the Credential Reuse Protocol in TO2 | No (default: false) |
| `http` | map | HTTP Server configuration | Yes |
| `database` | map | Database configuration | Yes |

## Rendezvous Server Configuration

The rendezvous server configuration is under the `[rendezvous]` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `http` | map | HTTP Server configuration | Yes |
| `database` | map | Database configuration | Yes |

## Configuration File Examples

### Manufacturing Server Configuration

```toml
debug = true

[manufacturing]
private-key = "/path/to/manufacturing.key"
owner-cert = "/path/to/owner.crt"

[manufacturing.http]
listen = "127.0.0.1:8038"
ssl = false
insecure-tls = false
cert = "/path/to/manufacturing.crt"
key = "/path/to/manufacturing.key"

[manufacturing.database]
path = "manufacturing.db"
password = "ManufacturingPass123!"

[manufacturing.device-ca]
cert = "/path/to/device.ca"
key = "/path/to/device.key"
```

### Owner Server Configuration

```toml
debug = true

[owner]
external-address = "0.0.0.0:8443"
device-ca-cert = "/path/to/device.ca"
owner-key = "/path/to/owner.key"
reuse-credentials = true

[owner.http]
listen = "127.0.0.1:8043"
ssl = false
insecure-tls = false
cert = "/path/to/owner.crt"
key = "/path/to/owner.key"

[owner.database]
path = "owner.db"
password = "OwnerPass123!"
```

### Rendezvous Server Configuration

```toml
debug = true

[rendezvous]

[rendezvous.http]
listen = "127.0.0.1:8041"
ssl = false
insecure-tls = false
cert = "/path/to/rendezvous.crt"
key = "/path/to/rendezvous.key"

[rendezvous.database]
path = "rendezvous.db"
password = "RendezvousPass123!"
```

### YAML Configuration Example

```yaml
debug: true

manufacturing:
  private-key: "/path/to/manufacturing.key"
  owner-cert: "/path/to/owner.crt"
  http:
    listen: "127.0.0.1:8038"
    ssl: false
    insecure-tls: false
    cert: "/path/to/manufacturing.crt"
    key: "/path/to/manufacturing.key"
  database:
    path: "manufacturing.db"
    password: "ManufacturingPass123!"
  device-ca:
    cert: "/path/to/device.ca"
    key: "/path/to/device.key"
```

## Notes

- All file paths in the configuration should be absolute paths or paths relative to the current working directory
- Database passwords have strict requirements: minimum 8 characters, must include at least one number, one uppercase letter, and one special character
- Boolean values can be specified as `true`/`false` in TOML or `true`/`false` in YAML
- The configuration file uses a hierarchical structure where each server type has its own section
- Only the relevant server section will be processed when running a specific server type (e.g., only `[manufacturing]` section is used when running the manufacturing server)
- Command-line arguments take precedence over configuration file values
- The server listen address can be overridden by providing it as a positional argument to the command
