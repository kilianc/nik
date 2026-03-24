package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ToolCallListRow struct {
	Name    string
	Input   string
	Output  string
	Round   int
	RoundID string
}

func ToolCallList(ctx context.Context, db *sql.DB, activationID string, round *int) ([]ToolCallListRow, error) {
	rows, err := db.QueryContext(ctx, queries.ToolCallList, activationID, round)
	if err != nil {
		return nil, fmt.Errorf("list tool_call for activation %s: %w", activationID, err)
	}
	defer rows.Close()

	var calls []ToolCallListRow

	for rows.Next() {
		var tc ToolCallListRow

		err = rows.Scan(&tc.Name, &tc.Input, &tc.Output, &tc.Round, &tc.RoundID)
		if err != nil {
			return nil, fmt.Errorf("scan tool_call: %w", err)
		}

		calls = append(calls, tc)
	}

	return calls, rows.Err()
}

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
