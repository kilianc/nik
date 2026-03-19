package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func scanContact(s scanner) (Contact, error) {
	var c Contact
	var nicknames, emails, whatsappIDs, telegramIDs, slackIDs, phoneNumbers any

	err := s.Scan(
		&c.ID,
		&c.Name,
		&nicknames,
		&emails,
		&whatsappIDs,
		&telegramIDs,
		&slackIDs,
		&phoneNumbers,
		&c.Timezone,
		&c.Location,
		&c.OneLiner,
		&c.Notes,
		&c.LastMessageAt,
		&c.LastSeenAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return c, err
	}

	if c.Nicknames, err = scanStringSlice(nicknames); err != nil {
		return c, fmt.Errorf("scan nicknames: %w", err)
	}
	if c.Emails, err = scanStringSlice(emails); err != nil {
		return c, fmt.Errorf("scan emails: %w", err)
	}
	if c.WhatsappIDs, err = scanStringSlice(whatsappIDs); err != nil {
		return c, fmt.Errorf("scan whatsapp_ids: %w", err)
	}
	if c.TelegramIDs, err = scanStringSlice(telegramIDs); err != nil {
		return c, fmt.Errorf("scan telegram_ids: %w", err)
	}
	if c.SlackIDs, err = scanStringSlice(slackIDs); err != nil {
		return c, fmt.Errorf("scan slack_ids: %w", err)
	}
	if c.PhoneNumbers, err = scanStringSlice(phoneNumbers); err != nil {
		return c, fmt.Errorf("scan phone_numbers: %w", err)
	}

	return c, nil
}

func GetContact(ctx context.Context, db *sql.DB, identifier string) (Contact, error) {
	row := db.QueryRowContext(ctx, queries.ContactGet, identifier)
	return scanContact(row)
}

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

type ContactUpdateFieldParams struct {
	ID         string
	Field      string
	Value      string
	ArrayValue []string
}

func ContactUpdateField(ctx context.Context, db *sql.DB, p ContactUpdateFieldParams) error {
	if p.ID == "" {
		return fmt.Errorf("empty contact_id")
	}

	switch p.Field {
	case "name", "notes", "one_liner", "timezone", "location", "nicknames", "emails", "phone_numbers":
		_, err := db.ExecContext(ctx, queries.ContactUpdateField, p.ID, p.Field, p.Value, MarshalStringSlice(p.ArrayValue))
		if err != nil {
			return fmt.Errorf("update contact %s field %s: %w", p.ID, p.Field, err)
		}

		return nil
	default:
		return fmt.Errorf("unknown contact field %q", p.Field)
	}
}

type ContactAddWhatsAppIDParams struct {
	ContactID string
	JID       string
	Phone     string
}

func ContactAddWhatsAppID(ctx context.Context, db *sql.DB, p ContactAddWhatsAppIDParams) error {
	if p.ContactID == "" || p.JID == "" {
		return nil
	}

	_, err := db.ExecContext(ctx, queries.ContactAddWhatsAppID, p.ContactID, p.JID, p.Phone)
	if err != nil {
		return fmt.Errorf("add whatsapp id %s to contact %s: %w", p.JID, p.ContactID, err)
	}

	return nil
}
