# FDO 2.0 Support

This server supports both FDO 1.1 and FDO 2.0 protocols simultaneously.

## Dual Protocol Support

The server automatically handles both protocol versions on different HTTP paths:

```
POST /fdo/101/msg/{msgType}  # FDO 1.1 protocol
POST /fdo/200/msg/{msgType}  # FDO 2.0 protocol
```

Clients select the protocol version by using the appropriate path.

## FDO 2.0 Features

### Message Types
FDO 2.0 uses different message type ranges:
- **TO0**: Message types 20-23 (instead of 30-33 in v1.1)
- **TO1**: Message types 30-33 (instead of 40-43 in v1.1)  
- **TO2**: Message types 80-91 (instead of 60-71 in v1.1)

### Capability Flags
The server advertises FDO 2.0 support via capability flags during TO1:
- `Capb0SupFDO11`: Supports FDO 1.1
- `Capb0SupFDO20`: Supports FDO 2.0

This allows clients to negotiate the highest common protocol version.

### Delegation Protocol
FDO 2.0 introduces delegation for distributed ownership:

#### Configuration
Configure delegation via the owner service:

1. **Add Delegate Keys** (Management API):
```bash
curl -X POST https://owner.example.com/api/v1/owner/delegate-keys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "delegate-owner",
    "certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "privateKey": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
  }'
```

2. **Set Delegation Name** (Environment or Config):
```yaml
owner:
  delegate_name: "delegate-owner"  # Name of delegate to use for TO0
```

#### How It Works
When delegation is configured:
- TO0: Owner signs with delegate key, includes `DelegateChain` in OwnerSign
- TO1: Device validates delegation chain against root owner key
- TO2: Ownership transfer proceeds with delegated authority

#### Delegation Permissions
Delegate certificates can include OID extensions limiting permissions:
- TO0 only (rendezvous registration)
- TO1 only (device discovery)
- TO2 only (ownership transfer)
- Any combination

Default: No OID extension = all permissions

### Hash Binding Chain
FDO 2.0 implements anti-replay protection via hash binding:
- `HashPrev`: Hash of previous TO2 message (n-1)
- `HashPrev2`: Hash of message n-2

The server validates the hash chain to prevent message replay attacks.

### Additional Authenticated Data (AAD)
FDO 2.0 uses AAD for COSE signatures to prevent signature substitution attacks.
The server validates AAD in all signed messages.

## Cross-Version Compatibility

The server supports all protocol version combinations:
- FDO 1.1 client → Server (via `/fdo/101/msg/`)
- FDO 2.0 client → Server (via `/fdo/200/msg/`)

Version negotiation:
1. Client selects version by choosing HTTP path
2. TO1 capability flags confirm mutual support
3. Protocol proceeds with selected version

## Configuration Examples

### Basic FDO 2.0 Server

```yaml
owner:
  # Delegation is optional
  # delegate_name: ""  # Empty = no delegation

rendezvous:
  # Both protocols enabled by default
```

### Server with Delegation

```yaml
owner:
  delegate_name: "delegate-owner"
  
  # Delegate keys loaded via Management API
  # or filesystem (implementation-specific)
```

### Environment Variables

```bash
# Optional delegation configuration
export FDO_OWNER_DELEGATE_NAME="delegate-owner"

# Start server
./go-fdo-server owner --config config.yaml
```

## Delegation Management API

### Add Delegate Key
```http
POST /api/v1/owner/delegate-keys
Content-Type: application/json

{
  "name": "delegate-owner",
  "certificate": "...",
  "privateKey": "...",
  "permissions": ["TO0", "TO1"]  // Optional
}
```

### List Delegate Keys
```http
GET /api/v1/owner/delegate-keys
```

Response:
```json
[
  {
    "name": "delegate-owner",
    "certificate": "...",
    "permissions": ["TO0", "TO1"]
  }
]
```

### Delete Delegate Key
```http
DELETE /api/v1/owner/delegate-keys/{name}
```

### Set Active Delegate
```http
PUT /api/v1/owner/delegate
Content-Type: application/json

{
  "name": "delegate-owner"  // or "" to disable
}
```

## Monitoring

### Protocol Version Metrics
The server logs protocol version usage:
```
[INFO] TO0 request: version=200, guid=a1b2c3...
[INFO] TO1 request: version=101, guid=d4e5f6...
```

### Delegation Metrics
When delegation is active:
```
[INFO] TO0 using delegation: delegate=delegate-owner
[DEBUG] Delegation chain length: 1
```

## Debugging

Enable debug logging to see protocol details:

```bash
./go-fdo-server owner --log-level debug
```

Look for:
- Protocol version in request path (`/fdo/200/msg/`)
- Capability flags in TO1 messages
- Delegation chain validation
- Hash binding validation

## Migration from FDO 1.1

The server maintains backward compatibility:
1. Existing FDO 1.1 clients continue working on `/fdo/101/msg/`
2. New FDO 2.0 clients use `/fdo/200/msg/`
3. No configuration changes required for basic upgrade

To enable delegation (FDO 2.0 feature):
1. Generate delegate keys
2. Upload via Management API
3. Set `delegate_name` in configuration
4. Delegation applies only to FDO 2.0 flows

## Reference

- [FDO 2.0 Specification](https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-WD-v2.0-20250617/)
- [FDO 1.1 Specification](https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/)
- [go-fdo library](https://github.com/fido-device-onboard/go-fdo)
- [Management API Documentation](./README.md#management-api)
