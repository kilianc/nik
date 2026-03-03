package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ToolCallRow struct {
	Name       string
	DurationMS int64
	Error      bool
}

func ToolCallInsert(ctx context.Context, db *sql.DB, activationID string, calls []ToolCallRow) error {
	if len(calls) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tool_call tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	for _, c := range calls {
		errFlag := 0
		if c.Error {
			errFlag = 1
		}

		_, err = tx.ExecContext(ctx, queries.ToolCallInsert,
			id.V7(),
			activationID,
			c.Name,
			c.DurationMS,
			errFlag,
			now,
		)
		if err != nil {
			return fmt.Errorf("insert tool_call %s: %w", c.Name, err)
		}
	}

	return tx.Commit()
}
