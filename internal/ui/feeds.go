package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
)

const (
	storyPageSize = 30
	fetchTimeout  = 30 * time.Second
	rowHeight     = 3 // title + meta + blank separator
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
		items, err := client.Items(ctx, ids)
		return feedItemsMsg{feed: feed, offset: offset, items: items, err: err}
	}
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

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m feedsModel) handleKey(msg tea.KeyMsg) (feedsModel, tea.Cmd) {
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
		b.WriteString("\n\n" + styleMeta.Render("check your connection, then press tab to retry"))
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
	parts := []string{styleLogo.Render("HN")}
	for i, f := range feedOrder {
		style := styleTab
		if i == m.active {
			style = styleTabActive
		}
		parts = append(parts, style.Render(fmt.Sprintf("%d %s", i+1, feedNames[f])))
	}
	left := lipgloss.JoinHorizontal(lipgloss.Center, parts...)

	hint := func(k, name string) string {
		return styleCollapsed.Render(k) + " " + styleMeta.Render(name)
	}
	right := hint("p", "Pulse") + "  " + hint("H", "Hiring") + "  " + hint("W", "Watched")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 2 {
		return ansi.Truncate(left, m.width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m feedsModel) rows(st *feedState) string {
	visible := max(1, (m.height-2)/rowHeight)
	start := 0
	if st.cursor >= visible {
		start = st.cursor - visible + 1
	}
	end := min(start+visible, len(st.items))

	now := time.Now()
	var b strings.Builder
	for i := start; i < end; i++ {
		it := st.items[i]
		sel := i == st.cursor

		cur := "  "
		if sel {
			cur = styleCursorBar.Render("▍ ")
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
		line1 := fmt.Sprintf("%s%s %s %s", cur, styleMeta.Render(fmt.Sprintf("%3d.", i+1)), badge, titleStyle.Render(title))
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
		line2 := "             " + styleMeta.Render(meta)

		b.WriteString(ansi.Truncate(line1, m.width, "…"))
		b.WriteString("\n")
		b.WriteString(ansi.Truncate(line2, m.width, "…"))
		b.WriteString("\n\n")
	}
	if st.loading {
		b.WriteString(styleMeta.Render(m.spinner.View() + " loading more…"))
	}
	return strings.TrimRight(b.String(), "\n")
}
