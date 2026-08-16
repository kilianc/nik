package main

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    version
		wantErr bool
	}{
		{name: "plain", in: "v0.2.0", want: version{minor: 2}},
		{name: "trailing newline", in: "v1.4.11\n", want: version{major: 1, minor: 4, patch: 11}},
		{name: "no v prefix", in: "0.2.0", wantErr: true},
		{name: "two components", in: "v0.2", wantErr: true},
		{name: "four components", in: "v0.2.0.1", wantErr: true},
		{name: "leading zero", in: "v0.02.0", wantErr: true},
		{name: "not a number", in: "v0.x.0", wantErr: true},
		{name: "empty component", in: "v0..0", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseVersion(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersion(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBump(t *testing.T) {
	current := version{major: 1, minor: 4, patch: 11}

	tests := []struct {
		name    string
		level   string
		want    string
		wantErr bool
	}{
		{name: "patch", level: "patch", want: "v1.4.12"},
		{name: "minor resets patch", level: "minor", want: "v1.5.0"},
		{name: "major resets the rest", level: "major", want: "v2.0.0"},
		{name: "unknown level", level: "release", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bump(current, tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("bump(%v, %q) = %v, want error", current, tt.level, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("bump(%v, %q): %v", current, tt.level, err)
			}
			if got.String() != tt.want {
				t.Errorf("bump(%v, %q) = %s, want %s", current, tt.level, got, tt.want)
			}
		})
	}
}

func TestAfter(t *testing.T) {
	tests := []struct {
		name string
		v    version
		o    version
		want bool
	}{
		{name: "patch ahead", v: version{minor: 2, patch: 1}, o: version{minor: 2}, want: true},
		{name: "minor ahead of higher patch", v: version{minor: 3}, o: version{minor: 2, patch: 9}, want: true},
		{name: "major ahead of higher minor", v: version{major: 1}, o: version{minor: 9, patch: 9}, want: true},
		{name: "equal", v: version{minor: 2}, o: version{minor: 2}},
		{name: "behind", v: version{minor: 1, patch: 9}, o: version{minor: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.after(tt.o); got != tt.want {
				t.Errorf("%s.after(%s) = %v, want %v", tt.v, tt.o, got, tt.want)
			}
		})
	}
}
