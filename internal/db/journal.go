package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func JournalHasPage(ctx context.Context, db *sql.DB, date string) (bool, error) {
	var exists int

	err := db.QueryRowContext(ctx, queries.JournalCheck, date).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check journal page %s: %w", date, err)
	}

	return true, nil
}

func JournalGetPage(ctx context.Context, db *sql.DB, date string) (string, error) {
	var content string

	err := db.QueryRowContext(ctx, queries.JournalGet, date).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get journal page %s: %w", date, err)
	}

	return content, nil
}

func JournalStartPage(ctx context.Context, db *sql.DB, date string) error {
	_, err := db.ExecContext(ctx, queries.JournalStart, date)
	if err != nil {
		return fmt.Errorf("start journal page %s: %w", date, err)
	}

	return nil
}

func JournalWritePage(ctx context.Context, db *sql.DB, date, content string) error {
	_, err := db.ExecContext(ctx, queries.JournalWrite, date, content)
	if err != nil {
		return fmt.Errorf("write journal page %s: %w", date, err)
	}

	return nil
}

func JournalConversationsToday(ctx context.Context, db *sql.DB, dayStart, dayEnd time.Time) ([]JournalConversation, error) {
	rows, err := db.QueryContext(ctx, queries.JournalConversationsToday, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("journal conversations today: %w", err)
	}
	defer rows.Close()

	var out []JournalConversation
	for rows.Next() {
		var c JournalConversation

		err = rows.Scan(
			&c.ID,
			&c.Platform,
			&c.Kind,
			&c.Title,
			&c.MessageCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan journal conversation: %w", err)
		}

		out = append(out, c)
	}

	return out, rows.Err()
}

func JournalMessagesToday(ctx context.Context, db *sql.DB, dayStart, dayEnd time.Time, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := db.QueryContext(ctx, queries.JournalMessagesToday, dayStart, dayEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("journal messages today: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		m, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, m)
	}

	return out, rows.Err()
}

func JournalContactsToday(ctx context.Context, db *sql.DB, dayStart, dayEnd time.Time) ([]JournalContact, error) {
	rows, err := db.QueryContext(ctx, queries.JournalContactsToday, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("journal contacts today: %w", err)
	}
	defer rows.Close()

	var out []JournalContact
	for rows.Next() {
		var c JournalContact
		var nicknames any

		err = rows.Scan(
			&c.ID,
			&c.Name,
			&nicknames,
			&c.OneLiner,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan journal contact: %w", err)
		}

		c.Nicknames, err = scanStringSlice(nicknames)
		if err != nil {
			return nil, fmt.Errorf("scan journal contact nicknames: %w", err)
		}

		out = append(out, c)
	}

	return out, rows.Err()
}

func JournalCrewToday(ctx context.Context, db *sql.DB, dayStart, dayEnd time.Time) ([]JournalCrewHire, error) {
	rows, err := db.QueryContext(ctx, queries.JournalCrewToday, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("journal crew today: %w", err)
	}
	defer rows.Close()

	var out []JournalCrewHire
	for rows.Next() {
		var c JournalCrewHire

		err = rows.Scan(
			&c.ID,
			&c.Name,
			&c.Prompt,
			&c.CreatedAt,
			&c.TaskCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan journal crew hire: %w", err)
		}

		out = append(out, c)
	}

	return out, rows.Err()
}

func JournalMemoriesToday(ctx context.Context, db *sql.DB, dayStart, dayEnd time.Time) ([]JournalMemory, error) {
	rows, err := db.QueryContext(ctx, queries.JournalMemoriesToday, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("journal memories today: %w", err)
	}
	defer rows.Close()

	var out []JournalMemory
	for rows.Next() {
		var m JournalMemory

		err = rows.Scan(
			&m.ID,
			&m.Content,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan journal memory: %w", err)
		}

		out = append(out, m)
	}

	return out, rows.Err()
}
