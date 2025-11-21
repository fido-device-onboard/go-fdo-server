# FDO Owner Server JSON API

OpenAPI 3.1 specification for FIDO Device Onboard Owner Server.

## Implementation

✅ All endpoints return JSON by default  
✅ Backward compatible PEM responses via `Accept: application/x-pem-file`  
✅ OpenAPI code generation ready  
✅ Split server architecture preserved  

## Usage

```bash
# Generate client/server code
make generate-api

# Validate specification  
make validate-openapi
```

## Testing

```bash
go test -v ./api/handlersTest/ -run TestJSONResponsesRequired
```