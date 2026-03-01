package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type UpsertContactParams struct {
	Platform      string
	ExternalID    string
	Name          string
	Phone         string
	IsSelf        bool
	SelfID        string
	LastMessageAt time.Time
}

func UpsertContact(ctx context.Context, db *sql.DB, p UpsertContactParams) (Contact, error) {
	switch p.Platform {
	case "whatsapp":
		if p.IsSelf {
			return upsertSelfContactWhatsApp(ctx, db, p)
		}

		return upsertContactWhatsApp(ctx, db, p)
	default:
		return Contact{}, fmt.Errorf("upsert contact: unsupported platform %q", p.Platform)
	}
}

func upsertContactWhatsApp(ctx context.Context, db *sql.DB, p UpsertContactParams) (Contact, error) {
	newID := id.V7()

	_, err := db.ExecContext(ctx, queries.ContactUpsertWhatsAppInsert,
		newID, p.Name, p.ExternalID, p.Phone, p.LastMessageAt,
	)
	if err != nil {
		return Contact{}, fmt.Errorf("insert: %w", err)
	}

	_, err = db.ExecContext(ctx, queries.ContactUpsertWhatsAppUpdate,
		p.Name, p.Phone, p.LastMessageAt, p.ExternalID,
	)
	if err != nil {
		return Contact{}, fmt.Errorf("update: %w", err)
	}

	return GetContact(ctx, db, p.ExternalID)
}

func upsertSelfContactWhatsApp(ctx context.Context, db *sql.DB, p UpsertContactParams) (Contact, error) {
	if p.SelfID == "" {
		return Contact{}, fmt.Errorf("empty self contact id")
	}
	if p.ExternalID == "" {
		return Contact{}, fmt.Errorf("empty whatsapp id")
	}

	_, err := db.ExecContext(ctx, queries.ContactUpsertSelfWhatsApp, p.SelfID, p.ExternalID, p.LastMessageAt)
	if err != nil {
		return Contact{}, fmt.Errorf("upsert self contact: %w", err)
	}

	return GetContact(ctx, db, p.SelfID)
}
