package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(&config.Config{
		Home:  t.TempDir(),
		Shell: config.ShellConfig{DockerImage: "nik-shell"},
	}, nil, "")
}

func TestSeedDockerfile(t *testing.T) {
	t.Run("creates default when missing", func(t *testing.T) {
		svc := newTestService(t)

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if !stock {
			t.Fatal("a freshly seeded Dockerfile is not stock")
		}

		data, err := os.ReadFile(svc.dockerfilePath())
		if err != nil {
			t.Fatalf("read dockerfile: %v", err)
		}

		if string(data) != defaultDockerfile {
			t.Fatalf("unexpected dockerfile content: %s", data)
		}
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		svc := newTestService(t)

		custom := "FROM ubuntu:24.04\n"
		err := os.WriteFile(svc.dockerfilePath(), []byte(custom), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if stock {
			t.Fatal("a customised Dockerfile reported as stock")
		}

		data, err := os.ReadFile(svc.dockerfilePath())
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if string(data) != custom {
			t.Fatalf("seedDockerfile overwrote existing file")
		}
	})

	// A family who upgraded into this code has a Dockerfile but no marker.
	// Byte-identical to the one nik ships is the only evidence there is, and
	// it is enough.
	t.Run("adopts an unmarked but identical dockerfile", func(t *testing.T) {
		svc := newTestService(t)

		err := os.WriteFile(svc.dockerfilePath(), []byte(defaultDockerfile), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if !stock {
			t.Fatal("an untouched shipped Dockerfile reported as customised")
		}

		if !svc.matchesStockMarker([]byte(defaultDockerfile)) {
			t.Fatal("seedDockerfile did not record the digest")
		}
	})

	// The one this whole scheme exists for: an untouched Dockerfile from an
	// older release. It must upgrade to the new default and stay stock, or
	// every family who never customised anything builds locally forever.
	t.Run("upgrades an untouched dockerfile from an older release", func(t *testing.T) {
		svc := newTestService(t)

		old := "FROM golang:1.24.12-bookworm\nCMD [\"sleep\", \"infinity\"]\n"
		err := os.WriteFile(svc.dockerfilePath(), []byte(old), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		err = os.WriteFile(svc.stockMarkerPath(), []byte(digest([]byte(old))+"\n"), 0o644)
		if err != nil {
			t.Fatalf("write marker: %v", err)
		}

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if !stock {
			t.Fatal("an untouched older Dockerfile reported as customised")
		}

		data, err := os.ReadFile(svc.dockerfilePath())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != defaultDockerfile {
			t.Fatalf("seedDockerfile did not upgrade the file: %s", data)
		}
	})

	// The other half of the same question, and the expensive one to get wrong:
	// an edited file whose marker records what it used to be.
	t.Run("keeps an edited dockerfile that was once stock", func(t *testing.T) {
		svc := newTestService(t)

		err := os.WriteFile(svc.stockMarkerPath(),
			[]byte(digest([]byte(defaultDockerfile))+"\n"), 0o644)
		if err != nil {
			t.Fatalf("write marker: %v", err)
		}

		custom := defaultDockerfile + "\nRUN apt-get install -y --no-install-recommends python3\n"
		err = os.WriteFile(svc.dockerfilePath(), []byte(custom), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if stock {
			t.Fatal("an edited Dockerfile reported as stock")
		}

		data, err := os.ReadFile(svc.dockerfilePath())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != custom {
			t.Fatal("seedDockerfile threw away a customised Dockerfile")
		}
	})

	// Reverting an edit by hand is a return to stock, marker or no marker.
	t.Run("re-adopts a dockerfile edited back to the default", func(t *testing.T) {
		svc := newTestService(t)

		err := os.WriteFile(svc.stockMarkerPath(), []byte(digest([]byte("something else"))+"\n"), 0o644)
		if err != nil {
			t.Fatalf("write marker: %v", err)
		}
		err = os.WriteFile(svc.dockerfilePath(), []byte(defaultDockerfile), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		stock, err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}
		if !stock {
			t.Fatal("a Dockerfile reverted to the default reported as customised")
		}
	})
}

func TestWriteDefaultDockerfileRecordsMarker(t *testing.T) {
	svc := newTestService(t)

	err := svc.writeDefaultDockerfile()
	if err != nil {
		t.Fatalf("writeDefaultDockerfile: %v", err)
	}

	marker, err := os.ReadFile(filepath.Join(svc.cfg.Home, stockMarker))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if len(marker) == 0 {
		t.Fatal("marker is empty")
	}
	if !svc.matchesStockMarker([]byte(defaultDockerfile)) {
		t.Fatalf("marker does not match the file it was written for: %s", marker)
	}
}

// The estate's configuration reaches the sandbox, so a managed nik's skills
// call the estate's endpoints instead of the vendors' directly.
func TestSandboxEnvReachesTheContainer(t *testing.T) {
	s := &Service{cfg: &config.Config{
		Home: t.TempDir(),
		Shell: config.ShellConfig{Env: map[string]string{
			"EXA_BASE_URL": "https://exa-dev.hellonik.com",
			"X_BASE_URL":   "https://x-dev.hellonik.com",
		}},
	}}

	args := strings.Join(s.runArgs("nik-shell:test"), " ")
	for _, want := range []string{
		"-e EXA_BASE_URL=https://exa-dev.hellonik.com",
		"-e X_BASE_URL=https://x-dev.hellonik.com",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("docker run is missing %q\ngot: %s", want, args)
		}
	}
}

// Same configuration, same command. A map that iterated differently every time
// would recreate a container on every check for no reason at all.
func TestSandboxEnvIsOrdered(t *testing.T) {
	cfg := config.Config{
		Home: t.TempDir(),
		Shell: config.ShellConfig{Env: map[string]string{
			"D_URL": "4", "A_URL": "1", "C_URL": "3", "B_URL": "2",
		}},
	}
	s := &Service{cfg: &cfg}

	first := strings.Join(s.runArgs("img"), " ")
	for range 20 {
		if got := strings.Join(s.runArgs("img"), " "); got != first {
			t.Fatalf("the command changed between builds:\n%s\n%s", first, got)
		}
	}
	// And in the order somebody reading it would expect.
	if !strings.Contains(first, "-e A_URL=1 -e B_URL=2 -e C_URL=3 -e D_URL=4") {
		t.Errorf("environment is not sorted: %s", first)
	}
}

// No configuration adds no flags. A self-hosting family's container is exactly
// what it was.
func TestNoSandboxEnvAddsNothing(t *testing.T) {
	s := &Service{cfg: &config.Config{Home: t.TempDir()}}
	if got := strings.Join(s.runArgs("img"), " "); strings.Contains(got, "-e EXA") {
		t.Errorf("unexpected environment: %s", got)
	}
}

// The masks and mounts that make the sandbox a sandbox, asserted here because
// runArgs is where they live and a regression in any of them is silent.
func TestTheSandboxBoundaryHolds(t *testing.T) {
	home := t.TempDir()
	s := &Service{cfg: &config.Config{Home: home}}
	args := strings.Join(s.runArgs("img"), " ")

	// The owner socket's directory is masked. NIK_HOME is mounted read-write
	// and this container runs as root, so without the tmpfs the sandbox
	// reaches the unrestricted socket straight through the workspace mount.
	if !strings.Contains(args, "--tmpfs /workspace/"+apiSocketDir) {
		t.Errorf("the socket directory is not masked: %s", args)
	}
	if !strings.Contains(args, "-v "+home+":/workspace") {
		t.Errorf("the workspace is not mounted: %s", args)
	}
}
