package daemonctl

import (
	"fmt"
	"runtime"
)

// Install writes the service definition and, unless start is false, brings
// the daemon up on it — restarting one that is already running, which is what
// makes an upgrade take effect rather than leaving the old process serving
// from the inode its replaced binary left behind. start=false installs only:
// the new definition lands and the lifecycle is left to whoever owns it.
func Install(nikdBinary, nikHome string, start bool) error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(nikdBinary, nikHome, start)
	case "linux":
		return installSystemd(nikdBinary, nikHome, start)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func IsInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		return isInstalledLaunchd()
	case "linux":
		return isInstalledSystemd()
	default:
		return false
	}
}

func IsRunning() (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return isRunningLaunchd()
	case "linux":
		return isRunningSystemd()
	default:
		return false, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}
