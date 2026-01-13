package internal

//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/components.yaml ../api/components.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/health.yaml ../api/health.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/voucher.yaml ../api/voucher.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/rvto2addr.yaml ../api/rvto2addr.yaml
//go:generate go tool oapi-codegen -config ../configs/goapi-codegen/resell.yaml ../api/resell.yaml
