package journal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

const defaultMessageLimit = 200

func buildDayContext(ctx context.Context, conn *sql.DB, msgsSvc *messaging.Service, dayStart, dayEnd time.Time) []string {
	var lines []string

	briefing := briefingSection(ctx, conn, dayStart)
	convos := conversationSection(ctx, conn, dayStart, dayEnd)
	msgs := messagesSection(ctx, conn, msgsSvc, dayStart, dayEnd)
	contacts := contactsSection(ctx, conn, dayStart, dayEnd)
	memories := memoriesSection(ctx, conn, dayStart, dayEnd)

	if len(briefing) > 0 {
		lines = append(lines, briefing...)
	}

	if len(convos) > 0 {
		lines = append(lines, convos...)
	}

	if len(msgs) > 0 {
		lines = append(lines, msgs...)
	}

	if len(contacts) > 0 {
		lines = append(lines, contacts...)
	}

	if len(memories) > 0 {
		lines = append(lines, memories...)
	}

	return lines
}

func conversationSection(ctx context.Context, conn *sql.DB, dayStart, dayEnd time.Time) []string {
	convos, err := db.JournalConversationsToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		slog.Warn("journal context: conversations", "error", err)
		return nil
	}

	if len(convos) == 0 {
		return []string{"## Conversations", "", "No conversations today.", ""}
	}

	lines := []string{"## Conversations", ""}
	for _, c := range convos {
		title := c.Kind
		if c.Title.Valid && c.Title.String != "" {
			title = c.Title.String
		}

		lines = append(lines, fmt.Sprintf("- %s (%s, %d messages)", title, c.Platform, c.MessageCount))
	}

	lines = append(lines, "")
	return lines
}

func messagesSection(ctx context.Context, conn *sql.DB, msgsSvc *messaging.Service, dayStart, dayEnd time.Time) []string {
	msgs, err := db.JournalMessagesToday(ctx, conn, dayStart, dayEnd, defaultMessageLimit)
	if err != nil {
		slog.Warn("journal context: messages", "error", err)
		return nil
	}

	if len(msgs) == 0 {
		return nil
	}

	senderLabels := msgsSvc.SenderLabels(ctx, msgs)

	lines := []string{"## Messages", ""}
	for _, m := range msgs {
		label := senderLabels[m.ID]
		line := messaging.FormatMessageLine(m, label)
		if line != "" {
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	return lines
}

func contactsSection(ctx context.Context, conn *sql.DB, dayStart, dayEnd time.Time) []string {
	contacts, err := db.JournalContactsToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		slog.Warn("journal context: contacts", "error", err)
		return nil
	}

	if len(contacts) == 0 {
		return nil
	}

	lines := []string{"## New people", ""}
	for _, c := range contacts {
		parts := []string{fmt.Sprintf("- %s", contactDisplayName(c))}

		if c.OneLiner.Valid && c.OneLiner.String != "" {
			parts = append(parts, fmt.Sprintf(" — %s", c.OneLiner.String))
		}

		lines = append(lines, strings.Join(parts, ""))
	}

	lines = append(lines, "")
	return lines
}

func contactDisplayName(c db.JournalContact) string {
	if c.Name != "" {
		return c.Name
	}

	if len(c.Nicknames) > 0 {
		return c.Nicknames[0]
	}

	return c.ID
}

func memoriesSection(ctx context.Context, conn *sql.DB, dayStart, dayEnd time.Time) []string {
	memories, err := db.JournalMemoriesToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		slog.Warn("journal context: memories", "error", err)
		return nil
	}

	if len(memories) == 0 {
		return nil
	}

	lines := []string{"## Memories formed today", ""}
	for _, m := range memories {
		ts := m.CreatedAt.Format("15:04")
		lines = append(lines, fmt.Sprintf("- [%s] %s", ts, m.Content))
	}

	lines = append(lines, "")
	return lines
}

func briefingSection(ctx context.Context, conn *sql.DB, dayStart time.Time) []string {
	date := dayStart.Format("2006-01-02")

	content, err := db.BriefingGetPage(ctx, conn, date)
	if err != nil {
		slog.Warn("journal context: briefing", "error", err)
		return nil
	}

	if content == "" {
		return nil
	}

	return []string{"## Morning briefing", "", content, ""}
}
