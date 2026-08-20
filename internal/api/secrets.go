package api

import (
	"context"
	"errors"
	"net/http"
)

// Secrets is the encrypted store, reached through a door rather than through
// the filesystem.
type Secrets interface {
	List(ctx context.Context) ([]string, error)
	Get(ctx context.Context, scope Scope, name string) (string, error)
	Set(ctx context.Context, scope Scope, name, value string) error
	Delete(ctx context.Context, name string) error
}

// ErrSecretDenied is a secret this caller may not have. Distinct from
// ErrNotFound on purpose — and answered with the same 404, see below.
var ErrSecretDenied = errors.New("not available to this caller")

func (s *Server) SetSecrets(secrets Secrets) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secrets = secrets
}

func (s *Server) currentSecrets() Secrets {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.secrets
}

type secretsListResponse struct {
	Names []string `json:"names"`
}

func (s *Server) handleSecretsList(w http.ResponseWriter, r *http.Request) {
	store := s.currentSecrets()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	names, err := store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}

	writeJSON(w, http.StatusOK, secretsListResponse{Names: names})
}

type secretResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (s *Server) handleSecretGet(w http.ResponseWriter, r *http.Request) {
	store := s.currentSecrets()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	name := r.PathValue("name")

	value, err := store.Get(r.Context(), scopeOf(r), name)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, secretResponse{Name: name, Value: value})
}

// SecretRequest is the body of PUT /v1/secrets/{name}.
type SecretRequest struct {
	Value string `json:"value"`
}

func (s *Server) handleSecretSet(w http.ResponseWriter, r *http.Request) {
	store := s.currentSecrets()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	req, err := readJSON[SecretRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	err = store.Set(r.Context(), scopeOf(r), r.PathValue("name"), req.Value)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSecretDelete(w http.ResponseWriter, r *http.Request) {
	store := s.currentSecrets()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	err := store.Delete(r.Context(), r.PathValue("name"))
	if err != nil {
		writeSecretError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeSecretError answers a denied secret and a missing one identically, and
// that is the point. A 403 on `gateway_agent_key` and a 404 on `gateway_agnet_key`
// tells a caller which names are real, which is most of what it wanted to know
// before it started guessing. The daemon logs the difference; the caller does
// not learn it.
func writeSecretError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrSecretDenied):
		writeError(w, http.StatusNotFound, "no such secret")
	case errors.Is(err, ErrInvalidField):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
