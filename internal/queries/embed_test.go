package queries

import (
	"strings"
	"testing"
)

func TestEmbeddedQueriesArePresent(t *testing.T) {
	required := map[string]string{
		"contact_upsert_whatsapp_insert": ContactUpsertWhatsAppInsert,
		"conversation_upsert":            ConversationUpsert,
		"message_insert":                 MessageInsert,
		"media_upsert":                   MediaUpsert,
		"create_alarm":                   CreateAlarm,
	}

	for name, query := range required {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("expected embedded query %s to be non-empty", name)
		}
	}
}
