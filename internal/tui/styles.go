package tui

import (
	"fmt"
	"image/color"
	"math"
	"os"

	"charm.land/lipgloss/v2"
)

// theme bundles every color decision the TUI makes. Two base hues only:
// `main` (rose) for accents/foreground, `secondary` (grey) for muted
// chrome. `dim` and `sep` stay numbered greys so plain text doesn't ride
// the blend. Light/dark only differ in greyscale shades and blend target.
type theme struct {
	dim         color.Color
	sep         color.Color
	idleOpacity float64
	blendTarget [3]float64
	main        [3]float64
	secondary   [3]float64
}

var (
	rose = [3]float64{244, 168, 179}
	grey = [3]float64{158, 158, 158}

	darkTheme = theme{
		dim:         lipgloss.Color("242"),
		sep:         lipgloss.Color("245"),
		idleOpacity: 0.55,
		blendTarget: [3]float64{0, 0, 0},
		main:        rose,
		secondary:   grey,
	}
	lightTheme = theme{
		dim:         lipgloss.Color("245"),
		sep:         lipgloss.Color("240"),
		idleOpacity: 0.35,
		blendTarget: [3]float64{255, 255, 255},
		main:        rose,
		secondary:   grey,
	}
	th theme
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

func spinnerGlyph(tick int) string {
	return spinnerFrames[(tick/4)%len(spinnerFrames)]
}

var (
	appStyle   lipgloss.Style
	titleStyle lipgloss.Style

	labelStyle   lipgloss.Style
	successStyle lipgloss.Style
	errorStyle   lipgloss.Style

	nikLabel             lipgloss.Style
	youLabel             lipgloss.Style
	nikBorder            lipgloss.Style
	youBorder            lipgloss.Style
	systemBorder         lipgloss.Style
	msgText              lipgloss.Style
	dimText              lipgloss.Style
	sepText              lipgloss.Style
	promptStyle          lipgloss.Style
	dimStyle             lipgloss.Style
	hintStyle            lipgloss.Style
	toolRailStyle        lipgloss.Style
	toolNameStyle        lipgloss.Style
	toolDimStyle         lipgloss.Style
	checkStyle           lipgloss.Style
	errorIndicatorStyle  lipgloss.Style
	spinnerColor         lipgloss.Style
	headerBrandStyle     lipgloss.Style
	headerMetaStyle      lipgloss.Style
	headerModelStyle     lipgloss.Style
	headerPathStyle      lipgloss.Style
	headerDividerStyle   lipgloss.Style
	statusOKStyle        lipgloss.Style
	statusOKLabelStyle   lipgloss.Style
	statusDownStyle      lipgloss.Style
	thinkingStyle        lipgloss.Style
	thinkingSpinnerStyle lipgloss.Style
)

const morphPaletteSize = 20

var morphPalette [morphPaletteSize]string

func init() {
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		th = darkTheme
	} else {
		th = lightTheme
	}

	appStyle = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().Foreground(mainAt(1.0)).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(mainAt(1.0))
	errorStyle = lipgloss.NewStyle().Foreground(mainAt(1.0))

	nikLabel = lipgloss.NewStyle().Foreground(mainAt(1.0)).Bold(true).Italic(true)
	youLabel = lipgloss.NewStyle().Foreground(secondaryAt(1.0)).Bold(true).Italic(true)
	nikBorder = lipgloss.NewStyle().Foreground(mainAt(1.0))
	youBorder = lipgloss.NewStyle().Foreground(secondaryAt(1.0))
	systemBorder = lipgloss.NewStyle().Foreground(th.dim)
	msgText = lipgloss.NewStyle()
	dimText = lipgloss.NewStyle().Foreground(th.dim).Italic(true)
	sepText = lipgloss.NewStyle().Foreground(th.sep)
	promptStyle = lipgloss.NewStyle().Foreground(secondaryAt(1.0)).Bold(true)
	dimStyle = lipgloss.NewStyle().Foreground(th.dim)
	hintStyle = lipgloss.NewStyle().Foreground(th.sep).Italic(true)
	toolRailStyle = lipgloss.NewStyle().Foreground(secondaryAt(0.8))
	toolNameStyle = lipgloss.NewStyle().Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(th.sep)
	checkStyle = lipgloss.NewStyle().Foreground(mainAt(1.0))
	errorIndicatorStyle = lipgloss.NewStyle().Foreground(mainAt(1.0))
	spinnerColor = lipgloss.NewStyle().Foreground(secondaryAt(0.85))
	statusOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	statusOKLabelStyle = lipgloss.NewStyle().Foreground(mainAt(1.0))
	statusDownStyle = lipgloss.NewStyle().Foreground(th.dim).Italic(true)
	thinkingStyle = lipgloss.NewStyle().Italic(true)
	thinkingSpinnerStyle = lipgloss.NewStyle().Foreground(mainAt(0.55))

	headerBrandStyle = lipgloss.NewStyle().Foreground(mainAt(1.0)).Bold(true)
	headerMetaStyle = lipgloss.NewStyle().Foreground(mainAt(0.45))
	headerModelStyle = lipgloss.NewStyle().Foreground(mainAt(0.85))
	headerPathStyle = lipgloss.NewStyle().Foreground(mainAt(0.95))
	headerDividerStyle = lipgloss.NewStyle().Foreground(mainAt(th.idleOpacity))

	for i := range morphPalette {
		t := float64(i) / float64(morphPaletteSize-1)
		c := mainAt(th.idleOpacity + t*(1-th.idleOpacity))
		morphPalette[i] = lipgloss.NewStyle().Foreground(c).Render("─")
	}
}

func mainAt(t float64) color.Color      { return blendRGB(th.main, t) }
func secondaryAt(t float64) color.Color { return blendRGB(th.secondary, t) }

func blendRGB(base [3]float64, t float64) color.Color {
	bg := th.blendTarget
	r := int(bg[0] + (base[0]-bg[0])*t)
	g := int(bg[1] + (base[1]-bg[1])*t)
	b := int(bg[2] + (base[2]-bg[2])*t)
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
