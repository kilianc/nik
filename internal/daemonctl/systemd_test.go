package daemonctl

import (
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of the auto-detection is that one installer works on a
// laptop and inside a container without the caller knowing which. Getting
// the branch wrong is not a subtle bug in either direction: a user unit in a
// container fails at `daemon-reload` with "Failed to connect to bus: No medium
// found", and a system unit on a laptop is a root service nobody asked for.
func TestSystemdScopeFor(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}

	userUnit := filepath.Join(u.HomeDir, ".config", "systemd", "user", systemdUnitName)

	tests := []struct {
		name             string
		euid             int
		reachable        bool
		wantUser         bool
		wantUnitPath     string
		wantWantedBy     string
		wantSystemctlArg []string
	}{
		{
			name:             "laptop",
			euid:             501,
			reachable:        true,
			wantUser:         true,
			wantUnitPath:     userUnit,
			wantWantedBy:     "default.target",
			wantSystemctlArg: []string{"systemctl", "--user", "daemon-reload"},
		},
		{
			// /etc/systemd/system is not theirs to write, so there is nowhere
			// better to send them: the user-bus error is the honest one.
			name:             "non-root without a user manager",
			euid:             501,
			reachable:        false,
			wantUser:         true,
			wantUnitPath:     userUnit,
			wantWantedBy:     "default.target",
			wantSystemctlArg: []string{"systemctl", "--user", "daemon-reload"},
		},
		{
			name:             "root with a real session",
			euid:             0,
			reachable:        true,
			wantUser:         true,
			wantUnitPath:     userUnit,
			wantWantedBy:     "default.target",
			wantSystemctlArg: []string{"systemctl", "--user", "daemon-reload"},
		},
		{
			name:             "cell",
			euid:             0,
			reachable:        false,
			wantUser:         false,
			wantUnitPath:     "/etc/systemd/system/nikd.service",
			wantWantedBy:     "multi-user.target",
			wantSystemctlArg: []string{"systemctl", "daemon-reload"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope := systemdScopeFor(tt.euid, tt.reachable)
			if scope.user != tt.wantUser {
				t.Errorf("user scope = %v, want %v", scope.user, tt.wantUser)
			}

			path, err := scope.unitPath()
			if err != nil {
				t.Fatalf("unitPath: %v", err)
			}
			if path != tt.wantUnitPath {
				t.Errorf("unitPath = %q, want %q", path, tt.wantUnitPath)
			}

			if scope.wantedBy() != tt.wantWantedBy {
				t.Errorf("wantedBy = %q, want %q", scope.wantedBy(), tt.wantWantedBy)
			}

			got := scope.systemctl("daemon-reload").Args
			if strings.Join(got, " ") != strings.Join(tt.wantSystemctlArg, " ") {
				t.Errorf("systemctl args = %v, want %v", got, tt.wantSystemctlArg)
			}
		})
	}
}

// The install sequence is the whole fix: `enable --now` starts a stopped unit
// and leaves a running one alone, which is why upgrades put new binaries on
// disk and left the old daemon serving from the inode behind them.
func TestSystemdInstallSteps(t *testing.T) {
	tests := []struct {
		name  string
		start bool
		want  [][]string
	}{
		{
			// reset-failed earns its place here: without it an install over a
			// daemon that has spent its start limit is refused with "Start
			// request repeated too quickly", and the fix just installed never
			// runs.
			name:  "install and start",
			start: true,
			want: [][]string{
				{"daemon-reload"},
				{"enable", systemdUnitName},
				{"reset-failed", systemdUnitName},
				{"restart", systemdUnitName},
			},
		},
		{
			// --no-start: the definition lands and nothing about what is
			// running changes, including a daemon that is already failed.
			name:  "install only",
			start: false,
			want: [][]string{
				{"daemon-reload"},
				{"enable", systemdUnitName},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][]string
			for _, step := range systemdInstallSteps(tt.start) {
				got = append(got, step.args)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("steps = %v, want %v", got, tt.want)
			}

			for i := range got {
				if strings.Join(got[i], " ") != strings.Join(tt.want[i], " ") {
					t.Errorf("step %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// reset-failed is the only step allowed to fail — it is a no-op on a unit that
// never failed. Everything else is load-bearing and must surface.
func TestSystemdInstallStepsBestEffort(t *testing.T) {
	for _, step := range systemdInstallSteps(true) {
		wantBestEffort := step.args[0] == "reset-failed"
		if step.bestEffort != wantBestEffort {
			t.Errorf("%v bestEffort = %v, want %v", step.args, step.bestEffort, wantBestEffort)
		}
	}
}
