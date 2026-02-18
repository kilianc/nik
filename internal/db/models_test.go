package db

import "testing"

func TestModelZeroValues(t *testing.T) {
	var contact Contact
	var conversation Conversation
	var message Message
	var media Media
	var alarm Alarm

	if contact.ID != "" {
		t.Fatalf("expected zero contact id")
	}
	if conversation.Platform != "" {
		t.Fatalf("expected zero conversation platform")
	}
	if message.ExternalMessageID != "" {
		t.Fatalf("expected zero external message id")
	}
	if media.ID != "" {
		t.Fatalf("expected zero media id")
	}
	if alarm.Goal != "" {
		t.Fatalf("expected zero alarm goal")
	}
}
