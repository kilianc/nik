package main

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"
)

// The bug this file exists for: `nikctl secrets write openai_key --home /nik`
// read no --home at all, because Go's flag package stops at `write`. It wrote
// to whatever NIK_HOME happened to say — on a capsule, the right directory by
// luck — and `nikctl config set a.b c --home /nik` in the same script counted
// four positionals where it wanted two and exited on a usage message.
//
// So: a flag this command defines is read wherever it is written, and nothing
// that is not one of its flags moves.
func TestFlagsAreReadWhereverTheyAreWritten(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		home   string
		errors bool
		want   []string
	}{
		{
			name: "in front, the way they have always worked",
			args: []string{"--home", "/nik", "write", "openai_key"},
			home: "/nik",
			want: []string{"write", "openai_key"},
		},
		{
			name: "at the end, which is how a line gets edited",
			args: []string{"write", "openai_key", "--home", "/nik"},
			home: "/nik",
			want: []string{"write", "openai_key"},
		},
		{
			name: "in the middle, between the action and its arguments",
			args: []string{"set", "--home", "/nik", "models.main.backend", "api"},
			home: "/nik",
			want: []string{"set", "models.main.backend", "api"},
		},
		{
			name: "joined by an equals sign",
			args: []string{"write", "openai_key", "--home=/nik"},
			home: "/nik",
			want: []string{"write", "openai_key"},
		},
		{
			name: "spelled with one dash",
			args: []string{"write", "openai_key", "-home", "/nik"},
			home: "/nik",
			want: []string{"write", "openai_key"},
		},
		{
			// The token after a bool is an argument, not its value.
			name:   "a bool flag takes nothing with it",
			args:   []string{"logs", "--errors", "20"},
			errors: true,
			want:   []string{"logs", "20"},
		},
		{
			// `nikctl shell ls -la`. -la is not a flag of this command, and
			// after a positional it never was — it is what the caller is
			// running.
			name: "an undefined flag after an argument is an argument",
			args: []string{"ls", "-la"},
			want: []string{"ls", "-la"},
		},
		{
			name: "-- still means the rest is verbatim",
			args: []string{"query", "--", "--home", "/nik"},
			want: []string{"query", "--home", "/nik"},
		},
		{
			// Whatever follows a flag that takes a value is that value.
			name: "a value that looks like a flag",
			args: []string{"write", "openai_key", "--home", "--weird"},
			home: "--weird",
			want: []string{"write", "openai_key"},
		},
		{
			name: "a lone dash is an argument",
			args: []string{"write", "-"},
			want: []string{"write", "-"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.String("home", "", "workspace directory")
			fs.Bool("errors", false, "warnings and above")

			parseFlags(fs, tc.args)

			if got := fs.Lookup("home").Value.String(); got != tc.home {
				t.Errorf("--home is %q, want %q", got, tc.home)
			}
			if got := fs.Lookup("errors").Value.String(); got != boolText(tc.errors) {
				t.Errorf("--errors is %s, want %v", got, tc.errors)
			}
			if got := strings.Join(fs.Args(), " "); got != strings.Join(tc.want, " ") {
				t.Errorf("arguments are %q, want %q", got, strings.Join(tc.want, " "))
			}
		})
	}

	// And a flag nobody defined, written where a flag belongs, is still a
	// typo. Hoisting must not turn `--hom /nik` into two arguments that some
	// command counts and another ignores.
	var out bytes.Buffer
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&out)
	fs.String("home", "", "workspace directory")

	parseFlags(fs, []string{"--hom", "/nik", "write", "openai_key"})

	if !strings.Contains(out.String(), "not defined") {
		t.Errorf("--hom was accepted rather than refused:\n%s", out.String())
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
