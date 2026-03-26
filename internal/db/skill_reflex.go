package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func SkillReflexLatest(ctx context.Context, db DBTX, skillName string) (string, time.Time, error) {
	var meta, ts string

	err := db.QueryRowContext(ctx, queries.SkillReflexGet, skillName).Scan(&meta, &ts)
	if err == sql.ErrNoRows {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("skill reflex latest %s: %w", skillName, err)
	}

	parsed, err := parseTimestampString(ts)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("skill reflex latest %s: %w", skillName, err)
	}

	return meta, parsed, nil
}

func SkillReflexInsert(ctx context.Context, db DBTX, skillName, meta string) error {
	_, err := db.ExecContext(ctx, queries.SkillReflexInsert,
		id.V7(),
		skillName,
		meta,
		ISO8601MS(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("insert skill reflex %s: %w", skillName, err)
	}

	return nil
}
