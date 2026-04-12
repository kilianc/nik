package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type orphanedStart struct {
	ID             string
	ConversationID string
	Body           string
	SentAt         time.Time
}

func ToolCallStartRecover(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, queries.ToolCallStartOrphaned)
	if err != nil {
		return fmt.Errorf("query orphaned tool call starts: %w", err)
	}
	defer rows.Close()

	var orphans []orphanedStart
	for rows.Next() {
		var o orphanedStart
		var sentAt string
		err = rows.Scan(&o.ID, &o.ConversationID, &o.Body, &sentAt)
		if err != nil {
			return fmt.Errorf("scan orphaned tool call start: %w", err)
		}
		o.SentAt, err = parseTimestampString(sentAt)
		if err != nil {
			return fmt.Errorf("parse sent_at for %s: %w", o.ID, err)
		}
		orphans = append(orphans, o)
	}

	err = rows.Err()
	if err != nil {
		return fmt.Errorf("iterate orphaned tool call starts: %w", err)
	}

	for _, o := range orphans {
		var start struct {
			Name  string `json:"name"`
			Input string `json:"input"`
			Round int    `json:"round"`
		}
		json.Unmarshal([]byte(o.Body), &start)

		body := struct {
			Name   string `json:"name"`
			Input  string `json:"input"`
			Output string `json:"output"`
			Round  int    `json:"round"`
		}{
			Name:   start.Name,
			Input:  start.Input,
			Output: `{"error":"interrupted"}`,
			Round:  start.Round,
		}

		_, err = SystemMessageInsert(ctx, db, SystemMessageParams{
			ConversationID:  o.ConversationID,
			Kind:            "tool_call",
			Body:            body,
			SentAt:          o.SentAt,
			ContextStanzaID: o.ID,
		})
		if err != nil {
			slog.Warn("recover orphaned tool call start", "id", o.ID, "error", err)
			continue
		}
		slog.Info("recovered orphaned tool call start", "id", o.ID, "tool", start.Name)
	}

	return nil
}
