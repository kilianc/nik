package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func RecallMessages(ctx context.Context, db *sql.DB, since time.Time) ([]RecallMessage, error) {
	rows, err := db.QueryContext(ctx, queries.RecallMessages, since)
	if err != nil {
		return nil, fmt.Errorf("recall messages: %w", err)
	}
	defer rows.Close()

	var out []RecallMessage

	for rows.Next() {
		var m RecallMessage

		err = rows.Scan(
			&m.Body,
			&m.SentAt,
			&m.IsFromMe,
			&m.SenderName,
			&m.ConversationTitle,
			&m.ConversationKind,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recall message: %w", err)
		}

		out = append(out, m)
	}

	return out, rows.Err()
}

func RecallContacts(ctx context.Context, conn *sql.DB) ([]RecallContact, error) {
	rows, err := conn.QueryContext(ctx, queries.RecallContacts)
	if err != nil {
		return nil, fmt.Errorf("recall contacts: %w", err)
	}
	defer rows.Close()

	var out []RecallContact

	for rows.Next() {
		var c RecallContact
		var nicknames, emails, phones, wids any

		err = rows.Scan(
			&c.Name,
			&nicknames,
			&emails,
			&phones,
			&wids,
			&c.Timezone,
			&c.Location,
			&c.OneLiner,
			&c.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recall contact: %w", err)
		}

		c.Nicknames, _ = scanStringSlice(nicknames)
		c.Emails, _ = scanStringSlice(emails)
		c.PhoneNumbers, _ = scanStringSlice(phones)
		c.WhatsappIDs, _ = scanStringSlice(wids)
		out = append(out, c)
	}

	return out, rows.Err()
}

func RecallAlarms(ctx context.Context, conn *sql.DB) ([]RecallAlarm, error) {
	rows, err := conn.QueryContext(ctx, queries.RecallAlarms)
	if err != nil {
		return nil, fmt.Errorf("recall alarms: %w", err)
	}
	defer rows.Close()

	var out []RecallAlarm

	for rows.Next() {
		var a RecallAlarm

		err = rows.Scan(
			&a.Goal,
			&a.Recurrence,
			&a.NextFireAt,
			&a.CancelledAt,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recall alarm: %w", err)
		}

		out = append(out, a)
	}

	return out, rows.Err()
}

func RecallJournals(ctx context.Context, conn *sql.DB) ([]RecallJournal, error) {
	rows, err := conn.QueryContext(ctx, queries.RecallJournals)
	if err != nil {
		return nil, fmt.Errorf("recall journals: %w", err)
	}
	defer rows.Close()

	var out []RecallJournal

	for rows.Next() {
		var j RecallJournal

		err = rows.Scan(&j.Date, &j.Content)
		if err != nil {
			return nil, fmt.Errorf("scan recall journal: %w", err)
		}

		out = append(out, j)
	}

	return out, rows.Err()
}

func RecallDreams(ctx context.Context, conn *sql.DB) ([]RecallDream, error) {
	rows, err := conn.QueryContext(ctx, queries.RecallDreams)
	if err != nil {
		return nil, fmt.Errorf("recall dreams: %w", err)
	}
	defer rows.Close()

	var out []RecallDream

	for rows.Next() {
		var d RecallDream

		err = rows.Scan(&d.Date, &d.Pass, &d.Content)
		if err != nil {
			return nil, fmt.Errorf("scan recall dream: %w", err)
		}

		out = append(out, d)
	}

	return out, rows.Err()
}

func RecallBriefings(ctx context.Context, conn *sql.DB) ([]RecallBriefing, error) {
	rows, err := conn.QueryContext(ctx, queries.RecallBriefings)
	if err != nil {
		return nil, fmt.Errorf("recall briefings: %w", err)
	}
	defer rows.Close()

	var out []RecallBriefing

	for rows.Next() {
		var b RecallBriefing

		err = rows.Scan(&b.Date, &b.Content)
		if err != nil {
			return nil, fmt.Errorf("scan recall briefing: %w", err)
		}

		out = append(out, b)
	}

	return out, rows.Err()
}
