// Package home resolves NIK_HOME. It exists so nikd and nikctl agree on
// which directory they are talking about without either importing the
// other's world: nikd owns what is inside the directory, nikctl only ever
// needs to know where it is.
package home

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// Resolve returns the absolute workspace path, creating it if needed.
// Precedence: explicit override, then NIK_HOME, then ~/.nik.
func Resolve(override string) (string, error) {
	h := override
	if h == "" {
		h = os.Getenv("NIK_HOME")
	}
	if h == "" {
		u, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("get current user: %w", err)
		}
		h = filepath.Join(u.HomeDir, ".nik")
	}

	abs, err := filepath.Abs(h)
	if err != nil {
		return "", fmt.Errorf("resolve home path: %w", err)
	}

	err = os.MkdirAll(abs, 0o755)
	if err != nil {
		return "", fmt.Errorf("create home dir: %w", err)
	}

	return abs, nil
}
