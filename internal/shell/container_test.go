package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestSeedDockerfile(t *testing.T) {
	t.Run("creates default when missing", func(t *testing.T) {
		home := t.TempDir()
		svc := NewService(&config.Config{Home: home, Shell: config.ShellConfig{DockerImage: "nik-shell"}}, nil)

		err := svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(home, "shell", "Dockerfile"))
		if err != nil {
			t.Fatalf("read dockerfile: %v", err)
		}

		if string(data) != defaultDockerfile {
			t.Fatalf("unexpected dockerfile content: %s", data)
		}
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		home := t.TempDir()
		svc := NewService(&config.Config{Home: home, Shell: config.ShellConfig{DockerImage: "nik-shell"}}, nil)

		dir := filepath.Join(home, "shell")
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		custom := "FROM ubuntu:24.04\n"
		err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(custom), 0o644)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		err = svc.seedDockerfile()
		if err != nil {
			t.Fatalf("seedDockerfile: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if string(data) != custom {
			t.Fatalf("seedDockerfile overwrote existing file")
		}
	})
}

func TestFactoryResetOverwritesDockerfile(t *testing.T) {
	home := t.TempDir()
	svc := NewService(&config.Config{
		Home:  home,
		Shell: config.ShellConfig{DockerImage: "nik-shell-test"},
	}, nil)
	svc.container = "nik-shell-test"

	dir := filepath.Join(home, "shell")
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	custom := "FROM ubuntu:24.04\nRUN apt-get install -y python3\n"
	err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(custom), 0o644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	svc.factoryReset()

	data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(data) != defaultDockerfile {
		t.Fatalf("factoryReset did not reset Dockerfile, got: %s", data)
	}
}
