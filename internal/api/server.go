// Package api is nikd's one way in.
//
// The rule the whole design rests on: nikd owns NIK_HOME, and everything else
// asks. The TUI, the CLI, the shell sandbox and — later, through a tunnel over
// the gateway socket nikd already holds — a browser all speak this, so there
// is one place where a request is authorized, audited and answered rather than
// four processes reaching into the same SQLite file and hoping.
//
// HTTP and JSON, not a bespoke frame format, and not because it is ever served
// on a port. `curl --unix-socket` is a debugger that already exists, every
// language has a client, and a request line maps onto a tunnel envelope
// without translation — which is what makes the remote half a pipe rather
// than a protocol.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/version"
)

// APIVersion is the contract nikctl checks against. It changes when a
// response shape changes in a way an older client would misread — not when
// an endpoint is added, since a client that does not know an endpoint does
// not call it.
const APIVersion = 1

type Server struct {
	state  *State
	broker *Broker
	mux    *http.ServeMux

	// Everything below is plugged in as nikd converges, so the guard is a
	// nil check rather than a boot ordering rule. See SetChat.
	mu         sync.RWMutex
	chat       Chat
	config     Config
	gateway    Gateway
	secrets    Secrets
	onboarding Onboarding
	workload   Workload
	inspector  Inspector
	shell      Shell
	logs       Logs
	restarter  Restarter
}

func New(state *State) *Server {
	s := &Server{state: state, broker: NewBroker(), mux: http.NewServeMux()}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /v1/version", s.handleVersion)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)

	s.mux.HandleFunc("GET /v1/conversations/{id}", s.handleConversationGet)
	s.mux.HandleFunc("GET /v1/conversations/{id}/messages", s.handleMessagesList)
	s.mux.HandleFunc("POST /v1/conversations/{id}/messages", s.handleMessageSend)

	s.mux.HandleFunc("GET /v1/events", s.handleEvents)

	s.mux.HandleFunc("GET /v1/config", s.handleConfigGet)
	s.mux.HandleFunc("PATCH /v1/config", s.handleConfigPatch)
	s.mux.HandleFunc("POST /v1/gateway/connect", s.handleGatewayConnect)

	s.mux.HandleFunc("GET /v1/onboarding", s.handleOnboarding)
	s.mux.HandleFunc("GET /v1/workload", s.handleWorkload)

	s.mux.HandleFunc("POST /v1/db/query", s.handleQuery)
	s.mux.HandleFunc("POST /v1/shell", s.handleShell)
	s.mux.HandleFunc("GET /v1/logs", s.handleLogs)
	s.mux.HandleFunc("POST /v1/daemon/restart", s.handleRestart)

	s.mux.HandleFunc("GET /v1/secrets", s.handleSecretsList)
	s.mux.HandleFunc("GET /v1/secrets/{name}", s.handleSecretGet)
	s.mux.HandleFunc("PUT /v1/secrets/{name}", s.handleSecretSet)
	s.mux.HandleFunc("DELETE /v1/secrets/{name}", s.handleSecretDelete)
}

// Broker is where nikd publishes what it wants clients to know about.
func (s *Server) Broker() *Broker { return s.broker }

// Handler is the whole API for the owner, which is what lets the same routes
// serve a unix socket now and a tunnelled request later without either
// knowing about the other.
func (s *Server) Handler() http.Handler {
	return withScope(ScopeOwner, logRequests(s.mux))
}

// SandboxHandler is the same routes, narrowed. Scope comes from which
// listener accepted the connection, so there is nothing a caller can say
// about itself that widens it.
func (s *Server) SandboxHandler() http.Handler {
	return withScope(ScopeSandbox, logRequests(s.mux))
}

// Serve runs the owner API until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	return s.serve(ctx, ln, s.Handler())
}

// ServeSandbox runs the narrowed API for the shell container.
func (s *Server) ServeSandbox(ctx context.Context, ln net.Listener) error {
	return s.serve(ctx, ln, s.SandboxHandler())
}

// serve stops accepting when ctx is cancelled and gives in-flight requests a
// moment to finish. It never returns http.ErrServerClosed as an error: a
// clean shutdown is what cancelling ctx asked for.
func (s *Server) serve(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler: handler,
		// A request that arrives over a unix socket is local, so these are
		// not defence against a slow network — they are what stops one wedged
		// client from holding a connection forever.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		close(done)
	}()

	err := srv.Serve(ln)
	<-done

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

type versionResponse struct {
	Version    string `json:"version"`
	Number     string `json:"number"`
	Commit     string `json:"commit"`
	APIVersion int    `json:"api_version"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, versionResponse{
		Version:    version.String(),
		Number:     version.Number,
		Commit:     version.SHA,
		APIVersion: APIVersion,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.state.Snapshot(version.Number, version.SHA))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		// The status line is already sent, so this cannot become a 500. It is
		// still worth a line: a body that failed to encode is a bug here, not
		// a client's problem.
		slog.Warn("encode response", "pkg", "api", "error", err)
	}
}

// Error is the shape every failure takes, so a client never has to guess
// whether a body is the thing it asked for or the reason it did not get it.
type Error struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, Error{Error: msg})
}

// maxRequestBody is generous for a chat message and small enough that a
// malformed or hostile client cannot make nikd allocate its way out of a
// cell's memory budget. Bodies large enough to matter — media — are uploaded
// as their own thing, not as JSON.
const maxRequestBody = 1 << 20

func readJSON[T any](r *http.Request) (T, error) {
	var out T

	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	dec.DisallowUnknownFields()

	err := dec.Decode(&out)
	if err != nil {
		return out, fmt.Errorf("invalid request body: %w", err)
	}

	return out, nil
}

// logRequests records what was asked and how it went. On the owner socket
// this is mostly noise at Debug; it earns its place when the same handler
// starts answering tunnelled requests, where "we reached into their computer"
// needs a record the owner can read back.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Debug("api request",
			"pkg", "api",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap is what lets http.ResponseController reach the real writer through
// this middleware. Without it, wrapping the writer silently strips
// http.Flusher — and a stripped Flusher is an event stream that never
// streams, which is exactly the bug this method exists to prevent.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
