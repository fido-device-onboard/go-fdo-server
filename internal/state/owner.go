package state

import (
	"gorm.io/gorm"
)

type OwnerState struct {
	Health     *HealthState
	Token      *TokenService
	DeviceCA   *TrustedDeviceCACertsState
	TO0Session *TO0SessionState
	TO2Session *TO2SessionState
	Voucher    *VoucherPersistentState
	OwnerKey   *OwnerKeyPersistentState
	RVTO2Addr  *RVTO2AddrState
}

func InitOwnerDB(db *gorm.DB) (*OwnerState, error) {
	healthState, err := InitHealthDB(db)
	if err != nil {
		return nil, err
	}

	sessionState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}

	deviceCAState, err := InitTrustedDeviceCACertsDB(db)
	if err != nil {
		return nil, err
	}

	to2SessionState, err := InitTO2SessionDB(db)
	if err != nil {
		return nil, err
	}

	voucherState, err := InitVoucherDB(db)
	if err != nil {
		return nil, err
	}

	rvto2addrState, err := InitRVTO2AddrDB(db)
	if err != nil {
		return nil, err
	}

	state := &OwnerState{
		Health:     healthState,
		Token:      sessionState,
		DeviceCA:   deviceCAState,
		TO2Session: to2SessionState,
		Voucher:    voucherState,
		RVTO2Addr:  rvto2addrState,
		OwnerKey:   nil, // Initialized later with owner's signing key
	}

	return state, nil
}
