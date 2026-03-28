package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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
