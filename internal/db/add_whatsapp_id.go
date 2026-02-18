package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func AddWhatsAppID(ctx context.Context, db *sql.DB, contactID, jid, phone string) error {
	if contactID == "" || jid == "" {
		return nil
	}

	_, err := db.ExecContext(ctx, queries.ContactAddWhatsAppID, contactID, jid, phone)
	if err != nil {
		return fmt.Errorf("add whatsapp id %s to contact %s: %w", jid, contactID, err)
	}

	return nil
}
