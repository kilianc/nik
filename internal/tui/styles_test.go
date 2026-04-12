package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStylesNotNil(t *testing.T) {
	styles := []struct {
		name   string
		render string
	}{
		{"titleStyle", titleStyle.Render("test")},
		{"labelStyle", labelStyle.Render("test")},
		{"successStyle", successStyle.Render("test")},
		{"errorStyle", errorStyle.Render("test")},
		{"promptStyle", promptStyle.Render("test")},
		{"dimStyle", dimStyle.Render("test")},
		{"dimText", dimText.Render("test")},
		{"sepText", sepText.Render("test")},
		{"hintStyle", hintStyle.Render("test")},
		{"nikLabel", nikLabel.Render("test")},
		{"youLabel", youLabel.Render("test")},
		{"nikBorder", nikBorder.Render("test")},
		{"youBorder", youBorder.Render("test")},
		{"toolRailStyle", toolRailStyle.Render("test")},
		{"toolNameStyle", toolNameStyle.Render("test")},
		{"toolDimStyle", toolDimStyle.Render("test")},
		{"checkStyle", checkStyle.Render("test")},
		{"errorIndicatorStyle", errorIndicatorStyle.Render("test")},
		{"spinnerColor", spinnerColor.Render("test")},
	}

	for _, tt := range styles {
		t.Run(tt.name, func(t *testing.T) {
			if tt.render == "" {
				t.Errorf("%s rendered empty string", tt.name)
			}
		})
	}
}

func TestNikAt(t *testing.T) {
	for _, v := range []float64{0.0, 0.5, 1.0} {
		c := nikAt(v)
		s := string(c)
		if !strings.HasPrefix(s, "#") || len(s) != 7 {
			t.Errorf("nikAt(%v) = %q, want #rrggbb hex", v, s)
		}
	}
}

func TestThinkMorphWidth(t *testing.T) {
	for _, w := range []int{40, 80, 120} {
		out := thinkMorph(0, 1.0, w)
		got := lipgloss.Width(out)
		if got != w {
			t.Errorf("thinkMorph width %d: got %d", w, got)
		}
	}
}

func TestMorphPalettePopulated(t *testing.T) {
	for i, s := range morphPalette {
		if s == "" {
			t.Errorf("morphPalette[%d] is empty", i)
		}
		if !strings.Contains(s, "━") {
			t.Errorf("morphPalette[%d] missing dash glyph", i)
		}
	}
}
