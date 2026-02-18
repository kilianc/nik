package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kciuffolo/nik/internal/queries"
)

func TestNewIDReturnsUUIDv7(t *testing.T) {
	id := NewID()

	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}

	if parsed.Version() != 7 {
		t.Fatalf("expected uuid v7, got v%d", parsed.Version())
	}
}

func TestOpenInMemoryAppliesSchema(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	var tableName string
	err = conn.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='contact'").Scan(&tableName)
	if err != nil {
		t.Fatalf("query table metadata: %v", err)
	}

	if tableName != "contact" {
		t.Fatalf("expected contact table to exist, got %q", tableName)
	}
}

type insertTestMessageParams struct {
	ConversationID         string
	ContactID              string
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	SentAt                 time.Time
	IsFromMe               bool
	Kind                   string
	Body                   string
}

func insertTestMessage(t *testing.T, ctx context.Context, conn *sql.DB, p insertTestMessageParams) string {
	t.Helper()

	if p.Kind == "" {
		p.Kind = "text"
	}
	if p.SentAt.IsZero() {
		p.SentAt = time.Now()
	}

	id := NewID()
	_, err := conn.ExecContext(
		ctx,
		queries.MessageInsert,
		id,
		p.ConversationID,
		p.ContactID,
		p.Platform,
		p.ExternalConversationID,
		p.ExternalMessageID,
		p.ExternalSenderID,
		p.SentAt,
		p.IsFromMe,
		false,
		p.Kind,
		p.Body,
		nil,
		false,
		nil,
		nil,
		nil,
		false,
		nil,
		MarshalStringSlice([]string{}),
		false,
		false,
	)
	if err != nil {
		t.Fatalf("insert test message: %v", err)
	}

	if p.IsFromMe {
		_, err = conn.ExecContext(ctx, queries.ConversationMarkRead, p.ConversationID, p.SentAt)
		if err != nil {
			t.Fatalf("mark conversation read on outbound: %v", err)
		}
	}

	row := conn.QueryRowContext(ctx, queries.MessageGet, "", p.Platform, p.ExternalMessageID)
	msg, scanErr := scanMessage(row)
	if scanErr != nil {
		t.Fatalf("read back inserted message: %v", scanErr)
	}

	return msg.ID
}

// seedConversation creates a conversation and returns its canonical ID.
func seedConversation(t *testing.T, ctx context.Context, conn *sql.DB, platform, externalID, kind string) string {
	t.Helper()

	now := time.Now()
	err := UpsertConversation(ctx, conn, UpsertConversationParams{
		Platform:               platform,
		ExternalConversationID: externalID,
		Kind:                   kind,
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := GetConversation(ctx, conn, GetConversationParams{
		Platform:               platform,
		ExternalConversationID: externalID,
	})
	if err != nil {
		t.Fatalf("get seeded conversation: %v", err)
	}

	return conv.ID
}

func TestOpenCreatesDatabasePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "nik.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatalf("open db file: %v", err)
	}
	defer conn.Close()

	_, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat db path: %v", statErr)
	}
}
