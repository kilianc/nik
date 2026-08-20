package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

// The privileged surface: look at nik's data, run a command in its sandbox,
// restart it. These are the tools that only ever worked from a terminal, which
// is the whole reason a managed family cannot be handed a nik and left to it.
//
// Every one of them is something nik can already do to itself — db_query,
// shell and restart are brain tools — so what is new is the door, not the
// capability. It is a door worth being careful about all the same: on the
// owner socket this is somebody's own machine, and over the tunnel it is
// somebody's house.

// Inspector runs read-only queries against nik's database.
type Inspector interface {
	Query(ctx context.Context, query string) (any, error)
}

// Shell runs a command in nik's sandbox.
type Shell interface {
	Run(ctx context.Context, command string, timeoutSeconds int) (ShellResult, error)
}

// ShellResult is what a command did.
type ShellResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	// Running is true when the command outlived its wait rather than
	// finishing. The output is what it had produced so far.
	Running bool `json:"running"`
}

// Logs reads nikd's own log files — the first thing anybody wants from a nik
// that is up and not answering.
type Logs interface {
	Tail(ctx context.Context, errorsOnly bool, lines int) ([]string, error)
}

// Restarter restarts the daemon.
type Restarter interface {
	Restart(ctx context.Context) error
}

// ErrNotReadOnly is a query that would change something. Returned as 400: the
// caller asked for the wrong thing, and nikd is fine.
var ErrNotReadOnly = errors.New("query is not read-only")

func (s *Server) SetInspector(inspector Inspector) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inspector = inspector
}

func (s *Server) SetShell(shell Shell) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shell = shell
}

func (s *Server) SetLogs(logs Logs) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = logs
}

func (s *Server) SetRestarter(restarter Restarter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.restarter = restarter
}

// QueryRequest is the body of POST /v1/db/query.
type QueryRequest struct {
	Query string `json:"query"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	inspector := s.inspector
	s.mu.RUnlock()

	if inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	req, err := readJSON[QueryRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := inspector.Query(r.Context(), req.Query)
	if errors.Is(err, ErrNotReadOnly) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		// A malformed SQL statement is the caller's problem, not nikd's, and
		// telling them what SQLite said is the entire value of the endpoint.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ShellRequest is the body of POST /v1/shell.
type ShellRequest struct {
	Command string `json:"command"`
	// TimeoutSeconds bounds how long to wait for output. A command that
	// outlives it keeps running in its session; the response says so.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

func (s *Server) handleShell(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	shell := s.shell
	s.mu.RUnlock()

	if shell == nil {
		writeError(w, http.StatusServiceUnavailable, "nik's sandbox is not ready")
		return
	}

	req, err := readJSON[ShellRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	result, err := shell.Run(r.Context(), req.Command, req.TimeoutSeconds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

type logsResponse struct {
	Lines []string `json:"lines"`
}

const (
	defaultLogLines = 200
	maxLogLines     = 2000
)

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	logs := s.logs
	s.mu.RUnlock()

	if logs == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	lines := defaultLogLines
	if raw := r.URL.Query().Get("lines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "lines must be a positive number")
			return
		}
		lines = min(n, maxLogLines)
	}

	out, err := logs.Tail(r.Context(), r.URL.Query().Get("errors") == "true", lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []string{}
	}

	writeJSON(w, http.StatusOK, logsResponse{Lines: out})
}

// handleRestart is what an admin button and the `restart` brain tool both do.
//
// The reply is 202 and nothing else. An ack says the daemon read the request;
// only a fresh connection says it came back, so anything watching a restart
// waits for that rather than trusting this.
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	restarter := s.restarter
	s.mu.RUnlock()

	if restarter == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	err := restarter.Restart(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
