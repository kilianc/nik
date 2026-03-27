package llm

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
	CallID  string `json:"call_id,omitempty"`
}

func MarshalMessages(msgs []Message) string {
	if len(msgs) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(msgs)
	return string(data)
}

func UnmarshalMessages(s string) ([]Message, error) {
	if s == "" || s == "[]" {
		return nil, nil
	}
	var msgs []Message
	err := json.Unmarshal([]byte(s), &msgs)
	return msgs, err
}
