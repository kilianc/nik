package api

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

// The API is never served on a network port. A managed nik's whole shape
// depends on having no inbound port — no DNS, no certificates, no
// reachability problem — and a loopback listener would be the first crack in
// that. Remote access, when it comes, rides the socket nikd already dials out
// on, not one anybody can reach.

// SocketDir is where nikd's listeners live, relative to NIK_HOME.
const SocketDir = "run"

// OwnerSocketName is the socket only the user running nikd may reach.
const OwnerSocketName = "nikd.sock"

// OwnerSocketPath is where nikctl looks and nikd listens.
func OwnerSocketPath(home string) string {
	return filepath.Join(home, SocketDir, OwnerSocketName)
}

// Listen creates a unix socket with the caller as its only reachable user.
//
// Two locks, not one: the socket is 0600, and the directory holding it is
// 0700. The directory is what actually carries the guarantee — unix socket
// permissions are honoured by Linux and macOS but are not portable, while a
// directory nobody else may traverse is enforced everywhere and by the same
// rules as every other file. Anyone who can defeat both can already read
// NIK_HOME's database, so this is a boundary rather than a wall.
//
// A stale socket from a killed daemon is removed: bind fails on an existing
// path, and the pid file — checked before any of this — is what actually
// answers "is another daemon running".
func Listen(path string) (net.Listener, error) {
	err := checkPathLength(path)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)

	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	// MkdirAll leaves an existing directory's mode alone, so say it again for
	// the upgrade case: a home that predates this code has no run/ at all,
	// but one written by a future version with different ideas would.
	err = os.Chmod(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("lock down socket dir: %w", err)
	}

	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}

	err = os.Chmod(path, 0o600)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("lock down socket: %w", err)
	}

	return ln, nil
}

// maxSocketPath is the sun_path limit: a unix socket address is a fixed-size
// field in a C struct, and a path past it fails to bind with nothing more
// helpful than "invalid argument".
func maxSocketPath() int {
	if runtime.GOOS == "linux" {
		return 108
	}

	return 104
}

// checkPathLength turns the kernel's worst error message into one that names
// the cause. A default NIK_HOME is nowhere near the limit; a deeply nested one
// — a temp dir in a test, a home under several layers of mount point — can be.
func checkPathLength(path string) error {
	limit := maxSocketPath()
	if len(path) < limit {
		return nil
	}

	return fmt.Errorf(
		"socket path is %d bytes, over this platform's %d-byte limit: %s\n"+
			"set NIK_HOME to a shorter path", len(path), limit, path)
}
