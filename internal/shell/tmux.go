package shell

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var sessionPrefix = "nik-"

const (
	maxCaptureBytes = 512 * 1024
	maxContextBytes = 32 * 1024
	historyLimit    = 50000
	windowWidth     = 200
	windowHeight    = 50
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type SessionInfo struct {
	ID      string
	isAlive bool
}

func sessionName(id string) string {
	return sessionPrefix + id
}

func (s *Service) ensureTmux() error {
	if s.container != "" {
		out, err := s.tmux("-V")
		if err != nil {
			return fmt.Errorf("tmux in container %s: %w", s.container, err)
		}
		_ = out
		return nil
	}

	_, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("find tmux: %w", err)
	}

	return nil
}

func (s *Service) newSession(id, command, cwd string) error {
	name := sessionName(id)

	args := []string{
		"new-session", "-d",
		"-s", name,
		"-x", fmt.Sprintf("%d", windowWidth),
		"-y", fmt.Sprintf("%d", windowHeight),
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}

	_, err := s.tmux(args...)
	if err != nil {
		return fmt.Errorf("create session %s: %w", id, err)
	}

	_, err = s.tmux("set-option", "-t", name, "remain-on-exit", "on")
	if err != nil {
		return fmt.Errorf("set remain-on-exit %s: %w", id, err)
	}

	_, err = s.tmux("set-option", "-t", name, "history-limit", fmt.Sprintf("%d", historyLimit))
	if err != nil {
		return fmt.Errorf("set history-limit %s: %w", id, err)
	}

	basePath := os.Getenv("PATH")
	if s.container != "" {
		basePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	s.setEnv(id, "PATH", s.nikBinDir()+":"+basePath)
	s.setEnv(id, "NIK_HOME", s.workdir())

	if command != "" {
		ch := name + "-done"

		// The command records its own exit code before announcing it is
		// done, so the code is already there when stare wakes up. See
		// getExitCode for why tmux's own answer cannot be relied on.
		//
		// Both interpolated names are quoted. Session ids come from callers
		// and contain spaces often enough that this package's own tests have
		// two: unquoted, `tmux wait-for -S nik-exit code-done` reaches tmux as
		// two arguments, tmux rejects it as a usage error, and the done signal
		// is never sent at all — leaving stare to discover the exit the slow
		// way, on its two-second tick.
		wrapped := fmt.Sprintf(
			"(%s); __ec=$?; tmux set-option -t %s %s \"$__ec\"; tmux wait-for -S %s; exit $__ec",
			command, shellQuote(name), exitOption, shellQuote(ch),
		)

		_, err = s.tmux("respawn-pane", "-k", "-t", name, "sh", "-c", wrapped)
		if err != nil {
			return fmt.Errorf("respawn pane %s: %w", id, err)
		}
	}

	return nil
}

// shellQuote wraps s so that sh sees exactly one word, whatever is inside it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *Service) setEnv(id, key, value string) error {
	_, err := s.tmux("set-environment", "-t", sessionName(id), key, value)
	if err != nil {
		return fmt.Errorf("set env %s %s: %w", id, key, err)
	}

	return nil
}

func (s *Service) getEnv(id, key string) (string, error) {
	out, err := s.tmux("show-environment", "-t", sessionName(id), key)
	if err != nil {
		return "", fmt.Errorf("get env %s %s: %w", id, key, err)
	}

	parts := strings.SplitN(strings.TrimSpace(out), "=", 2)
	if len(parts) != 2 {
		return "", nil
	}

	return parts[1], nil
}

func (s *Service) sendKeys(id string, keys ...string) error {
	args := []string{"send-keys", "-t", sessionName(id)}
	args = append(args, keys...)

	_, err := s.tmux(args...)
	if err != nil {
		return fmt.Errorf("send keys %s: %w", id, err)
	}

	return nil
}

func (s *Service) capturePane(id string) (string, error) {
	out, err := s.tmux("capture-pane", "-t", sessionName(id), "-p", "-S", "-")
	if err != nil {
		return "", fmt.Errorf("capture output %s: %w", id, err)
	}

	out = ansiRe.ReplaceAllString(out, "")
	out = strings.TrimRight(out, "\n ")

	if len(out) > maxCaptureBytes {
		out = out[len(out)-maxCaptureBytes:]
	}

	return out, nil
}

func (s *Service) isAlive(id string) bool {
	out, err := s.tmux(
		"display-message", "-t", sessionName(id),
		"-p", "#{pane_dead}",
	)
	if err != nil {
		return false
	}

	return strings.TrimSpace(out) == "0"
}

// deadPaneWait bounds how long stare waits for an exit code to appear.
//
// A command that ran nik's wrapper has already recorded its code by the time
// it signals done, so that path does not wait at all. This budget is for the
// panes that never got to run it — killed from outside, or gone before the
// wrapper's first line — where tmux's own pane_dead_status is the only answer
// available and is worth a moment.
const deadPaneWait = time.Second

