package internal

//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/components.yaml ../api/definitions/components.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/health.yaml ../api/definitions/health.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/device-ca.yaml ../api/definitions/device-ca.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/rvinfo.yaml ../api/definitions/rvinfo.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/rvto2addr.yaml ../api/definitions/rvto2addr.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/voucher.yaml ../api/definitions/voucher.yaml
