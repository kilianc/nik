package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func ToolCallInsertOne(ctx context.Context, db *sql.DB, activationID, name string, duration time.Duration, isError bool) error {
	errFlag := 0
	if isError {
		errFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ToolCallInsertOne,
		id.V7(),
		activationID,
		name,
		duration.Milliseconds(),
		errFlag,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert tool_call %s: %w", name, err)
	}

	return nil
}
