package messaging

import (
	"testing"
	"time"
)

func TestInboundMessageCarriesCoreFields(t *testing.T) {
	now := time.Now()
	msg := InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation-1@s.whatsapp.net",
		ExternalMessageID:      "msg-1",
		ExternalSenderID:       "sender@s.whatsapp.net",
		SentAt:                 now,
		IsFromMe:               true,
	}

	if msg.Platform != "whatsapp" {
		t.Fatalf("expected platform whatsapp, got %q", msg.Platform)
	}
	if !msg.IsFromMe {
		t.Fatalf("expected IsFromMe to be true")
	}
	if msg.SentAt != now {
		t.Fatalf("expected sent timestamp to round-trip")
	}
}
