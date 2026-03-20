package shell

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
)

func requireTmux(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal("tmux not available")
	}
	sessionPrefix = "__nik_test__"
}

func testService(t *testing.T) *Service {
	t.Helper()
	return NewService(&config.Config{Home: t.TempDir()}, nil)
}

func (s *Service) cleanup(t *testing.T, id string) {
	t.Helper()
	s.killSession(id)
}

func TestNewSessionAndKill(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-new-kill"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	if !svc.isAlive(id) {
		t.Fatal("expected session to be alive")
	}

	err = svc.killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}
}

func TestEnvVars(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-env"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = svc.setEnv(id, "NIK_TEST_KEY", "hello world")
	if err != nil {
		t.Fatalf("setEnv: %v", err)
	}

	val, err := svc.getEnv(id, "NIK_TEST_KEY")
	if err != nil {
		t.Fatalf("getEnv: %v", err)
	}
	if val != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", val)
	}
}

func TestFastCommand(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-fast"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "echo hello-nik", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	output, alive, code := svc.stare(context.Background(), id, 5)

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
	svc := testService(t)

	id := "test-bg"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "sleep 30", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	start := time.Now()
	_, alive, _ := svc.stare(context.Background(), id, 2)
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
	svc := testService(t)

	id := "test-send"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "cat", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	err = svc.sendKeys(id, "test-input-line", "Enter")
	if err != nil {
		t.Fatalf("sendKeys input: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	output, err := svc.capturePane(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}
	if !strings.Contains(output, "test-input-line") {
		t.Fatalf("expected output to contain 'test-input-line', got: %s", output)
	}
}

func TestListSessions(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-list"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	sessions, err := svc.listSessions()
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
	svc := testService(t)

	id := "test-trunc"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "seq 1 500000", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	svc.stare(context.Background(), id, 10)

	output, err := svc.capturePane(id)
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}

	if len(output) > maxCaptureBytes+100 {
		t.Fatalf("output not truncated: %d bytes", len(output))
	}
}

func TestExitCode(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-exit"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "exit 42", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	_, alive, code := svc.stare(context.Background(), id, 5)

	if alive {
		t.Fatal("expected command to have exited")
	}
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
}

func TestStareMissingSession(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-stare-missing"

	err := svc.newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = svc.killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}

	_, alive, _ := svc.stare(context.Background(), id, 2)
	if alive {
		t.Fatal("stare reported alive for a killed session")
	}
}

func TestWaitForInstantReturn(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-waitfor-instant"
	defer svc.cleanup(t, id)

	err := svc.newSession(id, "echo done", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	start := time.Now()
	output, alive, code := svc.stare(context.Background(), id, 30)
	elapsed := time.Since(start)

	if alive {
		t.Fatal("expected command to have exited")
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(output, "done") {
		t.Fatalf("expected output to contain 'done', got: %s", output)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("wait-for should have returned near-instantly, took %v", elapsed)
	}
}

func TestGetEnvNonexistentSession(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	_, err := svc.getEnv("does-not-exist", "SOME_KEY")
	if err == nil {
		t.Fatal("expected error from getEnv on nonexistent session, got nil")
	}
}
