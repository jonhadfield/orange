package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/htmltext"
)

const hiringBatchSize = 30

type hiringPost struct {
	item       hn.Item
	text       string
	textLinked string // text with OSC 8 hyperlinks, for display
	textLow    string
	headline   string
}

type hiringThreadMsg struct {
	thread  hn.Item
	threads []hn.HiringThread
	kind    int
	err     error
}

type hiringPostsMsg struct {
	threadID int
	items    []hn.Item
	err      error
}

// hiringCheckMsg is the soft re-entry probe: when H is pressed and a thread
// is already loaded, ask Algolia for the latest monthly threads before
// throwing away what is on screen.
type hiringCheckMsg struct {
	threads []hn.HiringThread
	err     error
}

// hiringModel browses the monthly whoishiring threads (hiring, seeking,
// freelance) with keyword filtering over the posts.
type hiringModel struct {
	client  *hn.Client
	keys    keyMap
	spinner spinner.Model
	vp      viewport.Model
	input   textinput.Model

	threads   []hn.HiringThread
	kind      int // index into threads
	thread    hn.Item
	queue     []int
	posts     []hiringPost
	visible   []int // indexes into posts after filtering
	expanded  map[int]bool
	cursor    int
	lineOf    []int
	filtering bool
	loading   bool
	warn      string
	err       error
}

func newHiringModel(client *hn.Client, keys keyMap) hiringModel {
	in := textinput.New()
	in.Placeholder = "filter, e.g. remote golang"
	in.Prompt = "/ "
	in.CharLimit = 80
	return hiringModel{
		client:   client,
		keys:     keys,
		spinner:  spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
		vp:       viewport.New(),
		input:    in,
		expanded: map[int]bool{},
	}
}

func (m *hiringModel) setSize(w, h int) {
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(1, h-3))
	m.input.SetWidth(max(10, w-4))
	m.renderContent()
}

// capturing reports whether keystrokes belong to the filter input.
func (m hiringModel) capturing() bool { return m.filtering }

func (m hiringModel) start() (hiringModel, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	if m.thread.ID != 0 {
		// Keep the cached thread unless a newer monthly post has appeared.
		client := m.client
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			defer cancel()
			threads, err := client.LatestHiringThreads(ctx)
			return hiringCheckMsg{threads: threads, err: err}
		}
	}
	return m.beginLoad()
}

func (m hiringModel) beginLoad() (hiringModel, tea.Cmd) {
	m.loading = true
	m.warn = ""
	m.err = nil
	client, threads, kind := m.client, m.threads, m.kind
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		if len(threads) == 0 {
			var err error
			threads, err = client.LatestHiringThreads(ctx)
			if err != nil {
				return hiringThreadMsg{err: err}
			}
		}
		if kind >= len(threads) {
			kind = 0
		}
		th, err := client.Item(ctx, threads[kind].ID)
		return hiringThreadMsg{thread: th, threads: threads, kind: kind, err: err}
	})
}

// reset clears the loaded posts so the next beginLoad fetches afresh.
// The kind selection and thread catalogue are kept.
func (m *hiringModel) reset() {
	m.thread = hn.Item{}
	m.posts = nil
	m.visible = nil
	m.queue = nil
	m.expanded = map[int]bool{}
	m.cursor = 0
	m.lineOf = m.lineOf[:0]
	m.warn = ""
	m.err = nil
}

func (m hiringModel) selected() (hn.Item, bool) {
	if m.cursor < len(m.visible) {
		return m.posts[m.visible[m.cursor]].item, true
	}
	return hn.Item{}, false
}

func (m *hiringModel) nextBatch() tea.Cmd {
	if len(m.queue) == 0 {
		m.loading = false
		return nil
	}
	n := min(hiringBatchSize, len(m.queue))
	ids := append([]int(nil), m.queue[:n]...)
	m.queue = m.queue[n:]
	m.loading = true
	client, threadID := m.client, m.thread.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := client.Items(ctx, ids)
		return hiringPostsMsg{threadID: threadID, items: items, err: err}
	}
}

