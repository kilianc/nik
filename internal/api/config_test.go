package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type fakeConfig struct {
	values map[string]any
	set    []ConfigField
	err    error
}

func (f *fakeConfig) Get(context.Context) (map[string]any, error) {
	return f.values, f.err
}

func (f *fakeConfig) Set(_ context.Context, field, value string) error {
	if f.err != nil {
		return f.err
	}
	f.set = append(f.set, ConfigField{Field: field, Value: value})

	return nil
}

type fakeGateway struct {
	url   string
	token string
	err   error
}

func (f *fakeGateway) Connect(_ context.Context, url, token string) error {
	f.url, f.token = url, token

	return f.err
}

func TestConfigGetReturnsTheConfig(t *testing.T) {
	srv := New(NewState())
	srv.SetConfig(&fakeConfig{values: map[string]any{"timezone": "Europe/Rome"}})

	rec := do(t, srv, http.MethodGet, "/v1/config", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["timezone"] != "Europe/Rome" {
		t.Fatalf("timezone = %v", got["timezone"])
	}
}

// Several fields in one call, so a client can change a model and its
// reasoning effort without a window where only half of it is true.
func TestConfigPatchAppliesEveryField(t *testing.T) {
	cfg := &fakeConfig{values: map[string]any{}}
	srv := New(NewState())
	srv.SetConfig(cfg)

	rec := do(t, srv, http.MethodPatch, "/v1/config",
		`{"set":[{"field":"models.main.model","value":"sol"},{"field":"models.main.reasoning_effort","value":"high"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(cfg.set) != 2 {
		t.Fatalf("set %d fields, want 2", len(cfg.set))
	}
	if cfg.set[0].Field != "models.main.model" || cfg.set[1].Value != "high" {
		t.Fatalf("set = %+v", cfg.set)
	}
}

// A typo'd field is the client's mistake. Reporting it as a server error
// would send someone reading nikd's logs for their own spelling error.
func TestConfigPatchRejectsBadFieldsWith400(t *testing.T) {
	srv := New(NewState())
	srv.SetConfig(&fakeConfig{err: ErrInvalidField})

	rec := do(t, srv, http.MethodPatch, "/v1/config", `{"set":[{"field":"nope","value":"x"}]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConfigPatchRejectsAnEmptySet(t *testing.T) {
	srv := New(NewState())
	srv.SetConfig(&fakeConfig{values: map[string]any{}})

	rec := do(t, srv, http.MethodPatch, "/v1/config", `{"set":[]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConnectPassesTokenAndURL(t *testing.T) {
	gw := &fakeGateway{}
	srv := New(NewState())
	srv.SetGateway(gw)

	rec := do(t, srv, http.MethodPost, "/v1/gateway/connect",
		`{"token":"nik_abc","url":"wss://gw.example/v1/nik"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if gw.token != "nik_abc" || gw.url != "wss://gw.example/v1/nik" {
		t.Fatalf("gateway got token=%q url=%q", gw.token, gw.url)
	}
}

// A rejected token is 401 and not 400: the request was fine and the
// credential was not, and only one of those has a remedy a person can act on.
func TestConnectRejectedTokenIs401(t *testing.T) {
	srv := New(NewState())
	srv.SetGateway(&fakeGateway{err: ErrAuthRejected})

	rec := do(t, srv, http.MethodPost, "/v1/gateway/connect", `{"token":"expired"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestConnectRequiresAToken(t *testing.T) {
	srv := New(NewState())
	srv.SetGateway(&fakeGateway{})

	rec := do(t, srv, http.MethodPost, "/v1/gateway/connect", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Connect is reachable before there is any config — that is the entire point
// of it, and the one endpoint that must work on a nik that has nothing.
func TestConnectIsReachableWithoutConfig(t *testing.T) {
	srv := New(NewState())
	srv.SetGateway(&fakeGateway{})

	rec := do(t, srv, http.MethodPost, "/v1/gateway/connect", `{"token":"nik_abc"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 with no config present", rec.Code)
	}
}

func TestConfigIs503BeforeConfigExists(t *testing.T) {
	srv := New(NewState())

	rec := do(t, srv, http.MethodGet, "/v1/config", "")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
