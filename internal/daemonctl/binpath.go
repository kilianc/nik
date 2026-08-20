package daemonctl

import (
	"fmt"
	"os"
	"path/filepath"
)

// SiblingBinary finds the other half of the install: nikctl asking for nikd
// so it can write a service file, nikd asking for nikctl so it can mount a
// client — and nothing else — into the shell sandbox.
//
// The rule is "next to me", which is what both installs produce: the release
// tarball unpacks both into NIK_INSTALL_DIR, and `make build` writes both into
// bin/. Symlinks are resolved first, since `nik` is a link to `nikctl` and the
// answer has to be relative to the real file, not to the name it was called by.
func SiblingBinary(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve own path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve own path: %w", err)
	}

	path := filepath.Join(filepath.Dir(resolved), name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%s not found next to %s: %w",
			name, filepath.Base(resolved), err)
	}

	return path, nil
}
