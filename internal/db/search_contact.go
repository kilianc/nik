package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func SearchContact(ctx context.Context, db *sql.DB, query string, threshold float64, limit int) ([]ContactSearchResult, error) {
	rows, err := db.QueryContext(ctx, queries.SearchContact, query, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ContactSearchResult
	for rows.Next() {
		var r ContactSearchResult
		var nicknames, emails, whatsappIDs, telegramIDs, slackIDs, phoneNumbers any
		err := rows.Scan(
			&r.ID,
			&r.Name,
			&nicknames,
			&emails,
			&whatsappIDs,
			&telegramIDs,
			&slackIDs,
			&phoneNumbers,
			&r.Timezone,
			&r.Location,
			&r.OneLiner,
			&r.Notes,
			&r.LastMessageAt,
			&r.LastSeenAt,
			&r.CreatedAt,
			&r.UpdatedAt,
			&r.Score,
		)
		if err != nil {
			return nil, err
		}
		if r.Nicknames, err = scanStringSlice(nicknames); err != nil {
			return nil, fmt.Errorf("scan nicknames: %w", err)
		}
		if r.Emails, err = scanStringSlice(emails); err != nil {
			return nil, fmt.Errorf("scan emails: %w", err)
		}
		if r.WhatsappIDs, err = scanStringSlice(whatsappIDs); err != nil {
			return nil, fmt.Errorf("scan whatsapp_ids: %w", err)
		}
		if r.TelegramIDs, err = scanStringSlice(telegramIDs); err != nil {
			return nil, fmt.Errorf("scan telegram_ids: %w", err)
		}
		if r.SlackIDs, err = scanStringSlice(slackIDs); err != nil {
			return nil, fmt.Errorf("scan slack_ids: %w", err)
		}
		if r.PhoneNumbers, err = scanStringSlice(phoneNumbers); err != nil {
			return nil, fmt.Errorf("scan phone_numbers: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
