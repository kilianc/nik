package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type fakeInspector struct {
	query  string
	result any
	err    error
}

func (f *fakeInspector) Query(_ context.Context, query string) (any, error) {
	f.query = query

	return f.result, f.err
}

type fakeShell struct {
	command string
	timeout int
	result  ShellResult
	err     error
}

func (f *fakeShell) Run(_ context.Context, command string, timeout int) (ShellResult, error) {
	f.command, f.timeout = command, timeout

	return f.result, f.err
}

type fakeLogs struct {
	errorsOnly bool
	lines      int
}

func (f *fakeLogs) Tail(_ context.Context, errorsOnly bool, lines int) ([]string, error) {
	f.errorsOnly, f.lines = errorsOnly, lines

	return []string{"a log line"}, nil
}

type fakeRestarter struct{ called bool }

func (f *fakeRestarter) Restart(context.Context) error {
	f.called = true

	return nil
}

func TestQueryReturnsResults(t *testing.T) {
	inspector := &fakeInspector{result: map[string]any{"count": 2}}
	srv := New(NewState())
	srv.SetInspector(inspector)

	rec := do(t, srv, http.MethodPost, "/v1/db/query", `{"query":"SELECT 1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if inspector.query != "SELECT 1" {
		t.Fatalf("query = %q", inspector.query)
	}
}

// A write is the caller asking for the wrong thing, not nikd failing. 500
// would send somebody reading the daemon's logs for their own mistake.
func TestQueryRejectsWritesWith400(t *testing.T) {
	srv := New(NewState())
	srv.SetInspector(&fakeInspector{err: ErrNotReadOnly})

	rec := do(t, srv, http.MethodPost, "/v1/db/query", `{"query":"DELETE FROM message"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestShellRunsAndReportsExitCode(t *testing.T) {
	shell := &fakeShell{result: ShellResult{Output: "hello", ExitCode: 0}}
	srv := New(NewState())
	srv.SetShell(shell)

	rec := do(t, srv, http.MethodPost, "/v1/shell", `{"command":"echo hello","timeout_seconds":5}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if shell.command != "echo hello" || shell.timeout != 5 {
		t.Fatalf("shell got %q / %d", shell.command, shell.timeout)
	}

	var got ShellResult
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Output != "hello" {
		t.Fatalf("output = %q", got.Output)
	}
}

// A command that outlives its wait is still running, not failed. Saying so is
// what stops a caller treating partial output as the whole answer.
func TestShellReportsAStillRunningCommand(t *testing.T) {
	srv := New(NewState())
	srv.SetShell(&fakeShell{result: ShellResult{Output: "working...", Running: true}})

	rec := do(t, srv, http.MethodPost, "/v1/shell", `{"command":"sleep 600"}`)

	var got ShellResult
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Running {
		t.Fatal("running = false, want true")
	}
}

func TestShellRequiresACommand(t *testing.T) {
	srv := New(NewState())
	srv.SetShell(&fakeShell{})

	rec := do(t, srv, http.MethodPost, "/v1/shell", `{"command":""}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLogsPassThroughOptions(t *testing.T) {
	logs := &fakeLogs{}
	srv := New(NewState())
	srv.SetLogs(logs)

	rec := do(t, srv, http.MethodGet, "/v1/logs?errors=true&lines=50", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !logs.errorsOnly || logs.lines != 50 {
		t.Fatalf("logs got errors=%v lines=%d", logs.errorsOnly, logs.lines)
	}
}

func TestLogsClampsLines(t *testing.T) {
	logs := &fakeLogs{}
	srv := New(NewState())
	srv.SetLogs(logs)

	do(t, srv, http.MethodGet, "/v1/logs?lines=999999", "")

	if logs.lines != maxLogLines {
		t.Fatalf("lines = %d, want it clamped to %d", logs.lines, maxLogLines)
	}
}

// 202: the daemon read the request. Only a fresh connection says it came
// back, which is the caller's job to wait for.
func TestRestartIsAccepted(t *testing.T) {
	restarter := &fakeRestarter{}
	srv := New(NewState())
	srv.SetRestarter(restarter)

	rec := do(t, srv, http.MethodPost, "/v1/daemon/restart", `{}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if !restarter.called {
		t.Fatal("restarter was not called")
	}
}

// None of this is reachable from the shell container. A skill that could
// restart nik or read its whole database through the sandbox socket would
// make the deny list on secrets pointless.
func TestPrivilegedEndpointsAreClosedToTheSandbox(t *testing.T) {
	srv := New(NewState())
	srv.SetInspector(&fakeInspector{})
	srv.SetShell(&fakeShell{})
	srv.SetLogs(&fakeLogs{})
	srv.SetRestarter(&fakeRestarter{})

	for _, tc := range []struct{ method, target string }{
		{http.MethodPost, "/v1/db/query"},
		{http.MethodPost, "/v1/shell"},
		{http.MethodGet, "/v1/logs"},
		{http.MethodPost, "/v1/daemon/restart"},
	} {
		rec := sandboxDo(t, srv, tc.method, tc.target, `{"query":"SELECT 1","command":"whoami"}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403 from the sandbox", tc.method, tc.target, rec.Code)
		}
	}
}
