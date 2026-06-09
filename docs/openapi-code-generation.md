# OpenAPI Code Generation Guide

This document describes how Go server code and OpenAPI documentation are automatically generated from OpenAPI specifications in this project.

## Overview

The FDO server uses a two-phase code generation approach:

1. **Go Server Code Generation**: Uses [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) to generate Go server interfaces, handlers, and models from modular OpenAPI definitions
2. **Standalone OpenAPI Documentation**: Uses [openapi-format](https://github.com/thim81/openapi-format) to resolve `$ref` references and produce complete, standalone OpenAPI documents for each server role

This approach provides:
- Type-safe Go code with compile-time validation
- Modular, reusable OpenAPI definitions
- Complete OpenAPI specs for API documentation and client generation
- Automatic synchronization between code and documentation

## Directory Structure

The API code is organized by version (`v1`, `v2`). Each endpoint package contains its OpenAPI definition, oapi-codegen configuration, generated code, and handler implementation:

```
.
├── api/
│   ├── v1/                        # V1 API (deprecated, backward-compatible)
│   │   ├── components/
│   │   │   ├── openapi.yaml       # Common schemas, security, responses
│   │   │   ├── config.yaml        # oapi-codegen configuration
│   │   │   └── models.gen.go      # Generated common models
│   │   ├── health/
│   │   │   ├── openapi.yaml       # Health check endpoint definition
│   │   │   ├── config.yaml        # oapi-codegen configuration
│   │   │   ├── handler.gen.go     # Generated handler interfaces
│   │   │   └── handler.go         # Handler implementation
│   │   ├── deviceca/              # Device CA certificates API
│   │   ├── voucher/               # Ownership voucher API
│   │   ├── rvinfo/                # Rendezvous info API
│   │   └── ...                    # Other V1 endpoints
│   └── v2/                        # V2 API (current)
│       ├── components/
│       │   ├── openapi.yaml       # Common schemas
│       │   ├── config.yaml        # oapi-codegen configuration
│       │   └── handler.gen.go     # Generated common models
│       ├── health/                # Health check endpoint
│       ├── deviceca/              # Device CA certificates API
│       ├── voucher/               # Ownership voucher API (enhanced)
│       ├── rvinfo/                # Rendezvous info API
│       ├── rvto2addr/             # RV TO2 address API (new in V2)
│       ├── manufacturer/
│       │   ├── handler.go         # Manufacturer server wiring
│       │   ├── openapi.json       # Complete spec (generated)
│       │   └── openapi.yaml       # Complete spec (generated)
│       ├── owner/
│       │   ├── handler.go         # Owner server wiring
│       │   ├── openapi.json       # Complete spec (generated)
│       │   └── openapi.yaml       # Complete spec (generated)
│       └── rendezvous/
│           ├── handler.go         # Rendezvous server wiring
│           ├── openapi.json       # Complete spec (generated)
│           └── openapi.yaml       # Complete spec (generated)
```

## Code Generation Process

### Phase 1: Go Server Code Generation

**Tool**: [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2.5.1+

**Input**: Modular OpenAPI definitions in `api/v{1,2}/*/openapi.yaml`

**Output**: Go server interfaces and models in `api/v{1,2}/*/handler.gen.go`

**Process**:
1. Each OpenAPI definition file has a corresponding `config.yaml` in the same directory
2. Configuration specifies what to generate (models, servers, strict interfaces)
3. `oapi-codegen` reads the OpenAPI spec and config, generates Go code
4. Common types from `../components/openapi.yaml` are imported via `import-mapping`

**Generated Code Includes**:
- Type definitions for request/response schemas
- Server interface definitions with strongly-typed methods
- Strict server wrappers for enhanced type safety
- Request parameter structs
- OpenAPI spec embedding

### Phase 2: Standalone OpenAPI Documentation

**Tool**: [openapi-format](https://github.com/thim81/openapi-format) via npx

**Input**: Aggregation specs in `api/v2/{manufacturer,owner,rendezvous}/`

**Output**: Complete OpenAPI documents in `api/v2/{manufacturer,owner,rendezvous}/openapi.json`

**Process**:
1. Aggregation specs use `$ref` to compose endpoints from modular definitions
2. `openapi-format` resolves all `$ref` references
3. Produces standalone, self-contained OpenAPI documents
4. These documents can be used with Swagger UI, client generators, etc.

## Configuration Files

### oapi-codegen Configuration

Each endpoint has a `config.yaml` alongside its `openapi.yaml`. Example (`api/v2/voucher/config.yaml`):

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/HEAD/configuration-schema.json
package: voucher                    # Go package name
generate:
  std-http-server: true             # Generate standard http.Handler interfaces
  strict-server: true               # Generate strict server wrapper
  models: true                      # Generate request/response models
output: handler.gen.go              # Output file path (relative to config)
import-mapping:                     # Import shared types from other packages
  ../components/openapi.yaml: "github.com/fido-device-onboard/go-fdo-server/api/v2/components"
output-options:
  skip-prune: true                  # Generate all types, even unused ones
```

#### Configuration Options Explained

- **package**: Name of the generated Go package
- **generate.std-http-server**: Creates `ServerInterface` with `http.Handler` methods
- **generate.strict-server**: Creates `StrictServerInterface` with typed request/response
- **generate.models**: Generates Go structs for all schemas
- **import-mapping**: Maps OpenAPI file references to Go import paths
- **output-options.skip-prune**: Ensures all types are generated, not just referenced ones

### Components Configuration

`api/v2/components/config.yaml` generates shared models (the V1 equivalent outputs to `models.gen.go` instead of `handler.gen.go`):

```yaml
package: components
generate:
  models: true                      # Generate shared model types
  strict-server: true               # Generate strict server response types
output: handler.gen.go
output-options:
  skip-prune: true
```

This creates a shared package with common types (Error, RVInfo, etc.) that other handlers import.

## Implementing a Server Role

A server role (Manufacturer, Owner, Rendezvous) typically combines multiple management APIs. Here's how the Rendezvous server wires together multiple APIs as a complete example.

### Example: Rendezvous Server Implementation

The Rendezvous server combines:
- FDO protocol handlers (TO0, TO1)
- Health check API
- Device CA management API

**File: `api/v2/rendezvous/handler.go`** (simplified for illustration)

```go
package rendezvous

import (
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/elnormous/contenttype"
	fdo_lib "github.com/fido-device-onboard/go-fdo"
	v1deviceca "github.com/fido-device-onboard/go-fdo-server/api/v1/deviceca"
	v2deviceca "github.com/fido-device-onboard/go-fdo-server/api/v2/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/api/v2/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
)

// Rendezvous handles FDO protocol HTTP requests
type Rendezvous struct {
	DB          *gorm.DB
	State       *state.RendezvousPersistentState
	MinWaitSecs uint32
	MaxWaitSecs uint32
}

// NewRendezvous creates a new Rendezvous server instance
func NewRendezvous(db *gorm.DB, minWaitSecs, maxWaitSecs uint32) Rendezvous {
	return Rendezvous{DB: db, MinWaitSecs: minWaitSecs, MaxWaitSecs: maxWaitSecs}
}

// Handler wires together all APIs for the Rendezvous server role
func (r *Rendezvous) Handler() http.Handler {
	// Create main mux for the rendezvous server
	rendezvousServeMux := http.NewServeMux()

	// 1. Wire FDO Protocol Handler (TO0/TO1)
	fdoHandler := &fdo_http.Handler{
		Tokens: r.State.Token,
		TO0Responder: &fdo_lib.TO0Server{
			Session:       r.State.TO0Session,
			RVBlobs:       r.State.RVBlob,
			AcceptVoucher: r.acceptVoucher,
		},
		TO1Responder: &fdo_lib.TO1Server{
			Session: r.State.TO1Session,
			RVBlobs: r.State.RVBlob,
		},
	}
	rendezvousServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// 2. Wire Health Check API (from generated code)
	healthServer := health.NewServer(r.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rendezvousServeMux)

	// 3. Content negotiation configuration for Device CA endpoints
	deviceCACertContentTypes := []contenttype.MediaType{
		contenttype.NewMediaType("application/json"),
		contenttype.NewMediaType("application/x-pem-file"),
	}
	deviceCACertPreferredContentType := "application/json"

	// 4. Wire deprecated V1 management APIs
	mgmtAPIServeMuxV1 := http.NewServeMux()
	deviceCAServerV1 := v1deviceca.NewServer(r.State.DeviceCA)
	deviceCAMiddlewaresV1 := []v1deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware[v1deviceca.StrictHandlerFunc](
			deviceCACertContentTypes, deviceCACertPreferredContentType),
	}
	deviceCAStrictHandlerV1 := v1deviceca.NewStrictHandler(&deviceCAServerV1, deviceCAMiddlewaresV1)
	v1deviceca.HandlerFromMux(deviceCAStrictHandlerV1, mgmtAPIServeMuxV1)

	mgmtAPIHandlerV1 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(1<<20, mgmtAPIServeMuxV1),
	)

	// 5. Wire V2 management APIs
	mgmtAPIServeMuxV2 := http.NewServeMux()
	deviceCAServerV2 := v2deviceca.NewServer(r.State.DeviceCA)
	deviceCAMiddlewaresV2 := []v2deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware[v2deviceca.StrictHandlerFunc](
			deviceCACertContentTypes, deviceCACertPreferredContentType),
	}
	deviceCAStrictHandler := v2deviceca.NewStrictHandler(&deviceCAServerV2, deviceCAMiddlewaresV2)
	v2deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtAPIServeMuxV2)

	mgmtAPIHandlerV2 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(1<<20, mgmtAPIServeMuxV2),
	)

	// 6. Mount both API versions
	rendezvousServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))
	rendezvousServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return rendezvousServeMux
}
```

### Key Components Explained

#### 1. Server Structure
```go
type Rendezvous struct {
	DB          *gorm.DB
	State       *state.RendezvousPersistentState
	MinWaitSecs uint32
	MaxWaitSecs uint32
}
```
Contains server dependencies (database, state, configuration).

#### 2. Creating Generated API Handlers
```go
// Create the server implementation
healthServer := health.NewServer(r.State.Health)