func (m hiringModel) Update(msg tea.Msg) (hiringModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case hiringCheckMsg:
		if msg.err != nil || len(msg.threads) == 0 {
			// Soft probe failed: keep what is on screen.
			return m, nil
		}
		m.threads = msg.threads
		if m.kind >= len(m.threads) {
			m.kind = 0
		}
		if m.threads[m.kind].ID == m.thread.ID {
			return m, nil
		}
		(&m).reset()
		return m.beginLoad()

	case hiringThreadMsg:
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			return m, nil
		}
		if len(msg.threads) > 0 {
			m.threads = msg.threads
			m.kind = msg.kind
		}
		m.thread = msg.thread
		m.queue = append([]int(nil), msg.thread.Kids...)
		return m, (&m).nextBatch()

	case hiringPostsMsg:
		if msg.threadID != m.thread.ID {
			return m, nil
		}
		m.loading = false
		m.warn = ""
		had := len(m.posts) > 0
		for _, it := range msg.items {
			if it.Deleted || it.Dead || it.Type != "comment" || strings.TrimSpace(it.Text) == "" {
				continue
			}
			text := htmltext.Convert(it.Text)
			headline := text
			if i := strings.IndexByte(headline, '\n'); i >= 0 {
				headline = headline[:i]
			}
			m.posts = append(m.posts, hiringPost{
				item:       it,
				text:       text,
				textLinked: htmltext.ConvertLinked(it.Text),
				textLow:    strings.ToLower(text),
				headline:   headline,
			})
		}
		m.warn, m.err = applyFetchErr(msg.err, len(msg.items) > 0, had, "posts")
		if m.err != nil {
			return m, nil
		}
		(&m).recompute()
		(&m).renderContent()
		return m, (&m).nextBatch()

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			m.vp.SetYOffset(m.vp.YOffset() + wheelLines)
			(&m).selectTopPost()
		case tea.MouseWheelUp:
			m.vp.SetYOffset(max(0, m.vp.YOffset()-wheelLines))
			(&m).selectTopPost()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m hiringModel) handleKey(msg tea.KeyPressMsg) (hiringModel, tea.Cmd) {
	if m.filtering {
		// v2 replaces the KeyMsg.Type enum with a key code on the press.
		switch msg.Code {
		case tea.KeyEnter, tea.KeyEscape:
			m.filtering = false
			m.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			(&m).recompute()
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.Down):
		// As in the story view: after free scrolling, carry on from the
		// post that is on screen rather than snapping back.
		switch {
		case m.cursorOffScreen():
			if next := m.postFrom(m.vp.YOffset()); next >= 0 {
				m.cursor = next
				(&m).renderContent()
				(&m).ensureCursorVisible()
			} else {
				m.cursor = len(m.visible) - 1
				(&m).renderContent()
				m.vp.ScrollDown(1)
			}
		case m.cursor < len(m.visible)-1:
			m.cursor++
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.Up):
		switch {
		case m.cursorOffScreen():
			if prev := m.postUpTo(m.vp.YOffset() + m.vp.Height() - 1); prev >= 0 {
				m.cursor = prev
				(&m).renderContent()
				(&m).ensureCursorVisible()
			} else {
				m.cursor = 0
				(&m).renderContent()
				m.vp.SetYOffset(0)
			}
		case m.cursor > 0:
			m.cursor--
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		(&m).renderContent()
		m.vp.SetYOffset(0)
	case key.Matches(msg, m.keys.Bottom):
		if len(m.visible) > 0 {
			m.cursor = len(m.visible) - 1
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.Open):
		if it, ok := m.selected(); ok {
			m.expanded[it.ID] = !m.expanded[it.ID]
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.ScrollDown):
		// The selection follows the scroll, as in the story view.
		m.vp.SetYOffset(m.vp.YOffset() + max(1, m.vp.Height()/2))
		(&m).selectTopPost()
	case key.Matches(msg, m.keys.ScrollUp):
		m.vp.SetYOffset(max(0, m.vp.YOffset()-max(1, m.vp.Height()/2)))
		(&m).selectTopPost()
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
		m.input.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.NextFeed):
		if !m.loading && len(m.threads) > 1 {
			m.kind = (m.kind + 1) % len(m.threads)
			(&m).reset()
			return m.beginLoad()
		}
	case key.Matches(msg, m.keys.PrevFeed):
		if !m.loading && len(m.threads) > 1 {
			m.kind = (m.kind + len(m.threads) - 1) % len(m.threads)
			(&m).reset()
			return m.beginLoad()
		}
	case key.Matches(msg, m.keys.Refresh):
		if !m.loading {
			m.threads = nil
			(&m).reset()
			return m.beginLoad() // keeps the current filter text
		}
	}
	return m, nil
}

