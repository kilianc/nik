package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

func ListContacts(ctx context.Context, db *sql.DB, limit, offset int) ([]Contact, error) {
	rows, err := db.QueryContext(ctx, queries.ListContacts, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}
