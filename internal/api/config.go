package api

import (
	"context"
	"errors"
	"net/http"
)

// Config is nikd's configuration, as something a client may read and change.
//
// Deliberately field-at-a-time rather than PUT-the-whole-document: config.yaml
// carries comments and ordering a human wrote, the daemon live-reloads it, and
// a client that round-trips the whole file would flatten both. It is the same
// shape the `config` brain tool has always used, which is not a coincidence —
// it is the same operation, reached differently.
type Config interface {
	Get(ctx context.Context) (map[string]any, error)
	Set(ctx context.Context, field, value string) error
}

// Gateway is the account link: a token in, a probe, and a daemon that
// converges without being restarted.
type Gateway interface {
	Connect(ctx context.Context, url, token string) error
}

// ErrInvalidField is a client's mistake, not nikd's: an unknown or read-only
// config field, or a value that does not parse.
var ErrInvalidField = errors.New("invalid field")

// ErrAuthRejected is a token the gateway refused. It is separated from every
// other connect failure because it is the one with a specific remedy — make a
// new agent on the dashboard — rather than "try again".
var ErrAuthRejected = errors.New("gateway rejected the token")

func (s *Server) SetConfig(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = cfg
}

func (s *Server) SetGateway(gw Gateway) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gateway = gw
}

func (s *Server) currentConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

func (s *Server) currentGateway() Gateway {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gateway
}

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentConfig()
	if cfg == nil {
		writeError(w, http.StatusServiceUnavailable, "nik has no configuration yet")
		return
	}

	out, err := cfg.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// ConfigPatch sets one field. A list rather than a single pair so a client can
// apply a related set — a model and its reasoning effort — without a window
// where only half of it is true.
type ConfigPatch struct {
	Set []ConfigField `json:"set"`
}

type ConfigField struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

func (s *Server) handleConfigPatch(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentConfig()
	if cfg == nil {
		writeError(w, http.StatusServiceUnavailable, "nik has no configuration yet")
		return
	}

	patch, err := readJSON[ConfigPatch](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(patch.Set) == 0 {
		writeError(w, http.StatusBadRequest, "set is required")
		return
	}

	for _, field := range patch.Set {
		err = cfg.Set(r.Context(), field.Field, field.Value)
		if errors.Is(err, ErrInvalidField) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	out, err := cfg.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// The whole config back, not just what changed: a client that just set a
	// model wants to see what the daemon actually holds, including anything
	// normalization did to it.
	writeJSON(w, http.StatusOK, out)
}

// ConnectRequest links this nik to an account.
type ConnectRequest struct {
	Token string `json:"token"`
	// URL is optional; empty keeps whatever is configured, or falls back to
	// the production gateway on a fresh install.
	URL string `json:"url,omitempty"`
}

// handleGatewayConnect is what `nikctl connect` and, later, a provisioner
// call. It probes before storing, so a 200 means the token worked rather than
// that it was written down.
func (s *Server) handleGatewayConnect(w http.ResponseWriter, r *http.Request) {
	gw := s.currentGateway()
	if gw == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	req, err := readJSON[ConnectRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	err = gw.Connect(r.Context(), req.URL, req.Token)
	if errors.Is(err, ErrAuthRejected) {
		// 401 rather than 400: the request was well formed and the credential
		// was not, and a client can tell a person exactly what to do about it.
		writeError(w, http.StatusUnauthorized,
			"the gateway rejected that token — it may have expired (they last 15 minutes); make a new agent on your dashboard")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
