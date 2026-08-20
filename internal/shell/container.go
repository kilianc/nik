package shell

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed Dockerfile
var defaultDockerfile string

func (s *Service) dockerfilePath() string {
	return filepath.Join(s.cfg.Home, "Dockerfile")
}

func (s *Service) ensureContainer() error {
	if s.container == "" {
		return nil
	}

	err := s.seedDockerfile()
	if err != nil {
		return fmt.Errorf("seed dockerfile: %w", err)
	}

	running, err := s.containerRunning()
	if err != nil {
		return fmt.Errorf("check container: %w", err)
	}

	if running {
		return nil
	}

	s.removeContainer()

	exists, err := s.imageExists()
	if err != nil {
		return fmt.Errorf("check image: %w", err)
	}

	if !exists {
		_, err = s.buildImage()
		if err != nil {
			return err
		}
	}

	err = s.startContainer()
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	slog.Info("shell container started", "pkg", "shell", "container", s.container, "image", s.dockerImage())
	return nil
}

func (s *Service) rebuildContainer() (string, error) {
	buildLog, err := s.buildImage()
	if err != nil {
		return buildLog, err
	}

	s.removeContainer()

	err = s.startContainer()
	if err != nil {
		return buildLog, fmt.Errorf("start container: %w", err)
	}

	slog.Info("shell container rebuilt", "pkg", "shell", "container", s.container, "image", s.dockerImage())
	return buildLog, nil
}

func (s *Service) factoryReset() (string, error) {
	err := os.MkdirAll(filepath.Dir(s.dockerfilePath()), 0o755)
	if err != nil {
		return "", fmt.Errorf("create dockerfile dir: %w", err)
	}

	err = os.WriteFile(s.dockerfilePath(), []byte(defaultDockerfile), 0o644)
	if err != nil {
		return "", fmt.Errorf("write default dockerfile: %w", err)
	}

	return s.rebuildContainer()
}

func (s *Service) StopContainer() {
	if s.container == "" {
		return
	}

	exec.Command("docker", "stop", s.container).Run()
	s.removeContainer()
}

func (s *Service) seedDockerfile() error {
	path := s.dockerfilePath()

	_, err := os.Stat(path)
	if err == nil {
		return nil
	}

	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return fmt.Errorf("create dockerfile dir: %w", err)
	}

	err = os.WriteFile(path, []byte(defaultDockerfile), 0o644)
	if err != nil {
		return fmt.Errorf("write default dockerfile: %w", err)
	}

	slog.Info("seeded default dockerfile", "pkg", "shell", "path", path)
	return nil
}

func (s *Service) buildImage() (string, error) {
	ctx := filepath.Dir(s.dockerfilePath())

	cmd := exec.Command("docker", "build",
		"-t", s.dockerImage()+":latest",
		"-f", s.dockerfilePath(),
		ctx,
	)

	out, err := cmd.CombinedOutput()
	buildLog := string(out)

	if err != nil {
		return buildLog, fmt.Errorf("docker build: %w", err)
	}

	slog.Info("shell image built", "pkg", "shell", "image", s.dockerImage())
	return buildLog, nil
}

func (s *Service) startContainer() error {
	args := []string{"run", "-d",
		"--name", s.container,
		"-v", s.cfg.Home + ":/workspace",
		"-v", s.cfg.Home + "/nik.db:/workspace/nik.db:ro",
		// An empty tmpfs over the socket directory. NIK_HOME is mounted
		// read-write and this container runs as root, so without this the
		// sandbox could reach nikd's owner socket — the one with no
		// restrictions on it — straight through the workspace mount. What it
		// gets instead is the narrow socket below, and nothing else.
		"--tmpfs", "/workspace/" + apiSocketDir,
	}

	// The narrowed API: named secrets, and refusals for the ones that are
	// nik's own identity rather than a credential nik holds for somebody.
	if s.sandboxSocket != "" {
		args = append(args,
			"-v", s.sandboxSocket+":"+containerSocketPath,
			"-e", "NIK_SOCKET="+containerSocketPath,
		)
	}
	// Mounted at /usr/local/bin/nik so `nik secrets read` keeps working from
	// workspace/secrets/cli and every skill that shells out to it — the name
	// inside the container is the contract, not which binary backs it.
	if bin := s.nikBinLinux(); bin != "" {
		args = append(args, "-v", bin+":"+containerNikBin+":ro")
	}
	args = append(args, "-w", "/workspace", s.dockerImage()+":latest", "sleep", "infinity")

	cmd := exec.Command("docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func (s *Service) containerRunning() (bool, error) {
	out, err := exec.Command("docker", "inspect",
		"--format", "{{.State.Running}}",
		s.container,
	).CombinedOutput()

	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(string(out)) == "true", nil
}

func (s *Service) imageExists() (bool, error) {
	err := exec.Command("docker", "image", "inspect", s.dockerImage()+":latest").Run()
	return err == nil, nil
}

func (s *Service) removeContainer() {
	exec.Command("docker", "rm", "-f", s.container).Run()
}
