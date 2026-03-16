package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ShellOutputUpsertParams struct {
	SessionID    string
	ActivationID string
	Command      string
	Description  string
	Output       string
	ExitCode     *int
	Alive        bool
}

func ShellOutputUpsert(ctx context.Context, db DBTX, p ShellOutputUpsertParams) error {
	aliveFlag := 0
	if p.Alive {
		aliveFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ShellOutputUpsert,
		id.V7(),
		p.SessionID,
		p.ActivationID,
		p.Command,
		p.Description,
		p.Output,
		p.ExitCode,
		aliveFlag,
	)
	if err != nil {
		return fmt.Errorf("upsert shell output %s: %w", p.SessionID, err)
	}

	return nil
}

type ShellOutputUpdateParams struct {
	SessionID string
	Output    string
	ExitCode  *int
	Alive     bool
}

func ShellOutputUpdate(ctx context.Context, db DBTX, p ShellOutputUpdateParams) error {
	aliveFlag := 0
	if p.Alive {
		aliveFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ShellOutputUpdate,
		p.SessionID,
		p.Output,
		p.ExitCode,
		aliveFlag,
	)
	if err != nil {
		return fmt.Errorf("update shell output %s: %w", p.SessionID, err)
	}

	return nil
}

func ShellOutputAliveIDs(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx, queries.ShellOutputAlive)
	if err != nil {
		return nil, fmt.Errorf("list alive shell sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("scan alive shell session: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}
