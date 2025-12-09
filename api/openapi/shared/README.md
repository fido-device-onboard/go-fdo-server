# Shared OpenAPI Components

This directory contains OpenAPI components that are shared across multiple FDO servers:

- **owner server**: All endpoints and schemas
- **manufacturer server**: `/health` + `/vouchers/*` endpoints + shared schemas
- **rendezvous server**: `/health` endpoint + HealthResponse schema

## File Structure

```
shared/
├── paths/
│   ├── health.yaml          # /health endpoint (all servers)
│   └── vouchers.yaml        # /vouchers endpoints (manufacturer + owner)
├── schemas/
│   └── common.yaml          # Shared schema definitions
└── README.md               # This documentation
```

## Shared Components

### Endpoints Used by Multiple Servers

| Endpoint | Owner | Manufacturer | Rendezvous | Purpose |
|----------|-------|--------------|-----------|---------|
| `/health` | ✅ | ✅ | ✅ | Health check for all servers |
| `/vouchers` | ✅ | ✅ | ❌ | Query vouchers |
| `/vouchers/{guid}` | ✅ | ✅ | ❌ | Get specific voucher |

### Shared Schemas

| Schema | Owner | Manufacturer | Rendezvous | Purpose |
|--------|-------|--------------|-----------|---------|
| `HealthResponse` | ✅ | ✅ | ✅ | Health check response |
| `VoucherResponse` | ✅ | ✅ | ❌ | Individual voucher data |
| `VoucherMetadata` | ✅ | ✅ | ❌ | Voucher listing metadata |
| `VoucherInsertResponse` | ✅ | ✅ | ❌ | Voucher insertion results |

## Usage for Future Server Specs

When creating `manufacturer-server.yaml` or `rendezvous-server.yaml`, reference these shared components:

### Example: manufacturer-server.yaml
```yaml
openapi: 3.0.3
info:
  title: FDO Manufacturer Server API
  version: 1.1.0

paths:
  /health:
    $ref: 'shared/paths/health.yaml'
  
  /vouchers:
    $ref: 'shared/paths/vouchers.yaml#/~1vouchers'
  
  /vouchers/{guid}:
    $ref: 'shared/paths/vouchers.yaml#/~1vouchers~1{guid}'

components:
  schemas:
    HealthResponse:
      $ref: 'shared/schemas/common.yaml#/HealthResponse'
    VoucherResponse:
      $ref: 'shared/schemas/common.yaml#/VoucherResponse'
    # ... manufacturer-specific schemas
```

### Example: rendezvous-server.yaml
```yaml
openapi: 3.0.3
info:
  title: FDO Rendezvous Server API  
  version: 1.1.0

paths:
  /health:
    $ref: 'shared/paths/health.yaml'

components:
  schemas:
    HealthResponse:
      $ref: 'shared/schemas/common.yaml#/HealthResponse'
    # ... rendezvous-specific schemas
```

## Current Implementation Note

The current `owner-server.yaml` includes all components inline to maintain compatibility with `oapi-codegen` which has limited support for external file references. The shared components are clearly marked with comments for easy identification and extraction.

## Tool Compatibility

- ✅ **oapi-codegen**: Works with inline schemas (current approach)
- ⚠️ **External refs**: Requires additional tooling or import mapping
- ✅ **Swagger UI**: Can display both inline and external references
- ✅ **Future extraction**: Comments clearly mark shared vs owner-specific components