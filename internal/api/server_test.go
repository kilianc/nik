package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionEndpoint(t *testing.T) {
	srv := New(NewState())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}

	var got versionResponse
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.APIVersion != APIVersion {
		t.Fatalf("api_version = %d, want %d", got.APIVersion, APIVersion)
	}
	if got.Version == "" {
		t.Fatal("version is empty")
	}
}

// A daemon that has registered nothing is starting, not healthy — the whole
// point of the endpoint is that "the process is up" is not an answer.
func TestHealthNotReadyBeforeAnythingRegisters(t *testing.T) {
	srv := New(NewState())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	var got Health
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Ready {
		t.Fatal("ready with no subsystems registered")
	}
}

func TestHealthReportsDegradedSubsystems(t *testing.T) {
	state := NewState()
	state.Set("config", true, "/home/fam/.nik/config.yaml")
	state.Set("db", true, "/home/fam/.nik/nik.db")
	state.Set("gateway", false, "token rejected")

	srv := New(state)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	var got Health
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Ready {
		t.Fatal("ready with a failing gateway")
	}
	if len(got.Degraded) != 1 || got.Degraded[0] != "gateway" {
		t.Fatalf("degraded = %v, want [gateway]", got.Degraded)
	}
	if got.Subsystem["gateway"].Detail != "token rejected" {
		t.Fatalf("detail = %q, want the reason", got.Subsystem["gateway"].Detail)
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	srv := New(NewState())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServeStopsWithContext(t *testing.T) {
	ln, err := Listen(filepath.Join(shortHome(t), "run", "nikd.sock"))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New(NewState()).Serve(ctx, ln) }()

	cancel()

	select {
	case err := <-done:
		// A cancelled context is a clean stop, not an error to log.
		if err != nil {
			t.Fatalf("Serve returned %v, want nil on cancel", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

// The socket is the only thing standing between a local user and NIK_HOME,
// so its modes are load-bearing rather than tidy.
func TestListenLocksDownSocketAndDir(t *testing.T) {
	dir := filepath.Join(shortHome(t), "run")
	path := filepath.Join(dir, "nikd.sock")

	ln, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir mode = %o, want 700", perm)
	}

	sockInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := sockInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 600", perm)
	}
}

// A daemon killed with SIGKILL leaves the socket file behind. The next one
// has to bind anyway — the pid file, not the socket, is what answers "is
// another daemon already running".
func TestListenReplacesAStaleSocket(t *testing.T) {
	path := filepath.Join(shortHome(t), "run", "nikd.sock")

	first, err := Listen(path)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	// Close the listener but leave the file, which is what a hard kill does.
	first.Close()
	err = os.WriteFile(path, nil, 0o600)
	if err != nil {
		t.Fatalf("fake stale socket: %v", err)
	}

	second, err := Listen(path)
	if err != nil {
		t.Fatalf("second Listen over a stale socket: %v", err)
	}
	second.Close()
}

// shortHome is a temp directory with a short path. t.TempDir() embeds the test
// name, which on macOS pushes a socket path past the 104-byte sun_path limit —
// a real constraint, but not the one these tests are about.
func shortHome(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "nik")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	return dir
}
