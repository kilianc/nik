package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func UpdateContactField(ctx context.Context, db *sql.DB, id, field, value string, arrayValue []string) error {
	if id == "" {
		return fmt.Errorf("empty contact_id")
	}

	switch field {
	case "name", "notes", "one_liner", "timezone", "location", "nicknames", "emails", "phone_numbers":
		_, err := db.ExecContext(ctx, queries.UpdateContactField, id, field, value, MarshalStringSlice(arrayValue))
		if err != nil {
			return fmt.Errorf("update contact %s field %s: %w", id, field, err)
		}

		return nil
	default:
		return fmt.Errorf("unknown contact field %q", field)
	}
}
