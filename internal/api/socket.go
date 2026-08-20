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

// SandboxSocketName is the narrowed socket bind-mounted into the shell
// container. It lives in the same directory as the owner socket, which the
// container no longer sees: the shell service shadows NIK_HOME/run with an
// empty tmpfs and mounts this one file in by itself.
const SandboxSocketName = "sandbox.sock"

// OwnerSocketPath is where nikctl looks and nikd listens.
func OwnerSocketPath(home string) string {
	return filepath.Join(home, SocketDir, OwnerSocketName)
}

// SandboxSocketPath is where the narrowed socket lives on the host.
func SandboxSocketPath(home string) string {
	return filepath.Join(home, SocketDir, SandboxSocketName)
}

// ContainerSocketPath is where the sandbox socket is mounted inside the shell
// container, and what NIK_SOCKET points at in there. Outside NIK_HOME on
// purpose: the workspace mount is the thing being locked down.
const ContainerSocketPath = "/run/nik.sock"

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

// ListenSandbox creates the socket the shell container reaches nikd through.
//
// 0666 rather than 0600, and that is not a mistake: the container runs as
// root and bind-mounts this one file, so the file's mode is not what limits
// it — the scope attached to the listener is. Everything on the host is still
// behind the 0700 directory.
func ListenSandbox(path string) (net.Listener, error) {
	ln, err := Listen(path)
	if err != nil {
		return nil, err
	}

	err = os.Chmod(path, 0o666)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("open up sandbox socket: %w", err)
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
