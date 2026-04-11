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

var systemdUnitTmpl = template.Must(template.New("unit").Parse(`[Unit]
Description=nik daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{ .NikBinary }} daemon --home {{ .NikHome }}
Restart=on-failure
RestartSec=5
Environment=NIK_HOME={{ .NikHome }}

[Install]
WantedBy=default.target
`))

func systemdUnitPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}

	return filepath.Join(u.HomeDir, ".config", "systemd", "user", "nik.service"), nil
}

func installSystemd(nikBinary, nikHome string) error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(unitPath), 0o755)
	if err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}

	var buf bytes.Buffer
	err = systemdUnitTmpl.Execute(&buf, struct {
		NikBinary string
		NikHome   string
	}{
		NikBinary: nikBinary,
		NikHome:   nikHome,
	})
	if err != nil {
		return fmt.Errorf("render unit template: %w", err)
	}

	err = os.WriteFile(unitPath, buf.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}

	out, err = exec.Command("systemctl", "--user", "enable", "--now", "nik.service").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl enable: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func uninstallSystemd() error {
	_ = exec.Command("systemctl", "--user", "disable", "--now", "nik.service").Run()

	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}

	err = os.Remove(unitPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file: %w", err)
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	return nil
}

func isInstalledSystemd() bool {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(unitPath)
	return err == nil
}

func isRunningSystemd() (bool, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", "nik.service").CombinedOutput()
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(string(out)) == "active", nil
}
