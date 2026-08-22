package main

import (
	"strings"
	"testing"
)

// An argument this command did not ask for is the shape a misspelled flag
// takes once flag.Parse is done with the line: `--hom /nik` becomes two
// positionals nobody counts. Reading the name and walking past the rest wrote
// the secret to a different home and said nothing, which is the failure this
// refuses.
func TestSecretsTakesExactlyTheArgumentsItNames(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		action  string
		secret  string
		wantErr string
	}{
		{
			name:   "a name to read",
			args:   []string{"read", "openai_key"},
			action: "read",
			secret: "openai_key",
		},
		{
			name:   "a name to write",
			args:   []string{"write", "openai_key"},
			action: "write",
			secret: "openai_key",
		},
		{
			name:   "a name to delete",
			args:   []string{"delete", "openai_key"},
			action: "delete",
			secret: "openai_key",
		},
		{
			name:   "list names nothing",
			args:   []string{"list"},
			action: "list",
		},
		{
			// `nikctl secrets write openai_key --hom /nik`, after the flags
			// have been taken out of it.
			name:    "a misspelled flag left behind as arguments",
			args:    []string{"write", "openai_key", "--hom", "/nik"},
			wantErr: "usage: nikctl secrets write <name>",
		},
		{
			name:    "a second name to read",
			args:    []string{"read", "openai_key", "exa_api_key"},
			wantErr: "usage: nikctl secrets read <name>",
		},
		{
			name:    "arguments after list",
			args:    []string{"list", "--hom", "/nik"},
			wantErr: "usage: nikctl secrets list",
		},
		{
			name:    "no name at all",
			args:    []string{"write"},
			wantErr: "usage: nikctl secrets write <name>",
		},
		{
			name:    "an action nobody has",
			args:    []string{"rotate", "openai_key"},
			wantErr: `unknown secrets action "rotate"`,
		},
		{
			name:    "nothing",
			args:    nil,
			wantErr: "usage: nikctl secrets {read|list|write|delete}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action, secret, err := secretArgs(tc.args)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("%v was accepted, and %s %s would run against the wrong home",
						tc.args, action, secret)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("the refusal is %q, want it to contain %q", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("%v was refused: %v", tc.args, err)
			}
			if action != tc.action {
				t.Errorf("action is %q, want %q", action, tc.action)
			}
			if secret != tc.secret {
				t.Errorf("name is %q, want %q", secret, tc.secret)
			}
		})
	}
}
