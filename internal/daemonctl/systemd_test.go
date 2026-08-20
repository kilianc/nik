package daemonctl

import (
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of the auto-detection is that one installer works on a
// laptop and inside a nik-saas cell without the caller knowing which. Getting
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