// Wrap with strict handler (type-safe middleware chain)
healthStrictHandler := health.NewStrictHandler(&healthServer, nil)

// Register routes using generated HandlerFromMux
health.HandlerFromMux(healthStrictHandler, rendezvousServeMux)
```

**Pattern**:
1. Create server implementation (`NewServer`)
2. Wrap with strict handler (`NewStrictHandler`)
3. Register routes (`HandlerFromMux`)

#### 3. Adding Middleware to Generated Handlers
```go
deviceCAMiddlewares := []v2deviceca.StrictMiddlewareFunc{
	middleware.ContentNegotiationMiddleware[v2deviceca.StrictHandlerFunc](
		deviceCACertContentTypes, deviceCACertPreferredContentType),
}
deviceCAStrictHandler := v2deviceca.NewStrictHandler(&deviceCAServer, deviceCAMiddlewares)
```

Middlewares can be applied to strict handlers for cross-cutting concerns like:
- Content negotiation
- Authentication/authorization
- Logging
- Request validation

#### 4. Route Organization
```go
// Main server mux
rendezvousServeMux := http.NewServeMux()

// Separate mux for each API version
mgmtAPIServeMuxV1 := http.NewServeMux()
mgmtAPIServeMuxV2 := http.NewServeMux()

// Wrap each version with rate limiting and body size middleware
mgmtAPIHandlerV1 := middleware.RateLimitMiddleware(
	rate.NewLimiter(2, 10),
	middleware.BodySizeMiddleware(1<<20, mgmtAPIServeMuxV1),
)
mgmtAPIHandlerV2 := middleware.RateLimitMiddleware(
	rate.NewLimiter(2, 10),
	middleware.BodySizeMiddleware(1<<20, mgmtAPIServeMuxV2),
)

