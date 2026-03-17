package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

type ShellSessionUpsertParams struct {
	ID           string
	ActivationID string
	Command      string
	Description  string
	Output       string
	ExitCode     *int
	Alive        bool
}

func ShellSessionUpsert(ctx context.Context, db DBTX, p ShellSessionUpsertParams) error {
	aliveFlag := 0
	if p.Alive {
		aliveFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ShellSessionUpsert,
		p.ID,
		p.ActivationID,
		p.Command,
		p.Description,
		p.Output,
		p.ExitCode,
		aliveFlag,
	)
	if err != nil {
		return fmt.Errorf("upsert shell session %s: %w", p.ID, err)
	}

	return nil
}

type ShellSessionUpdateParams struct {
	ID       string
	Output   string
	ExitCode *int
	Alive    bool
}

func ShellSessionUpdate(ctx context.Context, db DBTX, p ShellSessionUpdateParams) error {
	aliveFlag := 0
	if p.Alive {
		aliveFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ShellSessionUpdate,
		p.ID,
		p.Output,
		p.ExitCode,
		aliveFlag,
	)
	if err != nil {
		return fmt.Errorf("update shell session %s: %w", p.ID, err)
	}

	return nil
}

func ShellSessionAliveIDs(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx, queries.ShellSessionAlive)
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
