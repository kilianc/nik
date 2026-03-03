package shell

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func requireTmux(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux not available")
	}
	sessionPrefix = "__nik_test__"
}

func cleanup(t *testing.T, id string) {
	t.Helper()
	killSession(id)
}

func TestNewSessionAndKill(t *testing.T) {
	requireTmux(t)

	id := "test-new-kill"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	if !isAlive(id) {
		t.Fatal("expected session to be alive")
	}

	err = killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}
}

func TestEnvVars(t *testing.T) {
	requireTmux(t)

	id := "test-env"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = setEnv(id, "NIK_TEST_KEY", "hello world")
	if err != nil {
		t.Fatalf("setEnv: %v", err)
	}

	val, err := getEnv(id, "NIK_TEST_KEY")
	if err != nil {
		t.Fatalf("getEnv: %v", err)
	}
	if val != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", val)
	}
}

func TestFastCommand(t *testing.T) {
	requireTmux(t)

	id := "test-fast"
	defer cleanup(t, id)

	err := newSession(id, "echo hello-nik", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	output, alive, code := stare(id, 5, "")

	if alive {
		t.Fatal("expected command to have exited")
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(output, "hello-nik") {
		t.Fatalf("expected output to contain 'hello-nik', got: %s", output)
	}
}

func TestStare(t *testing.T) {
	requireTmux(t)

	id := "test-bg"
	defer cleanup(t, id)

	err := newSession(id, "sleep 30", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	start := time.Now()
	_, alive, _ := stare(id, 2, "")
	elapsed := time.Since(start)

	if !alive {
		t.Fatal("expected command to still be running")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("stare took too long: %v", elapsed)
	}
}

func TestSendInput(t *testing.T) {
	requireTmux(t)

	id := "test-send"
	defer cleanup(t, id)

	err := newSession(id, "cat", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	err = sendKeys(id, "test-input-line", "Enter")
	if err != nil {
		t.Fatalf("sendKeys input: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	output, err := capturePane(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}
	if !strings.Contains(output, "test-input-line") {
		t.Fatalf("expected output to contain 'test-input-line', got: %s", output)
	}
}

func TestListSessions(t *testing.T) {
	requireTmux(t)

	id := "test-list"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	sessions, err := listSessions()
	if err != nil {
		t.Fatalf("listSessions: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s.ID == id {
			found = true
			if !s.isAlive {
				t.Fatal("expected session to be alive")
			}
		}
	}

	if !found {
		t.Fatal("session not found in list")
	}
}

func TestOutputTruncation(t *testing.T) {
	requireTmux(t)

	id := "test-trunc"
	defer cleanup(t, id)

	err := newSession(id, "seq 1 50000", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	stare(id, 5, "")

	output, err := capturePane(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}

	if len(output) > maxOutputBytes+100 {
		t.Fatalf("output not truncated: %d bytes", len(output))
	}
}

func TestExitCode(t *testing.T) {
	requireTmux(t)

	id := "test-exit"
	defer cleanup(t, id)

	err := newSession(id, "exit 42", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	_, alive, code := stare(id, 5, "")

	if alive {
		t.Fatal("expected command to have exited")
	}
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
}

func TestStareMissingSession(t *testing.T) {
	requireTmux(t)

	id := "test-stare-missing"

	err := newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}

	_, alive, _ := stare(id, 2, "")
	if alive {
		t.Fatal("stare reported alive for a killed session")
	}
}

func TestStareWatchFor(t *testing.T) {
	requireTmux(t)

	id := "test-watchfor"
	defer cleanup(t, id)

	err := newSession(id, `sh -c 'sleep 1; echo MARKER_READY; cat'`, "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	start := time.Now()
	output, alive, _ := stare(id, 10, "MARKER_READY")
	elapsed := time.Since(start)

	if !alive {
		t.Fatal("expected session to still be alive")
	}
	if !strings.Contains(output, "MARKER_READY") {
		t.Fatalf("expected output to contain MARKER_READY, got: %s", output)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("watch_for should have returned early, took %v", elapsed)
	}
}

func TestStareWatchForBaseline(t *testing.T) {
	requireTmux(t)

	id := "test-watchfor-baseline"
	defer cleanup(t, id)

	err := newSession(id, `sh -c 'echo OLD_MARKER; cat'`, "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// capture baseline that already contains OLD_MARKER
	baseline, _ := capturePane(id)
	baselineLen := len(baseline)

	// stare with watchFor looking for OLD_MARKER should NOT match early
	// because it only appears before the baseline
	start := time.Now()
	_, alive, _ := stareWith(id, 2, "OLD_MARKER", baselineLen)
	elapsed := time.Since(start)

	if !alive {
		t.Fatal("expected session to still be alive")
	}
	if elapsed < time.Second {
		t.Fatalf("stare with baseline should not have matched old content, returned in %v", elapsed)
	}
}

func TestGetEnvNonexistentSession(t *testing.T) {
	requireTmux(t)

	_, err := getEnv("does-not-exist", "SOME_KEY")
	if err == nil {
		t.Fatal("expected error from getEnv on nonexistent session, got nil")
	}
}
