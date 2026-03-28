package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/queries"
)

func Prune(ctx context.Context, conn *sql.DB, before string) (int64, error) {
	stmts := splitStatements(queries.Prune)
	if len(stmts) == 0 {
		return 0, nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune tx: %w", err)
	}
	defer tx.Rollback()

	var total int64
	for _, stmt := range stmts {
		res, err := tx.ExecContext(ctx, stmt, before)
		if err != nil {
			return 0, fmt.Errorf("prune exec: %w", err)
		}

		n, _ := res.RowsAffected()
		total += n
	}

	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf("commit prune tx: %w", err)
	}

	if total > 0 {
		conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
		conn.ExecContext(ctx, "VACUUM")
	}

	return total, nil
}

func splitStatements(raw string) []string {
	parts := strings.Split(raw, ";\n")

	var out []string
	for _, p := range parts {
		s := stripComments(p)
		s = strings.TrimRight(s, ";")
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}

	return out
}

func stripComments(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
