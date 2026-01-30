package state

import (
	"fmt"

	"gorm.io/gorm"
)

type HealthState struct {
	DB *gorm.DB
}

func InitHealthDB(db *gorm.DB) (*HealthState, error) {
	state := &HealthState{
		DB: db,
	}
	if err := state.Ping(); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *HealthState) Ping() error {
	// Send a ping to make sure the database connection is alive.
	sqlDB, err := s.DB.DB()
	if err != nil {
		return fmt.Errorf("unable to get db connection")
	}
	return sqlDB.Ping()
}
