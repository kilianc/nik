package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ToolCallInsertParams struct {
	ActivationID string
	Name         string
	Round        int
	Input        string
	Output       string
	Duration     time.Duration
	IsError      bool
}

func ToolCallInsertOne(ctx context.Context, db *sql.DB, p ToolCallInsertParams) error {
	errFlag := 0
	if p.IsError {
		errFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ToolCallInsertOne,
		id.V7(),
		p.ActivationID,
		p.Name,
		p.Round,
		p.Input,
		p.Output,
		p.Duration.Milliseconds(),
		errFlag,
	)
	if err != nil {
		return fmt.Errorf("insert tool_call %s: %w", p.Name, err)
	}

	return nil
}
