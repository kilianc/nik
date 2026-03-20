package queries

import (
	"strings"
	"testing"
)

func TestEmbeddedQueriesArePresent(t *testing.T) {
	// Standalone because this package-level embedding is isolated in queries, and
	// keeping a focused smoke assertion here avoids spreading import-path checks.
	required := map[string]string{
		"contact_upsert_whatsapp_insert": ContactUpsertWhatsAppInsert,
		"conversation_upsert":            ConversationUpsert,
		"message_insert":                 MessageInsert,
		"media_insert":                   MediaInsert,
		"alarm_insert":                   AlarmInsert,
	}

	for name, query := range required {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("expected embedded query %s to be non-empty", name)
		}
	}
}
