package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Top        key.Binding
	Bottom     key.Binding
	ScrollDown key.Binding
	ScrollUp   key.Binding
	NextFeed   key.Binding
	PrevFeed   key.Binding
	Open       key.Binding
	OpenURL    key.Binding
	OpenHN     key.Binding
	Watch      key.Binding
	Watched    key.Binding
	Pulse      key.Binding
	Hiring     key.Binding
	Refresh    key.Binding
	Filter     key.Binding
	Back       key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		// Up/Top/ScrollDown carry the help text for their pair, since the
		// help lists each pair once; see helpkeys.go.
		Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("j/k", "move")),
		Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/G", "top/bottom")),
		Bottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		ScrollDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("ctrl+d/u", "half page")),
		ScrollUp: key.NewBinding(key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("ctrl+u", "up half page")),
		NextFeed: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/1-6", "switch feed")),
		PrevFeed: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous feed")),
		Open:     key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter/l", "open / fold")),
		OpenURL:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open link")),
		OpenHN:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "open HN page")),
		Watch:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch/unwatch")),
		Watched:  key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "watched stories")),
		Pulse:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pulse")),
		Hiring:   key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "hiring")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Back:     key.NewBinding(key.WithKeys("esc", "h", "b"), key.WithHelp("esc/h/b", "back")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
