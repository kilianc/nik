package shell

import (
	_ "embed"
	"errors"
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

	stock, err := s.seedDockerfile()
	if err != nil {
		return fmt.Errorf("seed dockerfile: %w", err)
	}

	image := s.desiredImage(stock)

	running, err := s.containerRunning(image)
	if err != nil {
		return fmt.Errorf("check container: %w", err)
	}

	if running {
		return nil
	}

	s.removeContainer()

	image, _, err = s.ensureImage(image)
	if err != nil {
		return err
	}

	err = s.startContainer(image)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	s.pruneImages(image)

	slog.Info("shell container started", "pkg", "shell", "container", s.container, "image", image)
	return nil
}

// rebuildContainer is the shell-rebuild tool: build what the Dockerfile on
// disk says, whatever it says. It never pulls — the whole point of asking for
// a rebuild is that the file changed and the family wants that file run.
func (s *Service) rebuildContainer() (string, error) {
	image := s.localImage()

	buildLog, err := s.buildImage(image)
	if err != nil {
		return buildLog, err
	}

	s.removeContainer()

	err = s.startContainer(image)
	if err != nil {
		return buildLog, fmt.Errorf("start container: %w", err)
	}

	slog.Info("shell container rebuilt", "pkg", "shell", "container", s.container, "image", image)
	return buildLog, nil
}

// factoryReset puts the Dockerfile back to the one nik ships and the sandbox
// back on the image that file describes — pulled, if this release published
// one, since a factory reset wants nik's own sandbox and not a private
// re-derivation of it.
func (s *Service) factoryReset() (string, error) {
	err := s.writeDefaultDockerfile()
	if err != nil {
		return "", err
	}

	image, log, err := s.ensureImage(s.desiredImage(true))
	if err != nil {
		return log, err
	}

	s.removeContainer()

	err = s.startContainer(image)
	if err != nil {
		return log, fmt.Errorf("start container: %w", err)
	}

	s.pruneImages(image)

	slog.Info("shell container reset", "pkg", "shell", "container", s.container, "image", image)
	return log, nil
}

func (s *Service) StopContainer() {
	if s.container == "" {
		return
	}

	exec.Command("docker", "stop", s.container).Run()
	s.removeContainer()
}

// seedDockerfile makes sure there is a Dockerfile to run and reports whether
// it is still nik's own. That answer is what decides pull against build, so it
// is an exact comparison rather than a guess: a family customising their
// sandbox is a deliberate feature, and getting this wrong would silently throw
// their work away.
func (s *Service) seedDockerfile() (bool, error) {
	path := s.dockerfilePath()

	current, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		err = s.writeDefaultDockerfile()
		if err != nil {
			return false, err
		}

		slog.Info("seeded default dockerfile", "pkg", "shell", "path", path)
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dockerfile: %w", err)
	}

	// Already this release's default. Record the digest even so: a family who
	// upgraded into this code has no marker yet, and this is where they get one.
	if string(current) == defaultDockerfile {
		s.markStock()
		return true, nil
	}

	if !s.matchesStockMarker(current) {
		return false, nil
	}

	// Untouched, but seeded by an older nik. The file has to move with the
	// release: it is what shell-rebuild builds and what a family reads to see
	// what their sandbox is, and both would otherwise describe an image that
	// is no longer the one running.
	err = s.writeDefaultDockerfile()
	if err != nil {
		return false, err
	}

	slog.Info("updated stock dockerfile", "pkg", "shell", "path", path)
	return true, nil
}

func (s *Service) buildImage(image string) (string, error) {
	ctx := filepath.Dir(s.dockerfilePath())

	cmd := exec.Command("docker", "build",
		"-t", image,
		"-f", s.dockerfilePath(),
		ctx,
	)

	out, err := cmd.CombinedOutput()
	buildLog := string(out)

	if err != nil {
		return buildLog, fmt.Errorf("docker build: %w", err)
	}

	slog.Info("shell image built", "pkg", "shell", "image", image)
	return buildLog, nil
}

func (s *Service) startContainer(image string) error {
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
	args = append(args, "-w", "/workspace", image, "sleep", "infinity")

	cmd := exec.Command("docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// containerRunning reports whether the sandbox is up on an image nik still
// considers current. The image is half the question: a nik that upgraded while
// its container kept running would otherwise sit on the previous release's
// sandbox indefinitely, since nothing about a running container goes stale on
// its own.
//
// A locally built image always counts as current. A host that fell back to
// building — offline, or behind a registry it cannot reach — would otherwise
// be torn down and stood up again on every boot, taking its live sessions with
// it, to reach an image it already knows it cannot pull.
func (s *Service) containerRunning(want string) (bool, error) {
	out, err := exec.Command("docker", "inspect",
		"--format", "{{.State.Running}} {{.Config.Image}}",
		s.container,
	).CombinedOutput()

	if err != nil {
		return false, nil
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 || fields[0] != "true" {
		return false, nil
	}

	return fields[1] == want || fields[1] == s.localImage(), nil
}

func (s *Service) imageExists(image string) (bool, error) {
	err := exec.Command("docker", "image", "inspect", image).Run()
	return err == nil, nil
}

func (s *Service) removeContainer() {
	exec.Command("docker", "rm", "-f", s.container).Run()
}
