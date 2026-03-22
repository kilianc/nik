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
	ActivationID      string
	ActivationRoundID string
	Name              string
	Input             string
	Output            string
	Duration          time.Duration
	IsError           bool
}

func ToolCallInsert(ctx context.Context, db *sql.DB, p ToolCallInsertParams) error {
	errFlag := 0
	if p.IsError {
		errFlag = 1
	}

	var roundID any
	if p.ActivationRoundID != "" {
		roundID = p.ActivationRoundID
	}

	_, err := db.ExecContext(ctx, queries.ToolCallInsert,
		id.V7(),
		p.ActivationID,
		roundID,
		p.Name,
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
