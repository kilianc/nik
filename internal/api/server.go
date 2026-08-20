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
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/kciuffolo/nik/internal/version"
)

// APIVersion is the contract nikctl checks against. It changes when a
// response shape changes in a way an older client would misread — not when
// an endpoint is added, since a client that does not know an endpoint does
// not call it.
const APIVersion = 1

type Server struct {
	state *State
	mux   *http.ServeMux
}

func New(state *State) *Server {
	s := &Server{state: state, mux: http.NewServeMux()}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /v1/version", s.handleVersion)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
}

// Handler is the whole API as an http.Handler, which is what lets the same
// routes serve a unix socket now and a tunnelled request later without
// either knowing about the other.
func (s *Server) Handler() http.Handler {
	return logRequests(s.mux)
}

// Serve runs until ctx is cancelled, then stops accepting and gives in-flight
// requests a moment to finish. It never returns http.ErrServerClosed as an
// error: a clean shutdown is what cancelling ctx asked for.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler: s.Handler(),
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
