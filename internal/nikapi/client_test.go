package nikapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kciuffolo/nik/internal/api"
)

// serveOn starts a real nikd API on a real socket. The point of this suite is
// the pair — a client test against a mock server would prove the mock.
func serveOn(t *testing.T, state *api.State) *Client {
	t.Helper()

	path := filepath.Join(shortHome(t), "run", "nikd.sock")
	ln, err := api.Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = api.New(state).Serve(ctx, ln)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	return NewAtSocket(path)
}

func TestVersionRoundTrip(t *testing.T) {
	client := serveOn(t, api.NewState())

	got, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got.APIVersion != api.APIVersion {
		t.Fatalf("api_version = %d, want %d", got.APIVersion, api.APIVersion)
	}
}

func TestHealthRoundTrip(t *testing.T) {
	state := api.NewState()
	state.Set("db", true, "/tmp/nik.db")
	state.Set("gateway", false, "no token")

	client := serveOn(t, state)

	got, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.Ready {
		t.Fatal("ready with a failing subsystem")
	}
	if len(got.Degraded) != 1 || got.Degraded[0] != "gateway" {
		t.Fatalf("degraded = %v, want [gateway]", got.Degraded)
	}
	if !got.Subsystem["db"].OK {
		t.Fatal("db should be ok")
	}
}

// "nikd is not running" is the most common thing this client will ever
// report, and nikctl branches on it to print something a person can act on.
// If it degrades to a generic transport error, that message is lost.
func TestNoDaemonIsTyped(t *testing.T) {
	client := NewAtSocket(filepath.Join(shortHome(t), "nothing-here.sock"))

	_, err := client.Health(context.Background())
	if !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("err = %v, want ErrNoDaemon", err)
	}
}

// A socket that exists but has nobody behind it is the other half of the same
// story: a daemon killed hard leaves the file, and connect fails with
// ECONNREFUSED rather than ENOENT.
func TestStaleSocketIsAlsoNoDaemon(t *testing.T) {
	path := filepath.Join(shortHome(t), "run", "nikd.sock")

	ln, err := api.Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ln.Close()

	client := NewAtSocket(path)

	_, err = client.Health(context.Background())
	if !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("err = %v, want ErrNoDaemon", err)
	}
}

func TestNewUsesTheOwnerSocketPath(t *testing.T) {
	home := t.TempDir()

	client := New(home)

	want := api.OwnerSocketPath(home)
	if client.Socket() != want {
		t.Fatalf("socket = %q, want %q", client.Socket(), want)
	}
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
