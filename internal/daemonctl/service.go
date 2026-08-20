package daemonctl

import (
	"fmt"
	"runtime"
)

func Install(nikdBinary, nikHome string) error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(nikdBinary, nikHome)
	case "linux":
		return installSystemd(nikdBinary, nikHome)
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
