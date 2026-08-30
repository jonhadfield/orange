package ui

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// viewKeys is what the help bubble renders for the active view: both the
// short bar and the "?" overlay list only the keys that do something here,
// worded for the context, so nothing is advertised that the view ignores.
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

// hint is a help entry for something that is not a binding of its own, such
// as the digit keys, which each view interprets differently.
func hint(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}

// maxHelpColumns is as wide as the "?" overlay is ever laid out.
const maxHelpColumns = 4

// columns packs the view's keys into the widest layout that fits, which is
// also the shortest. The help bubble drops whole columns rather than
// reflowing when they do not fit, and on a narrow terminal the column it
// drops is the one with "esc" and "q" in it, so the count is chosen here.
func columns(all []key.Binding, width int) [][]key.Binding {
	for c := maxHelpColumns; c > 1; c-- {
		if columnsWidth(split(all, c)) <= width {
			return split(all, c)
		}
	}
	return split(all, 1)
}

func rowsFor(items, cols int) int { return (items + cols - 1) / cols }

// split deals the bindings into cols columns, filling each column top to
// bottom, which is the order the help bubble renders them in.
func split(all []key.Binding, cols int) [][]key.Binding {
	rows := rowsFor(len(all), cols)
	out := make([][]key.Binding, 0, cols)
	for i := 0; i < len(all); i += rows {
		out = append(out, all[i:min(i+rows, len(all))])
	}
	return out
}

// columnsWidth is what the help bubble will need to render this layout: each
// column is its widest key plus its widest description, with a gap between
// columns.
func columnsWidth(cols [][]key.Binding) int {
	const gap = 4
	total := 0
	for i, col := range cols {
		keyW, descW := 0, 0
		for _, b := range col {
			keyW = max(keyW, lipgloss.Width(b.Help().Key))
			descW = max(descW, lipgloss.Width(b.Help().Desc))
		}
		total += keyW + 1 + descW
		if i > 0 {
			total += gap
		}
	}
	return total
}

// helpKeys builds the contextual key map for the active view.
func (m Model) helpKeys() viewKeys {
	short, full := m.viewBindings()
	return viewKeys{short: short, full: columns(full, m.width)}
}

