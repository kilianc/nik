package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	GenesisStartedAtKey   = "genesis_started_at"
	GenesisCompletedAtKey = "genesis_completed_at"
)

func GenesisStartedAtEnsure(ctx context.Context, conn *sql.DB) (time.Time, error) {
	s, err := SettingGet(ctx, conn, GenesisStartedAtKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("get %s: %w", GenesisStartedAtKey, err)
	}
	if s != nil {
		return ParseTimeValue(s.Value)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := SettingSet(ctx, conn, GenesisStartedAtKey, ISO8601MS(now)); err != nil {
		return time.Time{}, fmt.Errorf("set %s: %w", GenesisStartedAtKey, err)
	}
	return now, nil
}
