package shell

import (
	"context"
	"fmt"
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
		return -1, nil
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
