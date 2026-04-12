package tui

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	dim         lipgloss.Color
	sep         lipgloss.Color
	idleOpacity float64
	blendTarget [3]float64
}

var (
	darkTheme = theme{
		dim:         lipgloss.Color("242"),
		sep:         lipgloss.Color("245"),
		idleOpacity: 0.55,
		blendTarget: [3]float64{0, 0, 0},
	}
	lightTheme = theme{
		dim:         lipgloss.Color("245"),
		sep:         lipgloss.Color("240"),
		idleOpacity: 0.35,
		blendTarget: [3]float64{255, 255, 255},
	}
	th theme
)

var nikRGB = [3]float64{158, 158, 158}

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

var (
	nikAccent lipgloss.Color
	youAccent lipgloss.Color

	appStyle   = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))

	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	nikLabel            lipgloss.Style
	youLabel            lipgloss.Style
	nikBorder           lipgloss.Style
	youBorder           lipgloss.Style
	msgText             lipgloss.Style
	dimText             lipgloss.Style
	sepText             lipgloss.Style
	promptStyle         lipgloss.Style
	dimStyle            lipgloss.Style
	hintStyle           lipgloss.Style
	toolRailStyle       lipgloss.Style
	toolNameStyle       lipgloss.Style
	toolDimStyle        lipgloss.Style
	checkStyle          lipgloss.Style
	errorIndicatorStyle lipgloss.Style
	spinnerColor        lipgloss.Style
)

const morphPaletteSize = 20

var morphPalette [morphPaletteSize]string

func init() {
	if lipgloss.HasDarkBackground() {
		th = darkTheme
	} else {
		th = lightTheme
	}

	nikAccent = lipgloss.Color("245")
	youAccent = lipgloss.Color("#1982fc")

	nikLabel = lipgloss.NewStyle().Foreground(nikAccent).Bold(true).Italic(true)
	youLabel = lipgloss.NewStyle().Foreground(youAccent).Bold(true).Italic(true)
	nikBorder = lipgloss.NewStyle().Foreground(nikAccent)
	youBorder = lipgloss.NewStyle().Foreground(youAccent)
	msgText = lipgloss.NewStyle()
	dimText = lipgloss.NewStyle().Foreground(th.dim).Italic(true)
	sepText = lipgloss.NewStyle().Foreground(th.sep)
	promptStyle = lipgloss.NewStyle().Foreground(nikAccent).Bold(true)
	dimStyle = lipgloss.NewStyle().Foreground(th.dim)
	hintStyle = lipgloss.NewStyle().Foreground(th.sep).Italic(true)
	toolRailStyle = lipgloss.NewStyle().Foreground(nikAt(0.8))
	toolNameStyle = lipgloss.NewStyle().Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(th.sep)
	checkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4caf50"))
	errorIndicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e53935"))
	spinnerColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9a825"))

	for i := range morphPalette {
		t := float64(i) / float64(morphPaletteSize-1)
		c := nikAt(th.idleOpacity + t*(1-th.idleOpacity))
		morphPalette[i] = lipgloss.NewStyle().Foreground(c).Render("━")
	}
}

func nikAt(t float64) lipgloss.Color {
	bg := th.blendTarget
	r := int(bg[0] + (nikRGB[0]-bg[0])*t)
	g := int(bg[1] + (nikRGB[1]-bg[1])*t)
	b := int(bg[2] + (nikRGB[2]-bg[2])*t)
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

func wavePos(tick int) float64 {
	t := float64(tick)
	return t + 40*math.Sin(2*math.Pi*t/160) + 12*math.Sin(2*math.Pi*t/97) + 7*math.Sin(2*math.Pi*t/53)
}

func waveBrightness(i, tick int, wavelength float64) float64 {
	pos := wavePos(tick)
	phase := 2 * math.Pi * (float64(i) - pos) / wavelength
	return (math.Sin(phase) + 1) / 2
}

func controlAmp(i, tick int) float64 {
	return waveBrightness(i, tick, 40)
}

func breatheAmp(i, tick, width int) float64 {
	ft := float64(tick)
	center := float64(width) / 2
	dist := math.Abs(float64(i) - center)
	t := 0.0
	for ring := 0; ring < 3; ring++ {
		period := 70.0 + float64(ring)*23
		phase := math.Mod(ft/period, 1.0)
		radius := phase * center
		ringDist := math.Abs(dist - radius)
		t += math.Exp(-ringDist*ringDist/6.0) * (0.8 - float64(ring)*0.2)
	}
	breathe := (math.Sin(ft/40) + 1) / 2 * 0.15
	return math.Min(t+breathe, 1.0)
}

func thinkMorph(tick int, energy float64, width int) string {
	ft := float64(tick)
	mix := (math.Sin(ft/197) + math.Sin(ft/311) + math.Sin(ft/127) + 3) / 6
	var b []byte
	for i := 0; i < width; i++ {
		a := controlAmp(i, tick)
		z := breatheAmp(i, tick, width)
		t := (a*(1-mix) + z*mix) * energy
		idx := int(t * float64(morphPaletteSize-1))
		if idx >= morphPaletteSize {
			idx = morphPaletteSize - 1
		}
		b = append(b, morphPalette[idx]...)
	}
	return string(b)
}
