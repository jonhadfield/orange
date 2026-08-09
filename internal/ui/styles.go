package ui

import "charm.land/lipgloss/v2"

// Lip Gloss v2 dropped AdaptiveColor: instead of each colour resolving itself
// at render time, the terminal background is reported once and the palette is
// built from it. setTheme rebuilds every style, and the app calls it when
// Bubble Tea reports the background colour.
var (
	styleLogo lipgloss.Style

	styleTab       lipgloss.Style
	styleTabActive lipgloss.Style

	styleTitle    lipgloss.Style
	styleTitleSel lipgloss.Style

	styleMeta    lipgloss.Style
	styleMetaSel lipgloss.Style

	stylePoints    lipgloss.Style
	styleCursorBar lipgloss.Style

	styleGuide     lipgloss.Style
	styleCollapsed lipgloss.Style

	styleHeaderTitle lipgloss.Style
	styleRule        lipgloss.Style

	styleError lipgloss.Style

	styleRise lipgloss.Style
	styleFall lipgloss.Style

	// styleLink marks text that is an OSC 8 hyperlink.
	styleLink lipgloss.Style
)

// Dark is the common case, and it is what the styles show until the terminal
// answers the background-colour query.
func init() { setTheme(true) }

// setTheme rebuilds the palette for a light or dark terminal background.
func setTheme(isDark bool) {
	pick := lipgloss.LightDark(isDark)
	var (
		colorAccent = pick(lipgloss.Color("#d35400"), lipgloss.Color("#ff8036"))
		colorDim    = pick(lipgloss.Color("#8a8a8a"), lipgloss.Color("#767676"))
		colorSubtle = pick(lipgloss.Color("#d0d0d0"), lipgloss.Color("#3a3a3a"))
		colorErrFg  = pick(lipgloss.Color("#870000"), lipgloss.Color("#ff8787"))
		colorRise   = pick(lipgloss.Color("#1a7f37"), lipgloss.Color("#57d364"))
		colorFall   = pick(lipgloss.Color("#a40e26"), lipgloss.Color("#f47067"))
	)

	styleLogo = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#ff6600")).
		Padding(0, 1)

	styleTab = lipgloss.NewStyle().Foreground(colorDim).Padding(0, 1)
	styleTabActive = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true).Padding(0, 1)

	styleTitle = lipgloss.NewStyle()
	styleTitleSel = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	styleMeta = lipgloss.NewStyle().Foreground(colorDim)
	styleMetaSel = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	stylePoints = lipgloss.NewStyle().Foreground(colorAccent)
	styleCursorBar = lipgloss.NewStyle().Foreground(colorAccent)

	styleGuide = lipgloss.NewStyle().Foreground(colorSubtle)
	styleCollapsed = lipgloss.NewStyle().Foreground(colorAccent)

	styleHeaderTitle = lipgloss.NewStyle().Bold(true)
	styleRule = lipgloss.NewStyle().Foreground(colorSubtle)

	styleError = lipgloss.NewStyle().Foreground(colorErrFg).Bold(true)

	styleRise = lipgloss.NewStyle().Foreground(colorRise)
	styleFall = lipgloss.NewStyle().Foreground(colorFall)

	styleLink = lipgloss.NewStyle().Foreground(colorDim).Underline(true)
}
