package daemonctl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"
)

const launchdLabel = "com.nik.daemon"

var launchdPlistTmpl = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{ .Label }}</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{ .NikBinary }}</string>
    <string>daemon</string>
    <string>--home</string>
    <string>{{ .NikHome }}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>{{ .NikHome }}/launchd.stdout.log</string>
  <key>StandardErrorPath</key>
  <string>{{ .NikHome }}/launchd.stderr.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>NIK_HOME</key>
    <string>{{ .NikHome }}</string>
  </dict>
</dict>
</plist>
`))

func launchdPlistPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}

	return filepath.Join(u.HomeDir, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func installLaunchd(nikBinary, nikHome string) error {
	plistPath, err := launchdPlistPath()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(plistPath), 0o755)
	if err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	var buf bytes.Buffer
	err = launchdPlistTmpl.Execute(&buf, struct {
		Label     string
		NikBinary string
		NikHome   string
	}{
		Label:     launchdLabel,
		NikBinary: nikBinary,
		NikHome:   nikHome,
	})
	if err != nil {
		return fmt.Errorf("render plist template: %w", err)
	}

	err = os.WriteFile(plistPath, buf.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	uid := fmt.Sprintf("gui/%d", os.Getuid())

	_ = exec.Command("launchctl", "bootout", uid, plistPath).Run()

	out, err := exec.Command("launchctl", "bootstrap", uid, plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func uninstallLaunchd() error {
	plistPath, err := launchdPlistPath()
	if err != nil {
		return err
	}

	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", uid, plistPath).Run()

	err = os.Remove(plistPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}

	return nil
}

func isInstalledLaunchd() bool {
	plistPath, err := launchdPlistPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(plistPath)
	return err == nil
}

func isRunningLaunchd() (bool, error) {
	out, err := exec.Command("launchctl", "list", launchdLabel).CombinedOutput()
	if err != nil {
		return false, nil
	}

	return strings.Contains(string(out), launchdLabel), nil
}
