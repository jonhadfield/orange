package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
)

const (
	storyPageSize = 30
	fetchTimeout  = 30 * time.Second
	rowHeight     = 3 // title + meta + blank separator
	tabBarHeight  = 2 // the tab bar and the blank line under it
	// narrowWidth is where the story rows drop their rank column: below it
	// the fixed gutter costs more than the numbering is worth.
	narrowWidth = 60
	// widths of the fixed columns in front of a story title, which the meta
	// line underneath is indented to match
	rankWidth  = 4 // "999."
	badgeWidth = 6 // "▲ 1234"
)

// cursorMark is the bar drawn beside the selected row in the list views;
// cursorMarkWidth is its width in cells, which is not its length in bytes.
const (
	cursorMark      = "▍ "
	cursorMarkWidth = 2
)

var feedOrder = []hn.Feed{hn.FeedTop, hn.FeedNew, hn.FeedBest, hn.FeedAsk, hn.FeedShow, hn.FeedJobs}

var feedNames = map[hn.Feed]string{
	hn.FeedTop:  "Top",
	hn.FeedNew:  "New",
	hn.FeedBest: "Best",
	hn.FeedAsk:  "Ask",
	hn.FeedShow: "Show",
	hn.FeedJobs: "Jobs",
}

// feedState is the loaded contents and cursor position of one feed, kept
// around so switching between feeds is instant.
type feedState struct {
	ids     []int
	items   []hn.Item
	cursor  int
	loading bool
	err     error
}

type feedIDsMsg struct {
	feed hn.Feed
	ids  []int
	err  error
}

type feedItemsMsg struct {
	feed   hn.Feed
	offset int
	items  []hn.Item
	err    error
}

type feedsModel struct {
	client  *hn.Client
	keys    keyMap
	spinner spinner.Model
	active  int
	states  map[hn.Feed]*feedState
	width   int
	height  int
}

func newFeedsModel(client *hn.Client, keys keyMap) feedsModel {
	states := make(map[hn.Feed]*feedState, len(feedOrder))
	for _, f := range feedOrder {
		states[f] = &feedState{}
	}
	return feedsModel{
		client:  client,
		keys:    keys,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
		states:  states,
	}
}

func (m feedsModel) feed() hn.Feed     { return feedOrder[m.active] }
func (m feedsModel) state() *feedState { return m.states[m.feed()] }
func (m *feedsModel) setSize(w, h int) { m.width, m.height = w, h }

func (m feedsModel) init() tea.Cmd {
	return tea.Batch(m.loadFeed(m.feed()), m.spinner.Tick)
}

// selected returns the story under the cursor, if any.
func (m feedsModel) selected() (hn.Item, bool) {
	st := m.state()
	if st.cursor < len(st.items) {
		return st.items[st.cursor], true
	}
	return hn.Item{}, false
}

func (m feedsModel) loadFeed(feed hn.Feed) tea.Cmd {
	st := m.states[feed]
	if st.loading || len(st.ids) > 0 {
		return nil
	}
	st.loading, st.err = true, nil
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		ids, err := client.FeedIDs(ctx, feed)
		return feedIDsMsg{feed: feed, ids: ids, err: err}
	}
}

func (m feedsModel) loadMore(feed hn.Feed) tea.Cmd {
	st := m.states[feed]
	if st.loading || st.err != nil || len(st.items) >= len(st.ids) {
		return nil
	}
	offset := len(st.items)
	end := min(offset+storyPageSize, len(st.ids))
	ids := append([]int(nil), st.ids[offset:end]...)
	st.loading = true
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		// Always bypass the item cache: pages are usually new IDs, and
		// after a refresh cached scores would be stale.
		items, err := client.ItemsFresh(ctx, ids)
		return feedItemsMsg{feed: feed, offset: offset, items: items, err: err}
	}
}

// visibleRows is how many stories fit in the space the view has. The last
// row's trailing blank line falls off the bottom, hence the +1.
func (m feedsModel) visibleRows() int {
	return max(1, (m.height-tabBarHeight+1)/rowHeight)
}

func (m feedsModel) maybeLoadMore() tea.Cmd {
	st := m.state()
	if st.cursor >= len(st.items)-storyPageSize/3 {
		return m.loadMore(m.feed())
	}
	return nil
}