// awaitDeadPane polls until an exit code is readable or the wait runs out.
// Returning on timeout rather than erroring is deliberate: the caller has a
// better-than-nothing answer either way, and a command whose exit code cannot
// be read is not a reason to lose its output.
func (s *Service) awaitDeadPane(id string) {
	deadline := time.Now().Add(deadPaneWait)

	for {
		// Poll the value stare is about to read, rather than the pane_dead
		// flag sitting next to it: pane_dead says the pty closed, which is
		// not the same event as an exit code becoming available, and on some
		// tmux versions never implies it. See getExitCode.
		if _, err := s.getExitCode(id); err == nil {
			return
		}
		if time.Now().After(deadline) {
			slog.Warn("pane published no exit status after its command signalled done",
				"pkg", "shell", "session", id)

			return
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// exitOption is the session option the wrapped command writes its exit code
// into, and the answer getExitCode prefers over anything tmux reports.
const exitOption = "@nik_exit"

// getExitCode returns the code the pane's command exited with, asking for our
// own record and tmux's in a single round trip and trusting ours first.
//
// tmux's own answer cannot carry this on its own. On tmux 3.4 — what Ubuntu
// 24.04 ships, and so every ubuntu-latest runner — a pane whose command has
// itself run a tmux client is frequently never reaped by the server, and
// neither pane_dead_status nor pane_dead_signal is ever published for it.
// nik's wrapper always runs one, so this lands on roughly half of all
// commands. The pane still reports pane_dead=1, because that comes from the
// pty reaching EOF rather than from a reaped child, which is what made this
// look like two properties arriving a moment apart. It is not a race: the
// status does not arrive late, it never arrives, and waiting longer for it —
// twice attempted — cannot work. tmux 3.5a fixes it, which is why the same
// code has always passed on macOS's 3.6a.
func (s *Service) getExitCode(id string) (int, error) {
	out, err := s.tmux(
		"display-message", "-t", sessionName(id),
		"-p", "#{"+exitOption+"}|#{pane_dead_status}",
	)
	if err != nil {
		return -1, fmt.Errorf("exit code %s: %w", id, err)
	}

	for _, field := range strings.SplitN(strings.TrimSpace(out), "|", 2) {
		var code int
		if _, err := fmt.Sscanf(strings.TrimSpace(field), "%d", &code); err == nil {
			return code, nil
		}
	}

	// No code from either source, which is not the same thing as a command
	// that exited -1. Returning a nil error here told every caller the read
	// had worked and handed them a sentinel as though it were an answer — so
	// nothing could wait for a value that had not arrived, because nothing
	// could tell that it had not.
	return -1, fmt.Errorf("exit code %s: no exit code recorded", id)
}

func (s *Service) killSession(id string) error {
	_, err := s.tmux("kill-session", "-t", sessionName(id))
	if err != nil {
		return fmt.Errorf("kill session %s: %w", id, err)
	}

	return nil
}

func (s *Service) listSessions() ([]SessionInfo, error) {
	out, err := s.tmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if strings.Contains(err.Error(), "no server") || strings.Contains(err.Error(), "no current") {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []SessionInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, sessionPrefix) {
			continue
		}

		id := strings.TrimPrefix(line, sessionPrefix)

		sessions = append(sessions, SessionInfo{
			ID:      id,
			isAlive: s.isAlive(id),
		})
	}

	return sessions, nil
}

func waitForChannel(id string) string {
	return sessionName(id) + "-done"
}

func (s *Service) stare(ctx context.Context, id string, maxWait int) (output string, alive bool, exitCode int) {
	stareCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		s.tmuxWaitFor(stareCtx, waitForChannel(id))
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	deadline := time.NewTimer(time.Duration(maxWait) * time.Second)
	defer deadline.Stop()

	for {
		select {
		case <-doneCh:
			// The command records its exit code and *then* signals:
			//
			//   (cmd); __ec=$?; tmux set-option @nik_exit "$__ec"; \
			//     tmux wait-for -S <chan>; exit $__ec
			//
			// so arriving here means the code is already recorded, and this
			// returns on its first read. The wait is left in place for the
			// panes that reach here without having run that line at all.
			s.awaitDeadPane(id)

			out, _ := s.capturePane(id)
			c, _ := s.getExitCode(id)

			return out, false, c

		case <-deadline.C:
			out, _ := s.capturePane(id)
			if !s.isAlive(id) {
				c, _ := s.getExitCode(id)
				return out, false, c
			}
			return out, true, 0

		case <-ctx.Done():
			out, _ := s.capturePane(id)
			if !s.isAlive(id) {
				c, _ := s.getExitCode(id)
				return out, false, c
			}
			return out, true, 0

		case <-ticker.C:
			if !s.isAlive(id) {
				out, _ := s.capturePane(id)
				c, _ := s.getExitCode(id)
				return out, false, c
			}
		}
	}
}

func (s *Service) tmuxWaitFor(ctx context.Context, channel string) {
	if s.container != "" {
		exec.CommandContext(ctx, "docker", "exec", s.container, "tmux", "wait-for", channel).Run()
		return
	}

	exec.CommandContext(ctx, "tmux", "wait-for", channel).Run()
}

func (s *Service) tmux(args ...string) (string, error) {
	var cmd *exec.Cmd

	if s.container != "" {
		cmdArgs := append([]string{"exec", s.container, "tmux"}, args...)
		cmd = exec.Command("docker", cmdArgs...)
	} else {
		cmd = exec.Command("tmux", args...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}

	return string(out), nil
}
