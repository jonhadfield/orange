package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#d35400", Dark: "#ff8036"}
	colorDim    = lipgloss.AdaptiveColor{Light: "#8a8a8a", Dark: "#767676"}
	colorSubtle = lipgloss.AdaptiveColor{Light: "#d0d0d0", Dark: "#3a3a3a"}
	colorErrFg  = lipgloss.AdaptiveColor{Light: "#870000", Dark: "#ff8787"}
	colorRise   = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#57d364"}
	colorFall   = lipgloss.AdaptiveColor{Light: "#a40e26", Dark: "#f47067"}
)

var (
	styleLogo = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#ff6600")).
			Padding(0, 1)

	styleTab       = lipgloss.NewStyle().Foreground(colorDim).Padding(0, 1)
	styleTabActive = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true).Padding(0, 1)

	styleTitle    = lipgloss.NewStyle()
	styleTitleSel = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	styleMeta    = lipgloss.NewStyle().Foreground(colorDim)
	styleMetaSel = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	stylePoints    = lipgloss.NewStyle().Foreground(colorAccent)
	styleCursorBar = lipgloss.NewStyle().Foreground(colorAccent)

	styleGuide     = lipgloss.NewStyle().Foreground(colorSubtle)
	styleCollapsed = lipgloss.NewStyle().Foreground(colorAccent)

	styleHeaderTitle = lipgloss.NewStyle().Bold(true)
	styleRule        = lipgloss.NewStyle().Foreground(colorSubtle)

	styleError = lipgloss.NewStyle().Foreground(colorErrFg).Bold(true)

	styleRise = lipgloss.NewStyle().Foreground(colorRise)
	styleFall = lipgloss.NewStyle().Foreground(colorFall)

	// styleLink marks text that is an OSC 8 hyperlink.
	styleLink = lipgloss.NewStyle().Foreground(colorDim).Underline(true)
)
