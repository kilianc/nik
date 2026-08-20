package daemonctl

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"
)

const systemdUnitName = "nikd.service"

//go:embed nikd.service.tmpl
var systemdUnitTmplSrc string

var systemdUnitTmpl = template.Must(template.New("unit").Parse(systemdUnitTmplSrc))

type systemdUnitData struct {
	NikdBinary string
	NikHome    string
	WantedBy   string
}

// systemdScope is which of the host's two service managers nikd goes into. A
// laptop has a user manager and that is where nikd belongs: it starts with the
// session and needs no privilege. A box driven by `docker exec` has no login
// session at all — pam_systemd never ran, so no user manager was ever started
// and the system one is the only manager there is.
type systemdScope struct {
	user bool
}

func (s systemdScope) unitPath() (string, error) {
	if !s.user {
		return filepath.Join("/etc/systemd/system", systemdUnitName), nil
	}

	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}

	return filepath.Join(u.HomeDir, ".config", "systemd", "user", systemdUnitName), nil
}

func (s systemdScope) wantedBy() string {
	if !s.user {
		return "multi-user.target"
	}

	return "default.target"
}

func (s systemdScope) systemctl(args ...string) *exec.Cmd {
	if s.user {
		args = append([]string{"--user"}, args...)
	}

	return exec.Command("systemctl", args...)
}

// systemdUserManagerReachable answers the only question that matters for the
// choice: can this process talk to a user service manager at all. Being root
// is not that question — root on a desktop has a real session and should keep
// getting a user service.
func systemdUserManagerReachable() bool {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir != "" {
		_, err := os.Stat(filepath.Join(dir, "systemd", "private"))
		if err == nil {
			return true
		}
	}

	// XDG_RUNTIME_DIR is not always set where a manager does exist (`su -`, a
	// cron job), so ask systemd itself before concluding there is none.
	// Reading a property connects to the bus and changes nothing.
	err := exec.Command("systemctl", "--user", "show", "--property=Version").Run()
	return err == nil
}

// systemdScopeFor is the branch, split out from the environment it reads so a
// test can state the case instead of arranging a container. Root with no user
// manager is the nik-saas cell: the installer runs through `docker exec`,
// which is not a login session. A non-root user goes to the user manager
// either way — /etc/systemd/system is not theirs to write.
func systemdScopeFor(euid int, userManagerReachable bool) systemdScope {
	return systemdScope{user: euid != 0 || userManagerReachable}
}

func currentSystemdScope() systemdScope {
	return systemdScopeFor(os.Geteuid(), systemdUserManagerReachable())
}

func installSystemd(nikdBinary, nikHome string) error {
	scope := currentSystemdScope()

	unitPath, err := scope.unitPath()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(unitPath), 0o755)
	if err != nil {
		return fmt.Errorf("create systemd unit dir: %w", err)
	}

	var buf bytes.Buffer
	err = systemdUnitTmpl.Execute(&buf, systemdUnitData{
		NikdBinary: nikdBinary,
		NikHome:    nikHome,
		WantedBy:   scope.wantedBy(),
	})
	if err != nil {
		return fmt.Errorf("render unit template: %w", err)
	}

	err = os.WriteFile(unitPath, buf.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	out, err := scope.systemctl("daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}

	out, err = scope.systemctl("enable", "--now", systemdUnitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl enable: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func uninstallSystemd() error {
	scope := currentSystemdScope()

	_ = scope.systemctl("disable", "--now", systemdUnitName).Run()

	unitPath, err := scope.unitPath()
	if err != nil {
		return err
	}

	err = os.Remove(unitPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file: %w", err)
	}

	_ = scope.systemctl("daemon-reload").Run()

	return nil
}

func isInstalledSystemd() bool {
	unitPath, err := currentSystemdScope().unitPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(unitPath)
	return err == nil
}

func isRunningSystemd() (bool, error) {
	out, err := currentSystemdScope().systemctl("is-active", systemdUnitName).CombinedOutput()
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(string(out)) == "active", nil
}