// recompute rebuilds the visible list from the current filter. Every
// space-separated term must appear somewhere in the post.
func (m *hiringModel) recompute() {
	terms := strings.Fields(strings.ToLower(m.input.Value()))
	m.visible = m.visible[:0]
	for i, p := range m.posts {
		match := true
		for _, t := range terms {
			if !strings.Contains(p.textLow, t) {
				match = false
				break
			}
		}
		if match {
			m.visible = append(m.visible, i)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

// cursorOffScreen reports whether the selected post has been scrolled out of
// view, which is what free scrolling does.
func (m *hiringModel) cursorOffScreen() bool {
	return cursorLineOffScreen(m.lineOf, m.cursor, m.vp.YOffset(), m.vp.Height())
}

// postFrom returns the first post starting at or below the given content
// line, or -1 when every post starts above it.
func (m *hiringModel) postFrom(line int) int {
	return indexFrom(m.lineOf, line)
}

// postInView returns the first post whose header is on screen, or -1 when
// none is.
func (m *hiringModel) postInView() int {
	return indexInView(m.lineOf, m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
}

// selectTopPost highlights the post at the top of the viewport, leaving the
// viewport where the reader scrolled it.
func (m *hiringModel) selectTopPost() {
	i := m.postInView()
	if i < 0 {
		i = m.postUpTo(m.vp.YOffset())
	}
	if i < 0 {
		i = 0
	}
	m.cursor = i
	m.renderContent()
}

// postUpTo is postFrom from the other end, for moving up.
func (m *hiringModel) postUpTo(line int) int {
	return indexUpTo(m.lineOf, line)
}

func (m *hiringModel) ensureCursorVisible() {
	if m.cursor >= len(m.lineOf) {
		return
	}
	target := m.lineOf[m.cursor]
	switch {
	case target < m.vp.YOffset():
		m.vp.SetYOffset(target)
	case target > m.vp.YOffset()+m.vp.Height()-3:
		m.vp.SetYOffset(target - m.vp.Height() + 3)
	}
}

func (m *hiringModel) renderContent() {
	if m.vp.Width() <= 0 {
		return
	}
	m.lineOf = m.lineOf[:0]
	now := time.Now()
	var b strings.Builder
	line := 0
	for vi, pi := range m.visible {
		m.lineOf = append(m.lineOf, line)
		p := m.posts[pi]
		sel := vi == m.cursor

		marker := "▸"
		if m.expanded[p.item.ID] {
			marker = "▾"
		}
		cur := "  "
		if sel {
			cur = styleCursorBar.Render("▍ ")
		}
		titleStyle := styleTitle
		if sel {
			titleStyle = styleTitleSel
		}
		head := cur + titleStyle.Render(marker+" "+p.headline)
		b.WriteString(ansi.Truncate(head, m.vp.Width(), "…") + "\n")
		line++

		if m.expanded[p.item.ID] {
			body := lipgloss.NewStyle().Width(max(20, m.vp.Width()-4)).Render(p.textLinked)
			meta := styleMeta.Render(fmt.Sprintf("— %s · %s · o opens on HN", p.item.By, relAge(p.item.Time, now)))
			block := prefixLines(body+"\n"+meta, "    ")
			b.WriteString(block + "\n")
			line += strings.Count(block, "\n") + 1
		}
		b.WriteString("\n")
		line++
	}
	m.vp.SetContent(b.String())
}

func (m hiringModel) View() string {
	title := "Who is hiring?"
	if m.thread.Title != "" {
		title = m.thread.Title
	}
	counts := ""
	switch {
	case m.err != nil:
		counts = ""
	case len(m.posts) > 0 && len(m.visible) != len(m.posts):
		counts = fmt.Sprintf("%d of %s match", len(m.visible), pluralize(len(m.posts), "post"))
	case len(m.posts) > 0:
		counts = pluralize(len(m.posts), "post")
	}
	if m.loading {
		counts = strings.TrimSpace(counts + " " + m.spinner.View() + " loading…")
	}
	left := styleLogo.Render("HN") + styleTabActive.Render("Hiring")
	if kinds := m.kindTabs(); kinds != "" {
		left += " " + kinds
	}
	flex := styleHeaderTitle.Render(title)
	if counts != "" {
		flex += "  " + styleMeta.Render(counts)
	}
	if m.warn != "" && m.err == nil {
		flex += "  " + styleError.Render("⚠ "+m.warn)
	}

	var body string
	switch {
	case m.err != nil:
		body = styleError.Render("✗ hiring thread unavailable: " + m.err.Error())
		body += "\n\n" + styleMeta.Render("check your connection, then press r to try again")
	case len(m.posts) == 0:
		body = styleMeta.Render(m.spinner.View() + " finding the latest hiring thread…")
	case len(m.visible) == 0:
		body = styleMeta.Render("no posts match — edit the filter with /")
	default:
		body = m.vp.View()
	}

	// The keys themselves are in the help bar below; this line carries only
	// what the help bar cannot say — the filter currently in force.
	footer := ""
	if m.filtering {
		footer = m.input.View()
	} else if q := strings.TrimSpace(m.input.Value()); q != "" {
		footer = styleCollapsed.Render("filter: "+q) + styleMeta.Render("  (/ to edit)")
	}

	return barWithFlex(left, flex, m.vp.Width(), viewHiring) + "\n\n" + body + "\n" + footer
}

// kindTabs renders the hiring/seeking/freelance switcher, with the active
// kind highlighted the way the feed tabs highlight the current feed.
func (m hiringModel) kindTabs() string {
	if len(m.threads) < 2 {
		return ""
	}
	parts := make([]string, 0, len(m.threads))
	for i, th := range m.threads {
		label := th.Label
		if i == m.kind {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTab.Render(label))
		}
	}
	return strings.Join(parts, " ")
}
