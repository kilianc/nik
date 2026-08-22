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
// manager is a container: the installer runs through `docker exec`,
// which is not a login session. A non-root user goes to the user manager
// either way — /etc/systemd/system is not theirs to write.
func systemdScopeFor(euid int, userManagerReachable bool) systemdScope {
	return systemdScope{user: euid != 0 || userManagerReachable}
}

func currentSystemdScope() systemdScope {
	return systemdScopeFor(os.Geteuid(), systemdUserManagerReachable())
}

// systemctlStep is one command install runs once the unit file is on disk.
// The sequence is split out from the running of it so a test can state the
// case instead of arranging a container to watch.
type systemctlStep struct {
	args       []string
	bestEffort bool
}

// systemdInstallSteps brings the service up on the unit that was just written,
// or stops after writing it when the caller asked to install only.
//
// `restart` rather than `enable --now`: `--now` starts a *stopped* unit and
// leaves a running one alone, so an install over a running nik replaced both
// binaries on disk and left the old daemon serving from its old inode —
// silently, with `nikctl version` reading the new binary on disk and reporting
// success. `restart` covers the upgrade and starts a stopped unit just as
// well, so first installs still end running.
//
// `reset-failed` before it because a daemon that has spent its start limit
// answers `restart` with "Start request repeated too quickly" and does
// nothing — so the fix that was just installed would never run. An install is
// precisely the moment the reason for a crash loop is most likely to have just
// been replaced, so the old binary's failure count is not evidence about the
// new one. It clears the counter for this unit only, and buys no silence: a
// new binary that also crashes loops and lands back in `failed` on its own
// record. On a unit that never failed it is a no-op, which is why it needs no
// prior state read to decide on.
func systemdInstallSteps(start bool) []systemctlStep {
	steps := []systemctlStep{
		{args: []string{"daemon-reload"}},
		{args: []string{"enable", systemdUnitName}},
	}

	if !start {
		return steps
	}

	return append(steps,
		systemctlStep{args: []string{"reset-failed", systemdUnitName}, bestEffort: true},
		systemctlStep{args: []string{"restart", systemdUnitName}},
	)
}

// removeStaleScopeUnit clears the unit out of the scope we are *not*
// installing into. Every install before v0.4.2 wrote a user unit, so a box
// that has since been re-installed somewhere without a user manager — a
// container, reached by `docker exec` — keeps the old file forever. The
// other direction is the one that bites: a box that gains a session between
// installs moves to a user unit while the system daemon goes on running, and
// two daemons share one home, one SQLite file and one socket.
//
// Stop it before the new unit is written, not after: a leftover unit file
// nobody reloads is litter, but two live daemons on one home are a corruption
// bug. That is also why this runs under --no-start, which is a promise about
// the unit being installed and not a licence to leave a second daemon holding
// the same SQLite file. Everything here is best effort — the other scope
// routinely needs privileges this process does not have, and never having had
// a unit there is the normal case.
func removeStaleScopeUnit(stale systemdScope) {
	unitPath, err := stale.unitPath()
	if err != nil {
		return
	}

	_, err = os.Stat(unitPath)
	if err != nil {
		return
	}

	_ = stale.systemctl("disable", "--now", systemdUnitName).Run()
	_ = os.Remove(unitPath)
	_ = stale.systemctl("daemon-reload").Run()
}

func installSystemd(nikdBinary, nikHome string, start bool) error {
	scope := currentSystemdScope()

	removeStaleScopeUnit(systemdScope{user: !scope.user})

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

	for _, step := range systemdInstallSteps(start) {
		out, err := scope.systemctl(step.args...).CombinedOutput()
		if err != nil && !step.bestEffort {
			return fmt.Errorf("systemctl %s: %s: %w", step.args[0], strings.TrimSpace(string(out)), err)
		}
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
