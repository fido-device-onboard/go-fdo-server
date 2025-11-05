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
go-fdo-server rendezvous --log-level=debug --config config.toml
```

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

The owner server also requires:
- `[device_ca]` section with `cert` field (see Device CA Configuration above)

**Note**: The `owner.cert` field is used by the manufacturing server to specify the owner certificate. The `owner.key` field is used by the owner server to specify its private key.

## Rendezvous Server Configuration

The rendezvous server configuration is under the `[rendezvous]` section:

No specific configuration options are required for the rendezvous server beyond the common HTTP and database configurations.

## Configuration File Examples

### Manufacturing Server Configuration

```toml
debug = true

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
debug = true

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
```

### Rendezvous Server Configuration

```toml
debug = true

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
debug: true

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

## Notes

- All file paths in the configuration should be absolute paths or paths relative to the current working directory
- Boolean values can be specified as `true`/`false` in TOML or `true`/`false` in YAML
- The configuration file uses a hierarchical structure where each server type has its own section
- Command-line arguments take precedence over configuration file values
- The HTTP server listen address can be overridden by providing it as a positional argument to the command (e.g., `go-fdo-server owner 127.0.0.1:8080`)
- Both `http.cert` and `http.key` MUST be provided in order to enable HTTP over TLS (HTTPS).
