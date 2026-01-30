package state

import (
	"gorm.io/gorm"
)

type RendezvousPersistentState struct {
	Health     *HealthState
	Token      *TokenService
	TO0Session *TO0SessionState
	TO1Session *TO1SessionState
	RVBlob     *RendezvousBlobPersistentState
	DeviceCA   *TrustedDeviceCACertsState
}

func InitRendezvousDB(db *gorm.DB) (*RendezvousPersistentState, error) {
	healthState, err := InitHealthDB(db)
	if err != nil {
		return nil, err
	}
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	to0SessionState, err := InitTO0SessionDB(db)
	if err != nil {
		return nil, err
	}
	to1SessionState, err := InitTO1SessionDB(db)
	if err != nil {
		return nil, err
	}
	rendezvousBlobPersistentState, err := InitRendezvousBlobDB(db)
	if err != nil {
		return nil, err
	}
	deviceCAState, err := InitTrustedDeviceCACertsDB(db)
	if err != nil {
		return nil, err
	}

	rendezvousPersistentState := &RendezvousPersistentState{
		Health:     healthState,
		Token:      tokenServiceState,
		TO0Session: to0SessionState,
		TO1Session: to1SessionState,
		RVBlob:     rendezvousBlobPersistentState,
		DeviceCA:   deviceCAState,
	}
	return rendezvousPersistentState, nil
}
