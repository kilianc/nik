package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeChat struct {
	conv     Conversation
	messages []Message
	query    MessagesQuery
	sent     []string
	sendTo   string
	err      error
}

func (f *fakeChat) Conversation(_ context.Context, id string) (Conversation, error) {
	if f.err != nil {
		return Conversation{}, f.err
	}
	f.conv.ID = id

	return f.conv, nil
}

func (f *fakeChat) Messages(_ context.Context, p MessagesQuery) ([]Message, error) {
	f.query = p

	return f.messages, f.err
}

func (f *fakeChat) Send(_ context.Context, convID, body string) error {
	if f.err != nil {
		return f.err
	}
	f.sendTo = convID
	f.sent = append(f.sent, body)

	return nil
}

func do(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	return rec
}

// The pre-config window is real — nikd serves before the database is open —
// so a client has to be able to tell "not yet" from "no such thing". 503 is
// retryable; 404 is not.
func TestChatEndpointsAre503BeforeTheDatabaseIsOpen(t *testing.T) {
	srv := New(NewState())

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/v1/conversations/local"},
		{http.MethodGet, "/v1/conversations/local/messages"},
		{http.MethodPost, "/v1/conversations/local/messages"},
	} {
		rec := do(t, srv, tc.method, tc.target, `{"body":"hi"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503", tc.method, tc.target, rec.Code)
		}
	}
}

func TestMessagesListReturnsMessages(t *testing.T) {
	chat := &fakeChat{messages: []Message{
		{ID: "1", Kind: "text", Body: "morning", SentAt: time.Now()},
		{ID: "2", Kind: "text", Body: "morning yourself", IsFromMe: true, SentAt: time.Now()},
	}}
	srv := New(NewState())
	srv.SetChat(chat)

	rec := do(t, srv, http.MethodGet, "/v1/conversations/local/messages?after=abc&limit=25", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got messagesResponse
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	if chat.query.After != "abc" || chat.query.Limit != 25 {
		t.Fatalf("query = %+v, want after=abc limit=25", chat.query)
	}
	if chat.query.ConversationID != "local" {
		t.Fatalf("conversation = %q, want local", chat.query.ConversationID)
	}
}

// An empty conversation must encode as [] rather than null, or every client
// grows a nil check that only fires on a fresh install.
func TestEmptyMessagesEncodeAsArray(t *testing.T) {
	srv := New(NewState())
	srv.SetChat(&fakeChat{})

	rec := do(t, srv, http.MethodGet, "/v1/conversations/local/messages", "")

	if !strings.Contains(rec.Body.String(), `"messages":[]`) {
		t.Fatalf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestMessagesLimitIsClamped(t *testing.T) {
	chat := &fakeChat{}
	srv := New(NewState())
	srv.SetChat(chat)

	do(t, srv, http.MethodGet, "/v1/conversations/local/messages?limit=99999", "")

	if chat.query.Limit != maxMessageLimit {
		t.Fatalf("limit = %d, want it clamped to %d", chat.query.Limit, maxMessageLimit)
	}
}

func TestMessagesRejectsNonsenseLimit(t *testing.T) {
	srv := New(NewState())
	srv.SetChat(&fakeChat{})

	for _, raw := range []string{"banana", "-1", "0"} {
		rec := do(t, srv, http.MethodGet, "/v1/conversations/local/messages?limit="+raw, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%s = %d, want 400", raw, rec.Code)
		}
	}
}

func TestSendRecordsTheMessage(t *testing.T) {
	chat := &fakeChat{}
	srv := New(NewState())
	srv.SetChat(chat)

	rec := do(t, srv, http.MethodPost, "/v1/conversations/local/messages", `{"body":"what's for dinner"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if len(chat.sent) != 1 || chat.sent[0] != "what's for dinner" {
		t.Fatalf("sent = %v, want the body", chat.sent)
	}
	if chat.sendTo != "local" {
		t.Fatalf("sent to %q, want local", chat.sendTo)
	}
}

func TestSendRejectsAnEmptyBody(t *testing.T) {
	srv := New(NewState())
	srv.SetChat(&fakeChat{})

	for _, body := range []string{`{}`, `{"body":""}`} {
		rec := do(t, srv, http.MethodPost, "/v1/conversations/local/messages", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s = %d, want 400", body, rec.Code)
		}
	}
}

// A typo'd field is a client bug, and silently dropping it means the message
// nik never received looks like one it ignored.
func TestSendRejectsUnknownFields(t *testing.T) {
	srv := New(NewState())
	srv.SetChat(&fakeChat{})

	rec := do(t, srv, http.MethodPost, "/v1/conversations/local/messages", `{"text":"wrong field name"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConversationNotFoundIs404(t *testing.T) {
	srv := New(NewState())
	srv.SetChat(&fakeChat{err: ErrNotFound})

	rec := do(t, srv, http.MethodGet, "/v1/conversations/nope", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
