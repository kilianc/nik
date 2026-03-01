package shell

import (
	"encoding/json"
	"fmt"
	"time"
)

type SessionMeta struct {
	Command        string    `json:"command"`
	Description    string    `json:"description"`
	ConversationID string    `json:"conversation_id,omitempty"`
	MessageID      string    `json:"message_id,omitempty"`
	RunID          string    `json:"run_id"`
	NextCheckAt    time.Time `json:"next_check_at"`
	StartedAt      time.Time `json:"started_at"`
}

const metaEnvKey = "NIK_META"

func loadMeta(id string) (SessionMeta, error) {
	raw, err := getEnv(id, metaEnvKey)
	if err != nil {
		return SessionMeta{}, fmt.Errorf("load meta %s: %w", id, err)
	}

	if raw == "" {
		return SessionMeta{}, nil
	}

	var m SessionMeta

	err = json.Unmarshal([]byte(raw), &m)
	if err != nil {
		return SessionMeta{}, fmt.Errorf("unmarshal meta %s: %w", id, err)
	}

	return m, nil
}

func saveMeta(id string, m SessionMeta) error {
	if m.Command == "" {
		return fmt.Errorf("save meta %s: empty command", id)
	}
	if m.RunID == "" {
		return fmt.Errorf("save meta %s: empty run_id", id)
	}
	if m.StartedAt.IsZero() {
		return fmt.Errorf("save meta %s: zero started_at", id)
	}
	if m.NextCheckAt.IsZero() {
		return fmt.Errorf("save meta %s: zero next_check_at", id)
	}

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal meta %s: %w", id, err)
	}

	err = setEnv(id, metaEnvKey, string(data))
	if err != nil {
		return fmt.Errorf("save meta %s: %w", id, err)
	}

	return nil
}
