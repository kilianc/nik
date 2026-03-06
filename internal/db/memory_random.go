package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func MemoryRandom(ctx context.Context, db *sql.DB, before time.Time, limit int) ([]RandomMemory, error) {
	rows, err := db.QueryContext(ctx, queries.MemoryRandom, before, limit)
	if err != nil {
		return nil, fmt.Errorf("random memories: %w", err)
	}
	defer rows.Close()

	var out []RandomMemory
	for rows.Next() {
		var m RandomMemory

		err = rows.Scan(
			&m.ID,
			&m.Content,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan random memory: %w", err)
		}

		out = append(out, m)
	}

	return out, rows.Err()
}
