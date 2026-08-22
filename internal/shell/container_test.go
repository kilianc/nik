package shell

import (
	"os"
	"path/filepath"
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
