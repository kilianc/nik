package daemonctl

import (
	"fmt"
	"runtime"
)

func Install(nikBinary, nikHome string) error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(nikBinary, nikHome)
	case "linux":
		return installSystemd(nikBinary, nikHome)
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
