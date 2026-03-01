package shell

import (
	"testing"
	"time"
)

func TestSaveAndLoadMeta(t *testing.T) {
	requireTmux(t)

	id := "test-meta"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	want := SessionMeta{
		Command:        "sleep 60",
		Description:    "test command",
		ConversationID: "conv-123",
		MessageID:      "msg-456",
		RunID:          "run-789",
		NextCheckAt:    now.Add(5 * time.Minute),
		StartedAt:      now,
	}

	err = saveMeta(id, want)
	if err != nil {
		t.Fatalf("saveMeta: %v", err)
	}

	got, err := loadMeta(id)
	if err != nil {
		t.Fatalf("loadMeta: %v", err)
	}

	if got.Command != want.Command {
		t.Fatalf("command: got %q, want %q", got.Command, want.Command)
	}
	if got.Description != want.Description {
		t.Fatalf("description: got %q, want %q", got.Description, want.Description)
	}
	if got.ConversationID != want.ConversationID {
		t.Fatalf("conversation_id: got %q, want %q", got.ConversationID, want.ConversationID)
	}
	if got.MessageID != want.MessageID {
		t.Fatalf("message_id: got %q, want %q", got.MessageID, want.MessageID)
	}
	if got.RunID != want.RunID {
		t.Fatalf("run_id: got %q, want %q", got.RunID, want.RunID)
	}
	if !got.NextCheckAt.Equal(want.NextCheckAt) {
		t.Fatalf("next_check_at: got %v, want %v", got.NextCheckAt, want.NextCheckAt)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Fatalf("started_at: got %v, want %v", got.StartedAt, want.StartedAt)
	}
}

func TestSaveMetaValidation(t *testing.T) {
	requireTmux(t)

	id := "test-meta-validation"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	now := time.Now().UTC()
	valid := SessionMeta{
		Command:     "sleep 60",
		RunID:       "run-1",
		StartedAt:   now,
		NextCheckAt: now.Add(5 * time.Minute),
	}

	missing := func(fn func(m *SessionMeta)) SessionMeta {
		m := valid
		fn(&m)
		return m
	}

	cases := []struct {
		name string
		meta SessionMeta
	}{
		{"empty command", missing(func(m *SessionMeta) { m.Command = "" })},
		{"empty run_id", missing(func(m *SessionMeta) { m.RunID = "" })},
		{"zero started_at", missing(func(m *SessionMeta) { m.StartedAt = time.Time{} })},
		{"zero next_check_at", missing(func(m *SessionMeta) { m.NextCheckAt = time.Time{} })},
	}

	for _, tc := range cases {
		err := saveMeta(id, tc.meta)
		if err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestLoadMetaMissingSession(t *testing.T) {
	requireTmux(t)

	_, err := loadMeta("nonexistent-session-id")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}