// viewBindings is the active view's keys: the handful worth a permanent bar
// along the bottom, and the complete list for the "?" overlay, in the order
// it should be read.
func (m Model) viewBindings() (short, full []key.Binding) {
	k := m.keys
	move := with(k.Up, "j/k", "move")
	scroll := with(k.ScrollDown, "ctrl+d/u", "half page")
	jump := with(k.Top, "g/G", "top/bottom")
	back := with(k.Back, "esc", "back")

	switch m.view {
	case viewStory:
		short = []key.Binding{move, with(k.Open, "enter/l", "fold")}
		if p := m.pastHint(); p != nil {
			short = append(short, *p)
		}
		short = append(short,
			with(k.OpenURL, "o", "open"),
			with(k.Watch, "w", "watch"),
			back, k.Help, k.Quit)

		full = []key.Binding{move, jump, scroll, with(k.Open, "enter/l", "fold/unfold")}
		if p := m.pastHint(); p != nil {
			full = append(full, *p)
		}
		full = append(full,
			with(k.OpenURL, "o", "open link"),
			with(k.OpenHN, "c", "open HN page"),
			with(k.Watch, "w", "watch/unwatch"),
			with(k.Refresh, "r", "reload thread"),
			k.Pulse, k.Hiring, k.Watched, k.Back, k.Help, k.Quit)
		return short, full

	case viewPulse:
		short = []key.Binding{
			move,
			with(k.Open, "enter/l", "open"),
			with(k.OpenURL, "o", "link"),
			with(k.Refresh, "r", "refresh now"),
			back, k.Help, k.Quit,
		}
		full = []key.Binding{
			move, jump, scroll, with(k.Open, "enter/l", "open story"),
			with(k.OpenURL, "o", "open link"),
			with(k.OpenHN, "c", "open HN page"),
			with(k.Watch, "w", "watch/unwatch"),
			with(k.Refresh, "r", "refresh now"),
			k.Hiring, k.Watched, k.Back, k.Help, k.Quit,
		}
		return short, full

	case viewWatched:
		short = []key.Binding{
			move,
			with(k.Open, "enter/l", "open"),
			with(k.Watch, "w", "unwatch"),
			k.Refresh, back, k.Help, k.Quit,
		}
		full = []key.Binding{
			move, jump, scroll, with(k.Open, "enter/l", "open story"),
			with(k.OpenURL, "o", "open link"),
			with(k.OpenHN, "c", "open HN page"),
			with(k.Watch, "w", "unwatch"),
			k.Refresh,
			k.Pulse, k.Hiring, k.Back, k.Help, k.Quit,
		}
		return short, full

	case viewHiring:
		short = []key.Binding{
			move,
			with(k.Open, "enter/l", "expand"),
			with(k.Filter, "/", "filter"),
			with(k.OpenURL, "o", "open post"),
			back, k.Help, k.Quit,
		}
		if m.hiring.capturing() {
			// The filter input has the keyboard, so nothing else on this
			// list is reachable until it gives it back.
			short = []key.Binding{
				hint("type", "narrow the posts"),
				hint("enter", "keep the filter"),
				hint("esc", "done"),
			}
		}
		full = []key.Binding{
			move, jump, scroll, with(k.Open, "enter/l", "expand/collapse"),
			with(k.Filter, "/", "filter posts"),
			hint("enter", "apply filter"),
			with(k.OpenURL, "o", "open post on HN"),
			with(k.Refresh, "r", "reload thread"),
			k.Pulse, k.Watched, k.Back, k.Help, k.Quit,
		}
		return short, full

	case viewSearch:
		if m.search.capturing() {
			// The query line has the keyboard until it is run or left.
			short = []key.Binding{
				hint("type", "a query"),
				hint("enter", "search"),
				hint("esc", "cancel"),
			}
			return short, short
		}
		short = []key.Binding{
			move,
			with(k.Open, "enter/l", "open"),
			with(k.Filter, "/", "new search"),
			with(k.Watch, "w", "watch"),
			back, k.Help, k.Quit,
		}
		full = []key.Binding{
			move, jump, scroll, with(k.Open, "enter/l", "open story"),
			with(k.Filter, "/", "new search"),
			with(k.OpenURL, "o", "open link"),
			with(k.OpenHN, "c", "open HN page"),
			with(k.Watch, "w", "watch/unwatch"),
			with(k.Refresh, "r", "search again"),
			k.Pulse, k.Hiring, k.Watched, back, k.Help, k.Quit,
		}
		return short, full

	default: // the story list
		short = []key.Binding{
			move,
			with(k.Open, "enter/l", "open"),
			with(k.NextFeed, "tab/1-6", "feed"),
			with(k.Filter, "/", "search"),
			with(k.Watch, "w", "watch"),
			k.Help, k.Quit,
		}
		full = []key.Binding{
			move, jump, scroll, with(k.Open, "enter/l", "open story"),
			with(k.NextFeed, "tab", "next feed"),
			with(k.PrevFeed, "shift+tab", "previous feed"),
			hint("1-6", "jump to feed"),
			with(k.Filter, "/", "search stories"),
			k.Refresh,
			with(k.OpenURL, "o", "open link"),
			with(k.OpenHN, "c", "open HN page"),
			with(k.Watch, "w", "watch/unwatch"),
			k.Pulse, k.Hiring, k.Watched, k.Help, k.Quit,
		}
		return short, full
	}
}

// pastHint is the "1-n past discussions" entry, present only while the open
// story actually has earlier submissions to jump to.
func (m Model) pastHint() *key.Binding {
	switch n := len(m.story.past); {
	case n == 1:
		b := hint("1", "past discussion")
		return &b
	case n > 1:
		b := hint("1", "past discussions")
		b.SetHelp("1-"+strconv.Itoa(n), "past discussions")
		return &b
	}
	return nil
}
