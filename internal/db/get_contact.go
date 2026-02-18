package db

import (
	"context"
	"database/sql"
	"fmt"

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

// GetContact looks up a contact by UUID or known platform identifier.
func GetContact(ctx context.Context, db *sql.DB, identifier string) (Contact, error) {
	row := db.QueryRowContext(ctx, queries.GetContact, identifier)
	return scanContact(row)
}
