package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Chat is what nikd plugs in once config has loaded and the database is open.
//
// It is an interface here, and implemented in internal/apisvc, so this package
// keeps importing nothing heavier than net/http. That matters because
// internal/nikapi — which nikctl links — imports this one for its types: an
// api package that reached for internal/db would put SQLite back inside the
// client binary and undo the split.
type Chat interface {
	Conversation(ctx context.Context, id string) (Conversation, error)
	Messages(ctx context.Context, p MessagesQuery) ([]Message, error)
	Send(ctx context.Context, conversationID, body string) error
}

// ErrNotFound lets an implementation say "no such conversation" without this
// package knowing what a database is.
var ErrNotFound = errors.New("not found")

// LocalConversationID is how the API spells the conversation a person talks to
// nik in directly — the TUI's chat, and the one a browser console renders.
//
// It is an alias, not the row's id. The database keys that conversation by a
// fixed UUID, and a console URL carrying a UUID nobody can type is worse than
// one that says what it is. Implementations resolve both spellings.
const LocalConversationID = "local"

// Conversation is one thread, as the API describes it.
type Conversation struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	// Activity is what nik is visibly doing right now — "typing" and friends.
	// A console renders it as a live indicator, which is why it rides on the
	// conversation rather than being inferred from message timing.
	Activity      []string  `json:"activity"`
	LastMessageAt time.Time `json:"last_message_at,omitzero"`
}

// Message is one message, flattened. The database model carries a dozen
// sql.Null fields and platform routing ids that no client has any use for;
// what survives here is what a reader needs to render a conversation.
type Message struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	SentAt    time.Time `json:"sent_at"`
	IsFromMe  bool      `json:"is_from_me"`
	Platform  string    `json:"platform"`
	ContactID string    `json:"contact_id,omitempty"`
	// MediaID, when set, is fetched separately rather than inlined: nik sends
	// voice notes and images, and base64 down a socket that later becomes a
	// tunnel is the wrong shape.
	MediaID string `json:"media_id,omitempty"`
	// Transcript and Description are what nik understood of the media, which
	// is often the only part worth rendering as text.
	Transcript  string `json:"transcript,omitempty"`
	Description string `json:"description,omitempty"`
}

// MessagesQuery pages a conversation.
//
// Forward-only for now: After is the newest id a client already has, which is
// what an incremental reader needs and what the TUI has always done. Backward
// paging for deep history is a separate query and waits for the console that
// needs it — a parameter with no consumer is a guess about what it should do.
type MessagesQuery struct {
	ConversationID string
	After          string
	Limit          int
}

const (
	defaultMessageLimit = 100
	maxMessageLimit     = 500
)

// SetChat plugs in the live implementation. Before it is called — the whole
// pre-config window — the chat endpoints answer 503 rather than 404, because
// "not yet" and "no such route" are different things to a client deciding
// whether to retry.
func (s *Server) SetChat(chat Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chat = chat
}

func (s *Server) currentChat() Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.chat
}

func (s *Server) handleConversationGet(w http.ResponseWriter, r *http.Request) {
	chat := s.currentChat()
	if chat == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	conv, err := chat.Conversation(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such conversation")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, conv)
}

type messagesResponse struct {
	Messages []Message `json:"messages"`
}

func (s *Server) handleMessagesList(w http.ResponseWriter, r *http.Request) {
	chat := s.currentChat()
	if chat == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	messages, err := chat.Messages(r.Context(), MessagesQuery{
		ConversationID: r.PathValue("id"),
		After:          r.URL.Query().Get("after"),
		Limit:          limit,
	})
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such conversation")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Never null: a client that has to distinguish an empty page from a
	// missing field is one bug waiting to happen.
	if messages == nil {
		messages = []Message{}
	}

	writeJSON(w, http.StatusOK, messagesResponse{Messages: messages})
}

// SendRequest is the body of POST /v1/conversations/{id}/messages.
type SendRequest struct {
	Body string `json:"body"`
}

// handleMessageSend is the local chat's in-door. It does exactly what the TUI
// does today — except in the process that owns the brain, so the message and
// the activation that answers it stop being two processes racing on one file.
func (s *Server) handleMessageSend(w http.ResponseWriter, r *http.Request) {
	chat := s.currentChat()
	if chat == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	req, err := readJSON[SendRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	err = chat.Send(r.Context(), r.PathValue("id"), req.Body)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such conversation")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 202, not 201: the message is recorded, and what happens next — nik
	// reading it, thinking, answering — has not happened yet and is watched
	// on the conversation rather than awaited here.
	w.WriteHeader(http.StatusAccepted)
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultMessageLimit, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be a number")
	}
	if n <= 0 {
		return 0, errors.New("limit must be positive")
	}
	if n > maxMessageLimit {
		return maxMessageLimit, nil
	}

	return n, nil
}
