# FDO OpenAPI Specifications

OpenAPI 3.0.3 specifications for FIDO Device Onboard (FDO) servers with shared component architecture.

## Overview

This directory contains OpenAPI specifications for FDO servers with a shared component architecture:

- **`owner-server.yaml`**: Complete OpenAPI spec for the FDO Owner Server
- **`shared/`**: Reusable components for multiple FDO servers (owner, manufacturer, rendezvous)

## Implementation Status

✅ **Owner Server**: Complete JSON API with all endpoints  
✅ **Shared Components**: Health, voucher, and common schemas ready for reuse  
✅ **Code Generation**: Uses oapi-codegen with two-step build process  
✅ **Zero Duplication**: Shared schemas defined once, referenced everywhere  
✅ **Backward Compatibility**: PEM responses via `Accept: application/x-pem-file`  

## Shared Component Architecture

### File Structure

```
api/openapi/
├── owner-server.yaml           # Owner server specification
├── shared/
│   ├── paths/
│   │   ├── health.yaml          # /health endpoint (all servers)
│   │   └── vouchers.yaml        # /vouchers endpoints (manufacturer + owner)
│   └── schemas/
│       └── common.yaml          # Shared schema definitions
├── config.yaml                 # oapi-codegen configuration
├── generate.go                 # Go generate directive
└── README.md                   # This documentation
```

### Shared Components Usage

| Endpoint | Owner | Manufacturer | Rendezvous | Purpose |
|----------|-------|--------------|-----------|---------|
| `/health` | ✅ | ✅ | ✅ | Health check for all servers |
| `/vouchers` | ✅ | ✅ | ❌ | Query vouchers |
| `/vouchers/{guid}` | ✅ | ✅ | ❌ | Get specific voucher |

| Schema | Owner | Manufacturer | Rendezvous | Purpose |
|--------|-------|--------------|-----------|---------|
| `HealthResponse` | ✅ | ✅ | ✅ | Health check response |
| `VoucherResponse` | ✅ | ✅ | ❌ | Individual voucher data |
| `VoucherMetadata` | ✅ | ✅ | ❌ | Voucher listing metadata |
| `VoucherInsertResponse` | ✅ | ✅ | ❌ | Voucher insertion results |

## Build Process

### Two-Step Generation

```bash
# 1. Generate shared types from common schemas
cd api/openapi && go tool oapi-codegen -package openapi -generate types shared/schemas/common.yaml > shared-types.go

# 2. Generate owner server code with external references resolved
go generate ./...
```

### Make Targets

```bash
# Generate all OpenAPI code
make generate

# Build entire project (includes OpenAPI generation)
make build

# Run tests including JSON API validation
go test -v ./api/handlersTest/
```

## External References

The specifications use external `$ref` to eliminate duplication:

```yaml
# owner-server.yaml references shared components
paths:
  /health:
    $ref: './shared/paths/health.yaml'
  /vouchers:
    $ref: './shared/paths/vouchers.yaml#/~1vouchers'

  /owner/vouchers:
    # Owner-specific endpoint with shared response schema
    schema:
      $ref: 'shared/schemas/common.yaml#/components/schemas/VoucherInsertResponse'
```

## Creating New Server Specifications

### Example: manufacturer-server.yaml
```yaml
openapi: 3.0.3
info:
  title: FDO Manufacturer Server API
  version: 1.1.0

paths:
  /health:
    $ref: './shared/paths/health.yaml'
  /vouchers:
    $ref: './shared/paths/vouchers.yaml#/~1vouchers'
  /vouchers/{guid}:
    $ref: './shared/paths/vouchers.yaml#/~1vouchers~1{guid}'

components:
  schemas:
    # Manufacturer-specific schemas only
    ManufacturerInfo: [...]
```

### Example: rendezvous-server.yaml
```yaml
openapi: 3.0.3
info:
  title: FDO Rendezvous Server API
  version: 1.1.0

paths:
  /health:
    $ref: './shared/paths/health.yaml'

components:
  schemas:
    # Rendezvous-specific schemas only  
    RendezvousInfo: [...]
```

## Tool Compatibility

- ✅ **oapi-codegen**: Full support with import mapping for external references
- ✅ **Two-step generation**: Shared types generated first, then server-specific code
- ✅ **External references**: Resolved via import mapping configuration
- ✅ **Zero duplication**: Each schema defined exactly once

## Testing

```bash
# Test JSON API responses
go test -v ./api/handlersTest/ -run TestJSONResponsesRequired

# Test with external references
make generate && make build

# Full CI test
make all
```

## Migration from Inline Components

The current implementation achieves zero duplication through:
1. **External file references** (`$ref: './shared/paths/health.yaml'`)
2. **Import mapping** configuration in `config.yaml`
3. **Two-step build** process ensuring shared types are available

This replaces the previous inline approach while maintaining full compatibility with oapi-codegen.