package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"

	_ "github.com/mattn/go-sqlite3"
)

type stats struct {
	total      int
	bodyOnly   int
	senderBody int
	full       int
}

func main() {
	home := flag.String("home", ".", "nik home directory")
	flag.Parse()

	dbPath := filepath.Join(*home, "nik.db")
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=1&_journal_mode=WAL")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()
	convIDs, err := allConversationIDs(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list conversations: %v\n", err)
		os.Exit(1)
	}

	contactsSvc := contacts.NewService(conn)
	svc := messaging.NewService(nil, conn, contactsSvc)

	var totals stats
	for _, convID := range convIDs {
		s := evaluate(ctx, svc, conn, convID)
		if s.total == 0 {
			continue
		}

		conv := convInfo(ctx, conn, convID)
		fmt.Printf("Conversation %s (%s, %d msgs)\n", convID, conv, s.total)
		fmt.Printf("  body only:            %d/%d unique (%d%%)\n", s.bodyOnly, s.total, pct(s.bodyOnly, s.total))
		fmt.Printf("  sender + body:        %d/%d unique (%d%%)\n", s.senderBody, s.total, pct(s.senderBody, s.total))
		fmt.Printf("  time + sender + body: %d/%d unique (%d%%)\n", s.full, s.total, pct(s.full, s.total))
		fmt.Println()

		totals.total += s.total
		totals.bodyOnly += s.bodyOnly
		totals.senderBody += s.senderBody
		totals.full += s.full
	}

	if totals.total > 0 {
		fmt.Printf("Total: %d messages across %d conversations\n", totals.total, len(convIDs))
		fmt.Printf("  body only:            %d/%d (%d%%)\n", totals.bodyOnly, totals.total, pct(totals.bodyOnly, totals.total))
		fmt.Printf("  sender + body:        %d/%d (%d%%)\n", totals.senderBody, totals.total, pct(totals.senderBody, totals.total))
		fmt.Printf("  time + sender + body: %d/%d (%d%%)\n", totals.full, totals.total, pct(totals.full, totals.total))
	}
}

func evaluate(ctx context.Context, svc *messaging.Service, conn *sql.DB, conversationID string) stats {
	msgs, err := db.GetMessagesByConversation(ctx, conn, conversationID, "", 200)
	if err != nil || len(msgs) == 0 {
		return stats{}
	}

	labels := svc.SenderLabels(ctx, msgs)

	type formatted struct {
		body       string
		senderBody string
		full       string
	}

	lines := make([]formatted, len(msgs))
	for i, msg := range msgs {
		sender := labels[msg.ID]
		text := bodyText(msg)
		ts := msg.SentAt.Format("15:04:05")

		lines[i] = formatted{
			body:       text,
			senderBody: fmt.Sprintf("%s: %s", sender, text),
			full:       fmt.Sprintf("[%s] %s: %s", ts, sender, text),
		}
	}

	var s stats
	s.total = len(msgs)

	for i := range msgs {
		if isUnique(lines, i, func(f formatted) string { return f.body }) {
			s.bodyOnly++
		}
		if isUnique(lines, i, func(f formatted) string { return f.senderBody }) {
			s.senderBody++
		}
		if isUnique(lines, i, func(f formatted) string { return f.full }) {
			s.full++
		}
	}

	return s
}

func bodyText(msg db.Message) string {
	text := strings.TrimSpace(msg.Body)
	if text == "" {
		text = "(" + msg.Kind + ")"
	}
	if msg.Kind == "reaction" {
		text = fmt.Sprintf("reaction %s", text)
	}
	if msg.IsEdit {
		text = fmt.Sprintf("edited: %s", text)
	}
	return text
}

func isUnique[T any](items []T, idx int, key func(T) string) bool {
	target := key(items[idx])
	for i, item := range items {
		if i != idx && key(item) == target {
			return false
		}
	}
	return true
}

func allConversationIDs(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx, "SELECT id FROM conversation ORDER BY last_message_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func convInfo(ctx context.Context, conn *sql.DB, id string) string {
	var kind string
	var title sql.NullString

	err := conn.QueryRowContext(ctx, "SELECT kind, title FROM conversation WHERE id = ?", id).Scan(&kind, &title)
	if err != nil {
		return kind
	}

	if title.Valid && title.String != "" {
		return fmt.Sprintf("%s %q", kind, title.String)
	}

	return kind
}

func pct(n, total int) int {
	if total == 0 {
		return 0
	}
	return n * 100 / total
}
