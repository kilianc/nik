package version

import "testing"

func TestString(t *testing.T) {
	number, sha := Number, SHA
	defer func() { Number, SHA = number, sha }()

	tests := []struct {
		name   string
		number string
		sha    string
		want   string
	}{
		{"unstamped", "dev", "dev", "dev"},
		{"released", "v0.2.0", "a8e2c2f", "v0.2.0 (a8e2c2f)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Number, SHA = tt.number, tt.sha
			if got := String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
