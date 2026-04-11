package tui

import "testing"

func TestStylesNotNil(t *testing.T) {
	styles := []struct {
		name   string
		render string
	}{
		{"titleStyle", titleStyle.Render("test")},
		{"labelStyle", labelStyle.Render("test")},
		{"successStyle", successStyle.Render("test")},
		{"errorStyle", errorStyle.Render("test")},
		{"timestampStyle", timestampStyle.Render("test")},
		{"promptStyle", promptStyle.Render("test")},
		{"dimStyle", dimStyle.Render("test")},
		{"hintStyle", hintStyle.Render("test")},
		{"chatNikName", chatNikName.Render("test")},
		{"chatYouName", chatYouName.Render("test")},
		{"chatSepStyle", chatSepStyle.Render("test")},
		{"spinnerStyle", spinnerStyle.Render("test")},
	}

	for _, tt := range styles {
		t.Run(tt.name, func(t *testing.T) {
			if tt.render == "" {
				t.Errorf("%s rendered empty string", tt.name)
			}
		})
	}
}
