package shell

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not available")
	}
	sessionPrefix = "__nik_test__"
}

func cleanup(t *testing.T, id string) {
	t.Helper()
	killSession(id)
}

func TestNewSessionAndKill(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-new-kill"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	alive, err := isAlive(id)
	if err != nil {
		t.Fatalf("isAlive: %v", err)
	}
	if !alive {
		t.Fatal("expected session to be alive")
	}

	err = killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}
}

func TestEnvVars(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-env"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60")
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

	env, err := getAllEnv(id)
	if err != nil {
		t.Fatalf("getAllEnv: %v", err)
	}
	if env["NIK_TEST_KEY"] != "hello world" {
		t.Fatalf("getAllEnv: expected %q, got %q", "hello world", env["NIK_TEST_KEY"])
	}
}

func TestFastCommand(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-fast"
	defer cleanup(t, id)

	err := newSession(id, "echo hello-nik")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	output, alive, code := stare(id, 5)

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

func TestStareAutoBackground(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-bg"
	defer cleanup(t, id)

	err := newSession(id, "sleep 30")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	start := time.Now()
	_, alive, _ := stare(id, 2)
	elapsed := time.Since(start)

	if !alive {
		t.Fatal("expected command to still be running")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("stare took too long: %v", elapsed)
	}
}

func TestSendInput(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-send"
	defer cleanup(t, id)

	err := newSession(id, "cat")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	err = sendKeys(id, "test-input-line", "Enter")
	if err != nil {
		t.Fatalf("sendKeys input: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	output, err := captureOutput(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}
	if !strings.Contains(output, "test-input-line") {
		t.Fatalf("expected output to contain 'test-input-line', got: %s", output)
	}
}

func TestListSessions(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-list"
	defer cleanup(t, id)

	err := newSession(id, "sleep 60")
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
			if !s.Alive {
				t.Fatal("expected session to be alive")
			}
		}
	}

	if !found {
		t.Fatal("session not found in list")
	}
}

func TestOutputTruncation(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-trunc"
	defer cleanup(t, id)

	err := newSession(id, "seq 1 50000")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	stare(id, 5)

	output, err := captureOutput(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}

	if len(output) > maxOutputBytes+100 {
		t.Fatalf("output not truncated: %d bytes", len(output))
	}
}

func TestExitCode(t *testing.T) {
	skipIfNoTmux(t)

	id := "test-exit"
	defer cleanup(t, id)

	err := newSession(id, "exit 42")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	_, alive, code := stare(id, 5)

	if alive {
		t.Fatal("expected command to have exited")
	}
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
}

func TestParseNextCheckAt(t *testing.T) {
	now := time.Now()

	t.Run("relative seconds", func(t *testing.T) {
		result, err := parseNextCheckAt("+30s")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		diff := result.Sub(now)
		if diff < 29*time.Second || diff > 31*time.Second {
			t.Fatalf("expected ~30s from now, got %v", diff)
		}
	})

	t.Run("relative minutes", func(t *testing.T) {
		result, err := parseNextCheckAt("+5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		diff := result.Sub(now)
		if diff < 4*time.Minute || diff > 6*time.Minute {
			t.Fatalf("expected ~5m from now, got %v", diff)
		}
	})

	t.Run("relative days", func(t *testing.T) {
		result, err := parseNextCheckAt("+1d")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		diff := result.Sub(now)
		if diff < 23*time.Hour || diff > 25*time.Hour {
			t.Fatalf("expected ~24h from now, got %v", diff)
		}
	})

	t.Run("RFC3339", func(t *testing.T) {
		ts := "2026-03-01T15:00:00Z"
		result, err := parseNextCheckAt(ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected, _ := time.Parse(time.RFC3339, ts)
		if !result.Equal(expected) {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	})
}
