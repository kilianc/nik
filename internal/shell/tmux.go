package shell

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var sessionPrefix = "nik-"

const (
	maxOutputBytes = 16 * 1024
	historyLimit   = 10000
	windowWidth    = 200
	windowHeight   = 50
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type SessionInfo struct {
	ID      string
	isAlive bool
}

func sessionName(id string) string {
	return sessionPrefix + id
}

func ensureTmux() error {
	_, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("find tmux: %w", err)
	}

	return nil
}

// newSession creates a tmux session with remain-on-exit on, then replaces the
// pane process with sh -c command. Options are set before the command runs so
// fast-exiting commands still have pane_dead captured.
func newSession(id, command, cwd string) error {
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

	_, err := tmux(args...)
	if err != nil {
		return fmt.Errorf("create session %s: %w", id, err)
	}

	_, err = tmux("set-option", "-t", name, "remain-on-exit", "on")
	if err != nil {
		return fmt.Errorf("set remain-on-exit %s: %w", id, err)
	}

	_, err = tmux("set-option", "-t", name, "history-limit", fmt.Sprintf("%d", historyLimit))
	if err != nil {
		return fmt.Errorf("set history-limit %s: %w", id, err)
	}

	if command != "" {
		_, err = tmux("respawn-pane", "-k", "-t", name, "sh", "-c", command)
		if err != nil {
			return fmt.Errorf("respawn pane %s: %w", id, err)
		}
	}

	return nil
}

func setEnv(id, key, value string) error {
	_, err := tmux("set-environment", "-t", sessionName(id), key, value)
	if err != nil {
		return fmt.Errorf("set env %s %s: %w", id, key, err)
	}

	return nil
}

func getEnv(id, key string) (string, error) {
	out, err := tmux("show-environment", "-t", sessionName(id), key)
	if err != nil {
		return "", fmt.Errorf("get env %s %s: %w", id, key, err)
	}

	parts := strings.SplitN(strings.TrimSpace(out), "=", 2)
	if len(parts) != 2 {
		return "", nil
	}

	return parts[1], nil
}

func sendKeys(id string, keys ...string) error {
	args := []string{"send-keys", "-t", sessionName(id)}
	args = append(args, keys...)

	_, err := tmux(args...)
	if err != nil {
		return fmt.Errorf("send keys %s: %w", id, err)
	}

	return nil
}

func capturePane(id string) (string, error) {
	out, err := tmux("capture-pane", "-t", sessionName(id), "-p", "-S", "-")
	if err != nil {
		return "", fmt.Errorf("capture output %s: %w", id, err)
	}

	out = ansiRe.ReplaceAllString(out, "")
	out = strings.TrimRight(out, "\n ")

	if len(out) > maxOutputBytes {
		out = out[len(out)-maxOutputBytes:]
	}

	return out, nil
}

func isAlive(id string) bool {
	out, err := tmux(
		"display-message", "-t", sessionName(id),
		"-p", "#{pane_dead}",
	)
	if err != nil {
		return false
	}

	return strings.TrimSpace(out) == "0"
}

func getExitCode(id string) (int, error) {
	out, err := tmux(
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

func killSession(id string) error {
	_, err := tmux("kill-session", "-t", sessionName(id))
	if err != nil {
		return fmt.Errorf("kill session %s: %w", id, err)
	}

	return nil
}

func listSessions() ([]SessionInfo, error) {
	out, err := tmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// no server running = no sessions
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
			isAlive: isAlive(id),
		})
	}

	return sessions, nil
}

func stare(id string, maxWait int, watchFor string) (output string, alive bool, exitCode int) {
	return stareWith(id, maxWait, watchFor, 0)
}

func stareWith(id string, maxWait int, watchFor string, baseline int) (output string, alive bool, exitCode int) {
	deadline := time.Now().Add(time.Duration(maxWait) * time.Second)

	for {
		if !isAlive(id) {
			out, _ := capturePane(id)
			c, _ := getExitCode(id)
			return out, false, c
		}

		if watchFor != "" {
			out, _ := capturePane(id)
			newContent := ""
			if baseline < len(out) {
				newContent = out[baseline:]
			}
			if strings.Contains(newContent, watchFor) {
				return out, true, 0
			}
		}

		if time.Now().After(deadline) {
			out, _ := capturePane(id)
			return out, true, 0
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}

	return string(out), nil
}
