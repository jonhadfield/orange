package ui

import (
	"strconv"

	"charm.land/bubbles/v2/key"
)

// viewKeys is what the help bubble renders for the active view: the short
// bar lists only keys that work right now, worded for the context, while
// the full overlay keeps the complete reference.
type viewKeys struct {
	short []key.Binding
	full  [][]key.Binding
}

func (v viewKeys) ShortHelp() []key.Binding  { return v.short }
func (v viewKeys) FullHelp() [][]key.Binding { return v.full }

// with returns the binding with its help text reworded for one context.
func with(b key.Binding, keys, desc string) key.Binding {
	b.SetHelp(keys, desc)
	return b
}

// helpKeys builds the contextual key map for the active view.
func (m Model) helpKeys() viewKeys {
	k := m.keys
	var short []key.Binding
	switch m.view {
	case viewStory:
		short = []key.Binding{
			with(k.Up, "j/k", "move"),
			with(k.ScrollDown, "ctrl+d/u", "scroll"),
			with(k.Open, "enter/l", "fold"),
		}
		if n := len(m.story.past); n == 1 {
			short = append(short, key.NewBinding(key.WithKeys("1"),
				key.WithHelp("1", "past discussion")))
		} else if n > 1 {
			short = append(short, key.NewBinding(key.WithKeys("1"),
				key.WithHelp("1-"+strconv.Itoa(n), "past discussions")))
		}
		short = append(short,
			with(k.OpenURL, "o", "open link"),
			with(k.Watch, "w", "watch"),
			k.Back, k.Refresh, k.Help, k.Quit)
	case viewPulse:
		short = []key.Binding{
			with(k.Up, "j/k", "move"),
			with(k.Open, "enter/l", "open story"),
			with(k.Refresh, "r", "refresh now"),
			k.Back, k.Help, k.Quit,
		}
	case viewWatched:
		short = []key.Binding{
			with(k.Up, "j/k", "move"),
			with(k.Open, "enter/l", "open story"),
			with(k.Watch, "w", "unwatch"),
			k.Refresh, k.Back, k.Help, k.Quit,
		}
	case viewHiring:
		short = []key.Binding{
			with(k.Up, "j/k", "move"),
			with(k.Open, "enter/l", "expand"),
			with(k.Filter, "/", "filter"),
			with(k.ScrollDown, "ctrl+d/u", "scroll"),
			with(k.OpenURL, "o", "open post"),
			k.Back, k.Refresh, k.Help, k.Quit,
		}
	default: // the story list
		short = []key.Binding{
			with(k.Up, "j/k", "move"),
			k.Top,
			with(k.Open, "enter/l", "open story"),
			with(k.NextFeed, "tab/1-6", "switch feed"),
			with(k.Watch, "w", "watch"),
			k.Refresh, k.Help, k.Quit,
		}
	}
	return viewKeys{short: short, full: k.FullHelp()}
}
