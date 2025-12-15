package internal

//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/components.yaml ../api/components.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/health.yaml ../api/health.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/device-ca.yaml ../api/device-ca.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/rvinfo.yaml ../api/rvinfo.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/rvto2addr.yaml ../api/rvto2addr.yaml
