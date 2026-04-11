package tui

import "github.com/charmbracelet/lipgloss"

var (
	nikAccent = lipgloss.Color("99")
	youAccent = lipgloss.Color("44")

	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(nikAccent)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	promptStyle = lipgloss.NewStyle().
			Foreground(nikAccent).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	chatNikName = lipgloss.NewStyle().
			Foreground(nikAccent).
			Bold(true)

	chatYouName = lipgloss.NewStyle().
			Foreground(youAccent).
			Bold(true)

	chatSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(nikAccent)
)
