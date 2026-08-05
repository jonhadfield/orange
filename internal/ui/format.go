package ui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// relAge formats a unix timestamp as a compact relative age like "3h ago".
func relAge(unix int64, now time.Time) string {
	d := now.Sub(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

// domain extracts a display host ("example.com") from a URL.
func domain(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// linkReset closes any open OSC 8 hyperlink. Emitting it when no link is
// open is a harmless no-op.
const linkReset = "\x1b]8;;\x1b\\"

// hyperlink wraps text in an OSC 8 terminal hyperlink. Terminals without
// hyperlink support ignore the escape sequences and show the text as-is.
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + linkReset
}

// styledLink renders text as an OSC 8 hyperlink with the style's SGR
// sequences around, not inside, the link markers. Two constraints force
// this shape: some terminals (JetBrains) cannot parse links whose anchor
// text contains SGR sequences, and lipgloss mangles content that already
// contains OSC 8, so the style and the link must be composed by hand.
func styledLink(style lipgloss.Style, url, text string) string {
	const marker = "\x00"
	pre, post, _ := strings.Cut(style.Render(marker), marker)
	return pre + hyperlink(url, text) + post
}

// navHints renders the top-right destination-view hints, omitting the view
// the user is already on.
func navHints(current view) string {
	hint := func(k, name string) string {
		return styleCollapsed.Render(k) + " " + styleMeta.Render(name)
	}
	var parts []string
	if current != viewPulse {
		parts = append(parts, hint("p", "Pulse"))
	}
	if current != viewHiring {
		parts = append(parts, hint("H", "Hiring"))
	}
	if current != viewWatched {
		parts = append(parts, hint("W", "Watched"))
	}
	return strings.Join(parts, "  ")
}

// barWithHints lays out a view's header line: left content, then the nav
// hints right-aligned. The hints are dropped when the terminal is too
// narrow to fit both.
func barWithHints(left string, width int, current view) string {
	right := navHints(current)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 2 {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
