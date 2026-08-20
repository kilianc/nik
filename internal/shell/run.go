package shell

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/kciuffolo/nik/internal/id"
)

func (s *Service) RunCommand(ctx context.Context, command, stdin string) (string, string, error) {
	err := s.ensureReady()
	if err != nil {
		return "", "", fmt.Errorf("ensure shell ready: %w", err)
	}

	var cmd *exec.Cmd

	if s.container != "" {
		cmd = exec.CommandContext(ctx, "docker", "exec", "-i",
			"-w", s.workdir(),
			s.container,
			"sh", "-c", command,
		)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = s.cfg.Home
	}

	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return "", stderr.String(), fmt.Errorf("run command: %w", err)
	}

	return stdout.String(), stderr.String(), nil
}

// RunForAPI runs one command and waits for it, returning the output, whether
// it is still running, and its exit code.
//
// It is the shell tool's own path — a session, then stare — rather than a bare
// exec, because the exit code is the part a caller cannot get any other way,
// and a command that outlives its wait keeps running rather than being killed
// out from under whoever asked. What it does not do is the tool's bookkeeping:
// no session metadata, no persisted output, no activation to attach to. A
// person at a console is not an activation.
func (s *Service) RunForAPI(ctx context.Context, command string, maxWaitSeconds int) (output string, running bool, exitCode int, err error) {
	if command == "" {
		return "", false, 0, fmt.Errorf("empty command")
	}

	err = s.ensureReady()
	if err != nil {
		return "", false, 0, fmt.Errorf("ensure shell ready: %w", err)
	}

	if maxWaitSeconds <= 0 {
		maxWaitSeconds = defaultAPIWait
	}
	maxWaitSeconds = min(maxWaitSeconds, maxAPIWait)

	sid := id.Short(4)

	err = s.newSession(sid, command, s.workdir())
	if err != nil {
		return "", false, 0, fmt.Errorf("start session: %w", err)
	}

	slog.Info("shell run over the api", "pkg", "shell", "id", sid, "command", command)

	output, running, exitCode = s.stare(ctx, sid, maxWaitSeconds)
	if !running {
		s.killSession(sid)
	}

	return output, running, exitCode, nil
}

const (
	// defaultAPIWait matches the shell tool's default: long enough for
	// anything interactive to have said something, short enough that a
	// console does not appear hung.
	defaultAPIWait = 10
	// maxAPIWait bounds what a caller can ask for. A command that needs
	// longer keeps running; the caller is told so and can look again.
	maxAPIWait = 120
)
