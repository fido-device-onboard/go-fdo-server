package state

import (
	"log/slog"

	"gorm.io/gorm"
)

// ManufacturingState aggregates all state needed for the manufacturing server
type ManufacturingState struct {
	DISession *DISessionState
	Voucher   *VoucherPersistentState
	RvInfo    *RvInfoState
	Token     *TokenService
	Health    *HealthState
}

// InitManufacturingDB initializes all database state needed for the manufacturing server
func InitManufacturingDB(db *gorm.DB) (*ManufacturingState, error) {
	diSessionState, err := InitDISessionDB(db)
	if err != nil {
		return nil, err
	}

	voucherState, err := InitVoucherDB(db)
	if err != nil {
		return nil, err
	}

	rvInfoState, err := InitRvInfoDB(db)
	if err != nil {
		return nil, err
	}

	healthState, err := InitHealthDB(db)
	if err != nil {
		return nil, err
	}

	slog.Info("Manufacturing database initialized successfully")

	return &ManufacturingState{
		DISession: diSessionState,
		Voucher:   voucherState,
		RvInfo:    rvInfoState,
		Token:     diSessionState.Token,
		Health:    healthState,
	}, nil
}
