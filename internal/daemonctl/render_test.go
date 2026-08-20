package daemonctl

import (
	"bytes"
	"strings"
	"testing"
)

// The service files are the one artifact nobody looks at until a machine
// reboots and nik does not come back. Since the split they invoke nikd
// directly, and the `daemon` subcommand they used to pass no longer exists —
// a template that still passed it would install a service that fails on every
// start with "unknown command".

func TestSystemdUnitInvokesNikd(t *testing.T) {
	var buf bytes.Buffer
	err := systemdUnitTmpl.Execute(&buf, struct {
		NikdBinary string
		NikHome    string
	}{NikdBinary: "/usr/local/bin/nikd", NikHome: "/home/fam/.nik"})
	if err != nil {
		t.Fatalf("render unit: %v", err)
	}

	got := buf.String()
	want := "ExecStart=/usr/local/bin/nikd --home /home/fam/.nik"
	if !strings.Contains(got, want) {
		t.Fatalf("unit does not contain %q:\n%s", want, got)
	}
	if strings.Contains(got, "nikd daemon") {
		t.Fatalf("unit still passes the retired `daemon` subcommand:\n%s", got)
	}
}

func TestLaunchdPlistInvokesNikd(t *testing.T) {
	var buf bytes.Buffer
	err := launchdPlistTmpl.Execute(&buf, struct {
		Label      string
		NikdBinary string
		NikHome    string
	}{Label: launchdLabel, NikdBinary: "/usr/local/bin/nikd", NikHome: "/Users/fam/.nik"})
	if err != nil {
		t.Fatalf("render plist: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"<string>/usr/local/bin/nikd</string>",
		"<string>--home</string>",
		"<string>/Users/fam/.nik</string>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<string>daemon</string>") {
		t.Fatalf("plist still passes the retired `daemon` subcommand:\n%s", got)
	}
}