// Mount management APIs under /api/v1/ (deprecated) and /api/v2/ (current)
rendezvousServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))
rendezvousServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))
```

This pattern allows:
- Different middleware for different API groups
- Clean separation of concerns
- Easy to add/remove APIs

#### 5. Handler Implementation Pattern

For each generated API, implement the `StrictServerInterface`:

**File: `api/v2/health/handler.go`**

```go
package health

import (
	"context"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/api/v2/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

type Server struct {
	State *state.HealthState
}

func NewServer(state *state.HealthState) Server {
	return Server{State: state}
}

// Make sure we conform to StrictServerInterface
var _ StrictServerInterface = (*Server)(nil)

// GetHealth implements the generated StrictServerInterface
func (s *Server) GetHealth(ctx context.Context, request GetHealthRequestObject) (GetHealthResponseObject, error) {
	if err := s.State.Ping(); err != nil {
		slog.Error("database error", "err", err)
		return GetHealth500JSONResponse{
			components.InternalServerError{Message: "database error"},
		}, nil
	}
	return GetHealth200JSONResponse{
		HealthStatusJSONResponse{
			Version: version.VERSION,
			Status:  "OK",
			Message: "the service is up and running",
		},
	}, nil
}
```

**Key Points**:
- Implement `StrictServerInterface` (generated)
- Use compile-time check: `var _ StrictServerInterface = (*Server)(nil)`
- Return typed response objects (e.g., `GetHealth200JSONResponse`)
- Use common types from `components` package

## Regenerating Code

### Prerequisites

Install required tools:

```bash
# Install oapi-codegen
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

# npx is included with Node.js/npm
# On Fedora/RHEL:
sudo dnf install npm

# On macOS:
brew install node

# On Ubuntu/Debian:
sudo apt install npm
```

### Regenerate All Code

From the project root:

```bash
go generate ./api/...
```

This executes all `//go:generate` directives in `api/v{1,2}/*/generate.go`, which:
1. Generates Go handlers from OpenAPI definitions
2. Generates standalone OpenAPI documents

### Regenerate Only Go Code

```bash
# All handlers
go generate -run oapi-codegen ./api/v2/...

# Specific handler
go tool oapi-codegen -config api/v2/voucher/config.yaml api/v2/voucher/openapi.yaml
```

### Regenerate Only OpenAPI Docs

```bash
# All three server specs
go generate -run openapi-format ./api/v2/...

# Specific server
npx openapi-format api/v2/manufacturer/openapi.yaml --json -o api/v2/manufacturer/openapi.json
```

## Adding a New Endpoint

Follow these steps to add a new API endpoint:

### 1. Create OpenAPI Definition

Create `api/v2/myendpoint/openapi.yaml`:

```yaml
openapi: 3.0.0
info:
  title: My Endpoint API
  version: 1.0.0
  description: Description of the endpoint
tags:
  - name: myendpoint
    description: Operations for my endpoint
paths:
  /v2/myendpoint:
    get:
      tags:
        - myendpoint
      operationId: GetMyEndpoint
      summary: Get my endpoint
      responses:
        '200':
          description: Success
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/MyResponse'
        '500':
          $ref: '../components/openapi.yaml#/components/responses/InternalServerError'
components:
  schemas:
    MyResponse:
      type: object
      properties:
        data:
          type: string
```

**Important**:
- Always reference common types from `../components/openapi.yaml` where possible
- Use `operationId` for every operation (becomes Go method name)
- Follow existing patterns for pagination, filtering, error responses

### 2. Create oapi-codegen Configuration

Create `api/v2/myendpoint/config.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/HEAD/configuration-schema.json
package: myendpoint
generate:
  std-http-server: true
  strict-server: true
  models: true
output: handler.gen.go
import-mapping:
  ../components/openapi.yaml: "github.com/fido-device-onboard/go-fdo-server/api/v2/components"
output-options:
  skip-prune: true
```

### 3. Add Generate Directive

Create `api/v2/myendpoint/generate.go`:

```go
package myendpoint

//go:generate go tool oapi-codegen -config config.yaml openapi.yaml
```

### 4. Generate Code

```bash
go generate ./api/...
```

This creates `api/v2/myendpoint/handler.gen.go` with:
- `ServerInterface` - interface you'll implement
- `StrictServerInterface` - strict typed version
- Request/response types
- Handler registration functions

### 5. Implement Handler

Create `api/v2/myendpoint/handler.go`:

```go
package myendpoint

import (
	"context"
)

type Server struct {
	// Add dependencies (DB, logger, state, etc.)
}

func NewServer(/* dependencies */) Server {
	return Server{/* ... */}
}

// Make sure we conform to StrictServerInterface
var _ StrictServerInterface = (*Server)(nil)

// Implement StrictServerInterface
func (s *Server) GetMyEndpoint(ctx context.Context, request GetMyEndpointRequestObject) (GetMyEndpointResponseObject, error) {
	// Your implementation here
	return GetMyEndpoint200JSONResponse{
		Data: "hello world",
	}, nil
}
```

### 6. Wire Handler into Server Role

Add to your server's `Handler()` method (e.g., `api/v2/manufacturer/handler.go`):

```go
// Wire My Endpoint API
myEndpointServer := myendpoint.NewServer(/* deps */)
myEndpointStrictHandler := myendpoint.NewStrictHandler(&myEndpointServer, nil)
myendpoint.HandlerFromMux(myEndpointStrictHandler, mgmtAPIServeMux)
```

### 7. Add to Aggregation Spec (Optional)

If this endpoint should appear in one of the server role specs, add it to the appropriate aggregation file.

Edit `api/v2/manufacturer/openapi.yaml` (or `owner/openapi.yaml`, `rendezvous/openapi.yaml`):

```yaml
paths:
  # ... existing paths ...
  /v2/myendpoint:
    $ref: 'myendpoint.yaml#/paths/~1v2~1myendpoint'
```

Then regenerate:

```bash
npx openapi-format api/v2/manufacturer/openapi.yaml --json -o api/v2/manufacturer/openapi.json
```

## Best Practices

### OpenAPI Definitions

1. **Use $ref for Reusability**: Reference common components rather than duplicating
   ```yaml
   $ref: '../components/openapi.yaml#/components/schemas/Error'
   ```

2. **Follow Naming Conventions**:
   - Operations: `GetResource`, `ListResources`, `CreateResource`, `UpdateResource`, `DeleteResource`
   - Schemas: `PascalCase` for types
   - Properties: `camelCase` for JSON fields

3. **Include Examples**: Add examples to schemas for better documentation
   ```yaml
   example: "192.168.1.100"
   ```

4. **Use Standard Responses**: Reference common responses from the components spec
   ```yaml
   '400':
     $ref: '../components/openapi.yaml#/components/responses/BadRequest'
   ```

5. **Document Everything**: Add descriptions to all operations, parameters, and schemas

### Code Generation

1. **Don't Edit Generated Files**: Files ending in `.gen.go` are regenerated - changes will be lost

2. **Implement StrictServerInterface**: Prefer strict interfaces for type safety
   ```go
   func (h *Handler) GetResource(ctx context.Context, req GetResourceRequestObject) (GetResourceResponseObject, error)
   ```

3. **Keep Handlers Pure**: Generated handlers should be thin wrappers around business logic

4. **Version Your API**: Include version in paths (`/v2/resource`) for future compatibility

5. **Use Compile-Time Checks**: Ensure your implementation satisfies the interface
   ```go
   var _ StrictServerInterface = (*Server)(nil)
   ```

## Viewing Generated Documentation

### Using Swagger UI (Local)

```bash
make fdo-openapi-ui
```

This launches Swagger UI in a container at http://localhost:9080 with all three API specs.

### Using Swagger UI (Manual)

```bash
# Using podman or docker
podman run --rm -p 9080:8080 \
  -v ./api:/usr/share/nginx/html/api:z \
  -e URLS='[
    {"url": "/api/v2/manufacturer/openapi.json", "name": "Manufacturer API"},
    {"url": "/api/v2/rendezvous/openapi.json", "name": "Rendezvous API"},
    {"url": "/api/v2/owner/openapi.json", "name": "Owner API"}
  ]' \
  docker.swagger.io/swaggerapi/swagger-ui

# Open browser
open http://localhost:9080
```

### Using Redoc

```bash
npx @redocly/cli preview-docs api/v2/manufacturer/openapi.yaml
```

## Troubleshooting

### Error: "cannot find package components"

**Cause**: Import mapping isn't working correctly.

**Solution**: Ensure the components package is generated first:
```bash
go tool oapi-codegen -config api/v2/components/config.yaml api/v2/components/openapi.yaml
```

### Error: "$ref could not be resolved"

**Cause**: Relative paths in `$ref` are incorrect.

**Solution**: Check that referenced files exist relative to the OpenAPI file:
```yaml
# From api/v2/voucher/openapi.yaml
$ref: '../components/openapi.yaml#/components/schemas/Error'  # ✓ Correct
$ref: 'components.yaml#/components/schemas/Error'              # ✗ Wrong (no such file)
```

### Generated Code Doesn't Compile

**Cause**: OpenAPI spec has validation issues.

**Solution**: Validate your spec:
```bash
npx @apidevtools/swagger-cli validate api/v2/myendpoint/openapi.yaml
```

### npx Command Not Found

**Cause**: npm is not installed.

**Solution**: Install Node.js/npm:
```bash
# Fedora/RHEL
sudo dnf install npm

# macOS
brew install node

# Ubuntu/Debian
sudo apt install npm
```

### Changes Not Reflected After Regeneration

**Cause**:
1. Modified a `.gen.go` file directly (don't do this)
2. Build cache needs clearing

**Solution**:
```bash
# Clean and regenerate
go clean -cache
go generate ./api/...
go build ./...
```

### Handler Doesn't Implement Interface

**Cause**: Missing method or wrong signature.

**Solution**: Check the compile-time assertion:
```go
var _ StrictServerInterface = (*Server)(nil)
```

The compiler will tell you exactly which methods are missing or have wrong signatures.

## References

- [oapi-codegen Documentation](https://github.com/oapi-codegen/oapi-codegen)
- [OpenAPI 3.0 Specification](https://swagger.io/specification/)
- [openapi-format Documentation](https://github.com/thim81/openapi-format)
- [FDO Specification](https://fidoalliance.org/specs/FDO/)

## See Also

- [`README.md`](../README.md) - General project documentation
- [User Guide](user-guide/README.md) - The FDO server user guide
