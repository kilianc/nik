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
		wrapped := fmt.Sprintf("(%s); __ec=$?; tmux wait-for -S %s; exit $__ec", command, ch)
		_, err = s.tmux("respawn-pane", "-k", "-t", name, "sh", "-c", wrapped)
		if err != nil {
			return fmt.Errorf("respawn pane %s: %w", id, err)
		}
	}

	return nil
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

// deadPaneWait bounds how long stare waits for a pane to finish dying. The
// gap is the microseconds between `tmux wait-for -S` returning and the shell
// running `exit`; a whole second is there for a loaded CI runner, not because
// anything should ever take it.
const deadPaneWait = time.Second

// awaitDeadPane polls until the pane is dead or the wait runs out. Returning
// on timeout rather than erroring is deliberate: the caller has a
// better-than-nothing answer either way, and a command whose exit code cannot
// be read is not a reason to lose its output.
func (s *Service) awaitDeadPane(id string) {
	deadline := time.Now().Add(deadPaneWait)

	for {
		// Poll the value stare is about to read, rather than the pane_dead
		// flag sitting next to it.
		//
		// They are two properties and they do not arrive together: a pane
		// reports itself dead a moment before tmux publishes the status it
		// died with. So a wait on the flag can finish while the status is
		// still empty, and the read that follows returns the -1 this function
		// exists to prevent. Waiting on the flag is why it kept happening
		// after somebody had already fixed it once.
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

func (s *Service) getExitCode(id string) (int, error) {
	out, err := s.tmux(
		"display-message", "-t", sessionName(id),
		"-p", "#{pane_dead_status}",
	)
	if err != nil {
		return -1, fmt.Errorf("exit code %s: %w", id, err)
	}

	var code int
	_, err = fmt.Sscanf(strings.TrimSpace(out), "%d", &code)
	if err != nil {
		// No status published yet, which is not the same thing as a command
		// that exited -1. Returning a nil error here told every caller the
		// read had worked and handed them a sentinel as though it were an
		// answer — so nothing could wait for a value that had not arrived,
		// because nothing could tell that it had not.
		return -1, fmt.Errorf("exit code %s: no status published yet", id)
	}

	return code, nil
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
			// The command signals the wait-for channel and *then* exits:
			//
			//   (cmd); __ec=$?; tmux wait-for -S <chan>; exit $__ec
			//
			// so arriving here means the command finished, not that the pane
			// is dead. tmux only publishes pane_dead_status once it is, and
			// reading too early gets an empty string — which parsed as -1 and
			// made this the flakiest thing in CI. Give the pane the moment it
			// needs to actually die.
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
