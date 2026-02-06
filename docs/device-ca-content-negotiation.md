# Device CA API Content Negotiation

The Device CA API endpoints support content negotiation based on the `Accept` HTTP header, allowing clients to choose between JSON and PEM response formats.

## Supported Endpoints

### GET /api/v1/device-ca

**List all trusted device CA certificates**

- **Accept: application/json** (default) - Returns certificate metadata as JSON
- **Accept: application/x-pem-file** - Returns certificates as concatenated PEM file

**Examples:**

```bash
# Get certificates as JSON (default)
curl -H "Accept: application/json" http://localhost:8080/api/v1/device-ca

# Get certificates as PEM file
curl -H "Accept: application/x-pem-file" http://localhost:8080/api/v1/device-ca

# No Accept header defaults to JSON
curl http://localhost:8080/api/v1/device-ca
```

### GET /api/v1/device-ca/{fingerprint}

**Get a specific trusted device CA certificate by fingerprint**

- **Accept: application/json** (default) - Returns certificate metadata as JSON
- **Accept: application/x-pem-file** - Returns certificate as PEM file

**Examples:**

```bash
# Get certificate as JSON (default)
curl -H "Accept: application/json" \
  http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0

# Get certificate as PEM file
curl -H "Accept: application/x-pem-file" \
  http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0

# No Accept header defaults to JSON
curl http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0
```

## Default Behavior

When no `Accept` header is provided, both endpoints default to `application/json`:
- **ListTrustedDeviceCACerts** defaults to `application/json`
- **GetTrustedDeviceCACertByFingerprint** defaults to `application/json`

This provides a consistent JSON-first API experience. Clients can explicitly request `application/x-pem-file` when they need the raw PEM certificate format.

## Implementation Details

Content negotiation is implemented using a middleware that follows RFC 7231 standards:
1. Extracts the `Accept` header from the request
2. Uses the `github.com/elnormous/contenttype` library for proper content negotiation
3. Determines the preferred content type based on quality factors (q values)
4. Stores the preferred type in the request context
5. The handler reads from context and formats the response accordingly

### RFC 7231 Compliance

The middleware properly handles:
- **Simple Accept headers**: `application/json`, `application/x-pem-file`
- **Quality parameters**: `application/json;q=0.9, application/x-pem-file;q=1.0`
  - Higher q values take precedence
  - Example: `application/json;q=0.8, application/x-pem-file;q=0.9` → returns PEM
- **Multiple types**: `application/json, application/x-pem-file`
  - When no q values specified, all have equal preference (q=1.0)
  - The library uses specificity rules to select the best match
- **Wildcards**: `*/*` (defaults to JSON for all endpoints)
- **Unsupported types**: Falls back to default (`application/json`)

### Example Quality Factor Scenarios

```bash
# Higher quality for PEM → returns PEM
curl -H "Accept: application/json;q=0.8, application/x-pem-file;q=0.9" \
  http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0

# Higher quality for JSON → returns JSON
curl -H "Accept: application/json;q=0.9, application/x-pem-file;q=0.5" \
  http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0

# Equal quality (both default to q=1.0) → returns first acceptable (JSON preferred)
curl -H "Accept: application/json, application/x-pem-file" \
  http://localhost:8080/api/v1/device-ca/a1b2c3d4e5f67890123456789abcdef0a1b2c3d4e5f67890123456789abcdef0
```
