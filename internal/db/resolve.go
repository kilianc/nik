package db

import (
	"context"
	"database/sql"
	"fmt"
)

var allowedTables = map[string]bool{
	"task":  true,
	"alarm": true,
}

// ResolveShortID finds a full ID from a short suffix in the given table.
// Returns an error if zero or multiple matches are found.
// The table parameter is interpolated into SQL, so it is restricted to an
// allowlist to prevent injection if a future caller passes untrusted input.
func ResolveShortID(ctx context.Context, db *sql.DB, table, shortID string) (string, error) {
	if !allowedTables[table] {
		return "", fmt.Errorf("resolve short id: invalid table %q", table)
	}

	if len(shortID) >= 36 {
		return shortID, nil
	}

	query := fmt.Sprintf(
		"SELECT id FROM %s WHERE id LIKE '%%' || ?1 ORDER BY created_at DESC LIMIT 2",
		table,
	)

	rows, err := db.QueryContext(ctx, query, shortID)
	if err != nil {
		return "", fmt.Errorf("resolve short id in %s: %w", table, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scan resolved id: %w", err)
		}
		ids = append(ids, id)
	}

	if err = rows.Err(); err != nil {
		return "", err
	}

	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no %s found matching id suffix %q", table, shortID)
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("multiple %ss match id suffix %q — use a longer id", table, shortID)
	}
}
