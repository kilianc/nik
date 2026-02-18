package shell

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
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
	ID    string
	Alive bool
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
func newSession(id, command string) error {
	name := sessionName(id)

	err := tmuxCmd(
		"new-session", "-d",
		"-s", name,
		"-x", fmt.Sprintf("%d", windowWidth),
		"-y", fmt.Sprintf("%d", windowHeight),
	)
	if err != nil {
		return fmt.Errorf("create session %s: %w", id, err)
	}

	err = tmuxCmd("set-option", "-t", name, "remain-on-exit", "on")
	if err != nil {
		return fmt.Errorf("set remain-on-exit %s: %w", id, err)
	}

	err = tmuxCmd("set-option", "-t", name, "history-limit", fmt.Sprintf("%d", historyLimit))
	if err != nil {
		return fmt.Errorf("set history-limit %s: %w", id, err)
	}

	if command != "" {
		err = tmuxCmd("respawn-pane", "-k", "-t", name, "sh", "-c", command)
		if err != nil {
			return fmt.Errorf("respawn pane %s: %w", id, err)
		}
	}

	return nil
}

func setEnv(id, key, value string) error {
	err := tmuxCmd("set-environment", "-t", sessionName(id), key, value)
	if err != nil {
		return fmt.Errorf("set env %s %s: %w", id, key, err)
	}

	return nil
}

func getEnv(id, key string) (string, error) {
	out, err := tmuxOutput("show-environment", "-t", sessionName(id), key)
	if err != nil {
		return "", nil
	}

	parts := strings.SplitN(strings.TrimSpace(out), "=", 2)
	if len(parts) != 2 {
		return "", nil
	}

	return parts[1], nil
}

func getAllEnv(id string) (map[string]string, error) {
	out, err := tmuxOutput("show-environment", "-t", sessionName(id))
	if err != nil {
		return nil, nil
	}

	env := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "-") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		env[parts[0]] = parts[1]
	}

	return env, nil
}

func sendKeys(id string, keys ...string) error {
	args := []string{"send-keys", "-t", sessionName(id)}
	args = append(args, keys...)

	err := tmuxCmd(args...)
	if err != nil {
		return fmt.Errorf("send keys %s: %w", id, err)
	}

	return nil
}

func captureOutput(id string) (string, error) {
	out, err := tmuxOutput("capture-pane", "-t", sessionName(id), "-p", "-S", "-")
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

func isAlive(id string) (bool, error) {
	out, err := tmuxOutput(
		"display-message", "-t", sessionName(id),
		"-p", "#{pane_dead}",
	)
	if err != nil {
		return false, fmt.Errorf("check alive %s: %w", id, err)
	}

	return strings.TrimSpace(out) == "0", nil
}

func exitCode(id string) (int, error) {
	out, err := tmuxOutput(
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
	err := tmuxCmd("kill-session", "-t", sessionName(id))
	if err != nil {
		return fmt.Errorf("kill session %s: %w", id, err)
	}

	return nil
}

func listSessions() ([]SessionInfo, error) {
	out, err := tmuxOutput("list-sessions", "-F", "#{session_name}")
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

		alive, err := isAlive(id)
		if err != nil {
			alive = false
		}

		sessions = append(sessions, SessionInfo{
			ID:    id,
			Alive: alive,
		})
	}

	return sessions, nil
}

func tmuxCmd(args ...string) error {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}

	return nil
}

func tmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}

	return string(out), nil
}