func (m feedsModel) switchFeed(i int) (feedsModel, tea.Cmd) {
	m.active = i
	st := m.state()
	if st.err != nil {
		// Retry a previously failed feed on revisit.
		*st = feedState{}
	}
	if len(st.ids) == 0 {
		return m, m.loadFeed(m.feed())
	}
	if len(st.items) == 0 {
		return m, m.loadMore(m.feed())
	}
	return m, nil
}

func (m feedsModel) Update(msg tea.Msg) (feedsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case feedIDsMsg:
		st := m.states[msg.feed]
		st.loading = false
		if msg.err != nil {
			st.err = msg.err
			return m, nil
		}
		st.ids = msg.ids
		return m, m.loadMore(msg.feed)

	case feedItemsMsg:
		st := m.states[msg.feed]
		st.loading = false
		if msg.err != nil {
			st.err = msg.err
			return m, nil
		}
		if msg.offset != len(st.items) {
			return m, nil
		}
		st.items = append(st.items, msg.items...)
		return m, nil

	case tea.MouseWheelMsg:
		// A list has no viewport of its own, so the wheel moves the
		// selection and the visible window follows it.
		st := m.state()
		switch msg.Button {
		case tea.MouseWheelDown:
			if st.cursor < len(st.items)-1 {
				st.cursor++
			}
			return m, m.maybeLoadMore()
		case tea.MouseWheelUp:
			if st.cursor > 0 {
				st.cursor--
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m feedsModel) handleKey(msg tea.KeyPressMsg) (feedsModel, tea.Cmd) {
	st := m.state()
	switch {
	case key.Matches(msg, m.keys.Down):
		if st.cursor < len(st.items)-1 {
			st.cursor++
		}
		return m, m.maybeLoadMore()
	case key.Matches(msg, m.keys.Up):
		if st.cursor > 0 {
			st.cursor--
		}
	case key.Matches(msg, m.keys.Top):
		st.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		if len(st.items) > 0 {
			st.cursor = len(st.items) - 1
		}
		return m, m.maybeLoadMore()
	case key.Matches(msg, m.keys.ScrollDown):
		st.cursor = scrollCursor(st.cursor, len(st.items), m.visibleRows(), 1)
		return m, m.maybeLoadMore()
	case key.Matches(msg, m.keys.ScrollUp):
		st.cursor = scrollCursor(st.cursor, len(st.items), m.visibleRows(), -1)
	case key.Matches(msg, m.keys.Refresh):
		if !st.loading {
			*st = feedState{}
			return m, m.loadFeed(m.feed())
		}
	case key.Matches(msg, m.keys.NextFeed):
		return m.switchFeed((m.active + 1) % len(feedOrder))
	case key.Matches(msg, m.keys.PrevFeed):
		return m.switchFeed((m.active + len(feedOrder) - 1) % len(feedOrder))
	default:
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '6' {
			return m.switchFeed(int(s[0] - '1'))
		}
	}
	return m, nil
}

func (m feedsModel) View() string {
	var b strings.Builder
	b.WriteString(m.tabBar())
	b.WriteString("\n\n")

	st := m.state()
	switch {
	case st.err != nil:
		b.WriteString(styleError.Render("✗ could not load " + feedNames[m.feed()] + " stories"))
		b.WriteString("\n" + styleMeta.Render(st.err.Error()))
		// r reloads this feed. tab does eventually retry it too, because
		// switchFeed clears the error on the way back, but that means
		// leaving the feed and returning to it — a longer route to the
		// same place, and r is already on the key bar below.
		b.WriteString("\n\n" + styleMeta.Render("check your connection, then press r to try again"))
	case len(st.items) == 0 && st.loading:
		b.WriteString(styleMeta.Render(m.spinner.View() + " loading " + feedNames[m.feed()] + " stories…"))
	case len(st.items) == 0:
		b.WriteString(styleMeta.Render("no stories here"))
	default:
		b.WriteString(m.rows(st))
	}
	return b.String()
}

func (m feedsModel) tabBar() string {
	logo := styleLogo.Render("HN")
	// The nav hints only get their space if what is left still fits every
	// tab; otherwise the tabs use the whole line and the hints drop out, as
	// they do for the other views.
	budget := m.width - lipgloss.Width(logo)
	if hints := lipgloss.Width(navHints(viewFeeds)); m.tabsWidth() <= budget-hints-2 {
		budget -= hints + 2
	}
	left := lipgloss.JoinHorizontal(lipgloss.Center, append([]string{logo}, m.tabs(budget)...)...)

	flex := ""
	if st := m.state(); st.loading && len(st.items) > 0 {
		flex = styleMeta.Render(m.spinner.View() + " loading more…")
	}
	return barWithFlex(left, flex, m.width, viewFeeds)
}

// tabLabel renders one feed tab, numbered with its hotkey.
func (m feedsModel) tabLabel(i int) string {
	style := styleTab
	if i == m.active {
		style = styleTabActive
	}
	return style.Render(fmt.Sprintf("%d %s", i+1, feedNames[feedOrder[i]]))
}

func (m feedsModel) tabsWidth() int {
	w := 0
	for i := range feedOrder {
		w += lipgloss.Width(m.tabLabel(i))
	}
	return w
}

// tabs returns the feed tabs that fit in budget columns, grown outwards from
// the active one so it is never the tab that falls off the end. A "‹" or "›"
// marks the feeds left out, and is paid for out of the same budget.
func (m feedsModel) tabs(budget int) []string {
	labels := make([]string, len(feedOrder))
	widths := make([]int, len(feedOrder))
	for i := range feedOrder {
		labels[i] = m.tabLabel(i)
		widths[i] = lipgloss.Width(labels[i])
	}
	marker := lipgloss.Width(styleTab.Render("›"))
	cost := func(first, last int) int {
		w := 0
		for i := first; i <= last; i++ {
			w += widths[i]
		}
		if first > 0 {
			w += marker
		}
		if last < len(labels)-1 {
			w += marker
		}
		return w
	}

	first, last := m.active, m.active
	for {
		grew := false
		if next := last + 1; next < len(labels) && cost(first, next) <= budget {
			last, grew = next, true
		}
		if prev := first - 1; prev >= 0 && cost(prev, last) <= budget {
			first, grew = prev, true
		}
		if !grew {
			break
		}
	}
	out := append([]string(nil), labels[first:last+1]...)
	if first > 0 {
		out = append([]string{styleTab.Render("‹")}, out...)
	}
	if last < len(labels)-1 {
		out = append(out, styleTab.Render("›"))
	}
	return out
}

func (m feedsModel) rows(st *feedState) string {
	visible := m.visibleRows()
	start := 0
	if st.cursor >= visible {
		start = st.cursor - visible + 1
	}
	end := min(start+visible, len(st.items))

	// On a narrow terminal the rank column costs more than it is worth, so
	// it goes and the meta line moves back under the title with it.
	showRank := m.width >= narrowWidth
	gutter := cursorMarkWidth + badgeWidth + 1
	if showRank {
		gutter += rankWidth + 1
	}
	indent := strings.Repeat(" ", gutter)

	now := time.Now()
	var b strings.Builder
	for i := start; i < end; i++ {
		it := st.items[i]
		sel := i == st.cursor

		cur := strings.Repeat(" ", cursorMarkWidth)
		if sel {
			cur = styleCursorBar.Render(cursorMark)
		}

		badge := stylePoints.Render(fmt.Sprintf("▲ %-4d", it.Score))
		if it.Type == "job" {
			badge = styleMeta.Render("job   ")
		}

		title := it.Title
		if title == "" {
			title = "(untitled)"
		}
		titleStyle := styleTitle
		if sel {
			titleStyle = styleTitleSel
		}
		line1 := cur
		if showRank {
			line1 += styleMeta.Render(fmt.Sprintf("%3d.", i+1)) + " "
		}
		line1 += badge + " " + titleStyle.Render(title)
		if d := domain(it.URL); d != "" {
			line1 += " " + styledLink(styleLink, it.URL, "("+d+")")
			// Truncation can cut the hyperlink's closing sequence; reset
			// unconditionally so a link never bleeds into later lines.
			line1 = ansi.Truncate(line1, m.width, "…") + linkReset
		}

		meta := relAge(it.Time, now)
		if it.By != "" {
			meta = "by " + it.By + " · " + meta
		}
		if it.Type != "job" {
			meta += " · " + pluralize(it.Descendants, "comment")
		}
		line2 := indent + styleMeta.Render(meta)

		b.WriteString(ansi.Truncate(line1, m.width, "…"))
		b.WriteString("\n")
		b.WriteString(ansi.Truncate(line2, m.width, "…"))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
