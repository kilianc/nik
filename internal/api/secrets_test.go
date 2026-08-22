package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSecrets struct {
	values map[string]string
	denied map[string]bool
	scopes []Scope
}

func newFakeSecrets() *fakeSecrets {
	return &fakeSecrets{
		values: map[string]string{"openai_key": "sk-test", "gateway_nik_key": "deadbeef"},
		denied: map[string]bool{"gateway_nik_key": true},
	}
}

func (f *fakeSecrets) List(context.Context) ([]string, error) {
	return []string{"openai_key", "gateway_nik_key"}, nil
}

func (f *fakeSecrets) Get(_ context.Context, scope Scope, name string) (string, error) {
	f.scopes = append(f.scopes, scope)

	if scope == ScopeSandbox && f.denied[name] {
		return "", ErrSecretDenied
	}
	value, ok := f.values[name]
	if !ok {
		return "", ErrNotFound
	}

	return value, nil
}

func (f *fakeSecrets) Set(_ context.Context, scope Scope, name, value string) error {
	f.scopes = append(f.scopes, scope)

	if scope == ScopeSandbox && f.denied[name] {
		return ErrSecretDenied
	}
	f.values[name] = value

	return nil
}

func (f *fakeSecrets) Delete(_ context.Context, name string) error {
	if _, ok := f.values[name]; !ok {
		return ErrNotFound
	}
	delete(f.values, name)

	return nil
}

// sandboxDo sends a request the way the shell container would: through the
// narrowed handler, which is the only thing that decides scope.
func sandboxDo(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}

	rec := httptest.NewRecorder()
	srv.SandboxHandler().ServeHTTP(rec, req)

	return rec
}

func TestOwnerCanReadASecret(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	rec := do(t, srv, http.MethodGet, "/v1/secrets/openai_key", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got secretResponse
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Value != "sk-test" {
		t.Fatalf("value = %q", got.Value)
	}
}

func TestSandboxCanReadAnAllowedSecret(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	rec := sandboxDo(t, srv, http.MethodGet, "/v1/secrets/openai_key", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// The whole point of the sandbox socket: nik's own identity is not something
// a skill may read, however it asks.
func TestSandboxCannotReadTheNikKey(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	rec := sandboxDo(t, srv, http.MethodGet, "/v1/secrets/gateway_nik_key", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "deadbeef") {
		t.Fatal("the denied value leaked into the response")
	}
}

// A denied name and a missing name answer identically, so a caller cannot map
// the store by watching which spellings come back 403 and which 404.
func TestDeniedAndMissingSecretsAreIndistinguishable(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	denied := sandboxDo(t, srv, http.MethodGet, "/v1/secrets/gateway_nik_key", "")
	missing := sandboxDo(t, srv, http.MethodGet, "/v1/secrets/no_such_thing", "")

	if denied.Code != missing.Code {
		t.Fatalf("denied = %d, missing = %d — they must not differ", denied.Code, missing.Code)
	}
	if denied.Body.String() != missing.Body.String() {
		t.Fatalf("denied body %q differs from missing body %q", denied.Body, missing.Body)
	}
}

// Listing is how you find out what is worth asking for.
func TestSandboxCannotListSecrets(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	rec := sandboxDo(t, srv, http.MethodGet, "/v1/secrets", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestOwnerCanListSecrets(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	rec := do(t, srv, http.MethodGet, "/v1/secrets", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// Everything not explicitly allowed is closed to the sandbox. If this ever
// starts failing because a new route was added, the new route is the bug.
func TestSandboxIsRefusedEverythingElse(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())
	srv.SetChat(&fakeChat{})
	srv.SetConfig(&fakeConfig{values: map[string]any{}})
	srv.SetGateway(&fakeGateway{})

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/v1/config"},
		{http.MethodPatch, "/v1/config"},
		{http.MethodPost, "/v1/gateway/connect"},
		{http.MethodGet, "/v1/conversations/local"},
		{http.MethodGet, "/v1/conversations/local/messages"},
		{http.MethodPost, "/v1/conversations/local/messages"},
		{http.MethodGet, "/v1/events"},
		{http.MethodDelete, "/v1/secrets/openai_key"},
	} {
		rec := sandboxDo(t, srv, tc.method, tc.target, `{"token":"x","body":"x","value":"x"}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403 from the sandbox", tc.method, tc.target, rec.Code)
		}
	}
}

// Health and version are open to the sandbox: a skill that wants to know
// whether nik is up learns nothing sensitive from the answer.
func TestSandboxCanCheckHealth(t *testing.T) {
	srv := New(NewState())

	for _, target := range []string{"/v1/health", "/v1/version"} {
		rec := sandboxDo(t, srv, http.MethodGet, target, "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", target, rec.Code)
		}
	}
}

// Scope comes from the listener, never from the request. Nothing a caller
// sends about itself may widen what it can reach.
func TestScopeCannotBeForgedFromTheRequest(t *testing.T) {
	srv := New(NewState())
	srv.SetSecrets(newFakeSecrets())

	req := httptest.NewRequest(http.MethodGet, "/v1/secrets", nil)
	req.Header.Set("X-Scope", string(ScopeOwner))
	req.Header.Set("Scope", string(ScopeOwner))

	rec := httptest.NewRecorder()
	srv.SandboxHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 — a header must not grant owner scope", rec.Code)
	}
}

func TestSandboxCanWriteAnAllowedSecret(t *testing.T) {
	store := newFakeSecrets()
	srv := New(NewState())
	srv.SetSecrets(store)

	rec := sandboxDo(t, srv, http.MethodPut, "/v1/secrets/hue_token", `{"value":"abc123"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if store.values["hue_token"] != "abc123" {
		t.Fatalf("stored %q", store.values["hue_token"])
	}
}

func TestSandboxCannotOverwriteTheNikKey(t *testing.T) {
	store := newFakeSecrets()
	srv := New(NewState())
	srv.SetSecrets(store)

	rec := sandboxDo(t, srv, http.MethodPut, "/v1/secrets/gateway_nik_key", `{"value":"mine now"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if store.values["gateway_nik_key"] != "deadbeef" {
		t.Fatal("the sandbox overwrote nik's nik key")
	}
}
