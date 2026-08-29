package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/htmltext"
)

// wheelLines is how far one wheel or trackpad notch scrolls a text view.
const wheelLines = 3

// minCommentWidth is the narrowest column a comment body is allowed, and so
// what the thread indentation has to leave room for.
const minCommentWidth = 32

// maxGuideDepth is how many levels of reply indentation a terminal this wide
// can show while still leaving a comment room to wrap.
func maxGuideDepth(width int) int {
	return max(1, (width-minCommentWidth)/2)
}

const (
	commentBatchSize = 24
	// Batches run concurrently so the Firebase reconciliation pass finishes
	// quickly in the background, rather than one round trip at a time.
	maxInFlightBatches = 3
)

type commentsMsg struct {
	storyID int
	items   []hn.Item
	err     error
}

// treeMsg carries the whole comment tree fetched from Algolia in one request.
type treeMsg struct {
	storyID int
	items   []hn.Item
	err     error
}

type pastMsg struct {
	storyID int
	list    []hn.PastDiscussion
}

// openItemMsg asks the app to load and open another story by ID.
type openItemMsg struct{ id int }

type storyModel struct {
	client  *hn.Client
	keys    keyMap
	spinner spinner.Model
	vp      viewport.Model

	story    hn.Item
	tree     *commentTree
	queue    []int        // comment IDs waiting to be fetched
	queued   map[int]bool // every ID ever queued, so nothing is fetched twice
	inflight int          // batches currently in flight
	past     []hn.PastDiscussion
	newSince int64 // comments after this time are marked new (0 = off)
	loading  bool
	warn     string // non-fatal loading problem, shown alongside the count
	err      error

	nodes  []*commentNode // currently visible nodes, in render order
	lineOf []int          // content line each visible node starts on
	cursor int
}

func newStoryModel(client *hn.Client, keys keyMap) storyModel {
	return storyModel{
		client:  client,
		keys:    keys,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
		vp:      viewport.New(),
	}
}

func (m *storyModel) setSize(w, h int) {
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(1, h-2)) // reserve a line each for the top bar and the status
	if m.tree != nil {
		m.renderContent()
		m.ensureCursorVisible()
	}
}

// open resets the model for a newly selected story and starts loading its
// comment thread and any past discussions of the same URL. Comments newer
// than newSince are highlighted (0 disables highlighting).
func (m storyModel) open(it hn.Item, newSince int64) (storyModel, tea.Cmd) {
	m.story = it
	m.tree = newCommentTree(it.ID)
	m.queue = nil
	m.queued = make(map[int]bool)
	m.inflight = 0
	m.past = nil
	m.newSince = newSince
	m.cursor = 0
	m.warn = ""
	m.err = nil
	m.loading = true
	m.vp.SetYOffset(0)
	(&m).renderContent()
	return m, tea.Batch((&m).fetchTree(), m.fetchPast(), m.spinner.Tick)
}

// fetchTree pulls the entire comment tree from Algolia in a single request.
// It is a fast path, not a complete one — Algolia drops removed comments and
// their replies and trails the live site — so the Firebase pass still runs
// behind it to fill the gaps.
func (m *storyModel) fetchTree() tea.Cmd {
	client, id := m.client, m.story.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := client.ItemTree(ctx, id)
		return treeMsg{storyID: id, items: items, err: err}
	}
}

// fetchPast looks up earlier submissions of the same URL. Failures are
// silent: past discussions are an extra, never an error state.
func (m *storyModel) fetchPast() tea.Cmd {
	if m.story.URL == "" {
		return nil
	}
	client, storyURL, id := m.client, m.story.URL, m.story.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		list, err := client.PastDiscussions(ctx, storyURL, id)
		if err != nil {
			return pastMsg{storyID: id}
		}
		return pastMsg{storyID: id, list: list}
	}
}

// enqueue adds comment IDs to the fetch queue, skipping any already queued.
func (m *storyModel) enqueue(ids []int) {
	for _, id := range ids {
		if m.queued[id] {
			continue
		}
		m.queued[id] = true
		m.queue = append(m.queue, id)
	}
}

// fillBatches starts fetches until the in-flight limit is reached, and clears
// the loading flag once nothing is left to do.
func (m *storyModel) fillBatches() tea.Cmd {
	var cmds []tea.Cmd
	for m.inflight < maxInFlightBatches {
		cmd := m.nextBatch()
		if cmd == nil {
			break
		}
		cmds = append(cmds, cmd)
	}
	m.loading = m.inflight > 0
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// nextBatch takes the next slice of queued comment IDs and fetches them.
func (m *storyModel) nextBatch() tea.Cmd {
	if len(m.queue) == 0 {
		return nil
	}
	n := min(commentBatchSize, len(m.queue))
	ids := append([]int(nil), m.queue[:n]...)
	m.queue = m.queue[n:]
	m.loading = true
	m.inflight++
	client, storyID := m.client, m.story.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := client.Items(ctx, ids)
		return commentsMsg{storyID: storyID, items: items, err: err}
	}
}

func (m storyModel) Update(msg tea.Msg) (storyModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case pastMsg:
		if msg.storyID != m.story.ID || len(msg.list) == 0 {
			return m, nil
		}
		m.past = msg.list
		(&m).renderContent()
		return m, nil

	case treeMsg:
		if m.tree == nil || msg.storyID != m.story.ID {
			return m, nil
		}
		if msg.err == nil {
			m.tree.add(msg.items)
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
		// Reconcile against Firebase either way: it recovers the comments
		// Algolia omits, and is the only source if the fast path failed.
		(&m).enqueue(m.story.Kids)
		return m, (&m).fillBatches()

	case commentsMsg:
		if m.tree == nil || msg.storyID != m.story.ID {
			return m, nil
		}
		m.inflight--
		if msg.err != nil {
			var partial *hn.PartialError
			if errors.As(msg.err, &partial) {
				m.warn = fmt.Sprintf("%d of %d comments failed to load",
					partial.Requested-partial.Fetched, partial.Requested)
			} else {
				// Surface the failure but keep whatever else is still
				// arriving, rather than discarding the whole thread.
				m.err = msg.err
			}
		}
		m.tree.add(msg.items)
		// Walk from every fetched comment, not just the newly added ones:
		// a comment already supplied by Algolia can still have replies that
		// Algolia dropped.
		for _, it := range msg.items {
			(&m).enqueue(it.Kids)
		}
		(&m).renderContent()
		(&m).ensureCursorVisible()
		return m, (&m).fillBatches()

	case tea.MouseWheelMsg:
		// The wheel scrolls the view and the selection follows it, exactly
		// as ctrl+d and ctrl+u do.
		switch msg.Button {
		case tea.MouseWheelDown:
			m.vp.SetYOffset(m.vp.YOffset() + wheelLines)
			(&m).selectTopComment()
		case tea.MouseWheelUp:
			m.vp.SetYOffset(max(0, m.vp.YOffset()-wheelLines))
			(&m).selectTopComment()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m storyModel) handleKey(msg tea.KeyPressMsg) (storyModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		switch {
		case m.cursorOffScreen():
			// Free scrolling moved the view and left the selection behind,
			// so continue from what is on screen instead of snapping back.
			if next := m.nodeFrom(m.vp.YOffset()); next >= 0 {
				m.cursor = next
				(&m).renderContent()
				(&m).ensureCursorVisible()
			} else {
				// Scrolled past every comment: take the last one without
				// dragging the view back to it, and keep scrolling.
				m.cursor = len(m.nodes) - 1
				(&m).renderContent()
				m.vp.ScrollDown(1)
			}
		case m.cursor < len(m.nodes)-1:
			m.cursor++
			(&m).renderContent()
			(&m).ensureCursorVisible()
		default:
			m.vp.ScrollDown(1)
		}
	case key.Matches(msg, m.keys.Up):
		switch {
		case m.cursorOffScreen():
			if prev := m.nodeUpTo(m.vp.YOffset() + m.vp.Height() - 1); prev >= 0 {
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
		default:
			m.vp.SetYOffset(0) // reveal the story header
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		(&m).renderContent()
		m.vp.SetYOffset(0)
	case key.Matches(msg, m.keys.Bottom):
		if len(m.nodes) > 0 {
			m.cursor = len(m.nodes) - 1
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.ScrollDown):
		// Free scrolling for comments taller than the screen. The
		// selection follows, so the comment being read stays highlighted.
		m.vp.SetYOffset(m.vp.YOffset() + max(1, m.vp.Height()/2))
		(&m).selectTopComment()
	case key.Matches(msg, m.keys.ScrollUp):
		m.vp.SetYOffset(max(0, m.vp.YOffset()-max(1, m.vp.Height()/2)))
		(&m).selectTopComment()
	case key.Matches(msg, m.keys.Refresh):
		// Reload via the app so the story item itself is refetched.
		if id := m.story.ID; id != 0 && !m.loading {
			return m, func() tea.Msg { return openItemMsg{id: id} }
		}
	case key.Matches(msg, m.keys.Open):
		if m.cursor < len(m.nodes) {
			n := m.nodes[m.cursor]
			n.collapsed = !n.collapsed
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	default:
		// Digits open the corresponding past discussion.
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(m.past) {
				id := m.past[i].ID
				return m, func() tea.Msg { return openItemMsg{id: id} }
			}
		}
	}
	return m, nil
}

// cursorOffScreen reports whether the selected comment has been scrolled out
// of view. That is exactly what free scrolling does: it moves the viewport
// and deliberately leaves the selection where it was.
func (m *storyModel) cursorOffScreen() bool {
	if m.cursor >= len(m.lineOf) {
		return false
	}
	line := m.lineOf[m.cursor]
	return line < m.vp.YOffset() || line >= m.vp.YOffset()+m.vp.Height()
}

// nodeFrom returns the first comment starting at or below the given content
// line, or -1 when every comment starts above it. Reading down, that is the
// next comment the reader can see, whether it is already on screen or just
// below the fold of a long comment they are part-way through.
func (m *storyModel) nodeFrom(line int) int {
	for i, l := range m.lineOf {
		if l >= line {
			return i
		}
	}
	return -1
}

// nodeInView returns the first comment whose header is on screen, or -1 when
// none is — which happens inside a comment taller than the whole viewport.
func (m *storyModel) nodeInView() int {
	top, bottom := m.vp.YOffset(), m.vp.YOffset()+m.vp.Height()
	for i, l := range m.lineOf {
		if l >= top {
			if l < bottom {
				return i
			}
			break // sorted, so nothing later is in view either
		}
	}
	return -1
}

// selectTopComment highlights the comment at the top of the viewport: the
// first whose header is on screen, or, when the reader is part-way through
// one taller than the screen, the comment they are inside. The viewport is
// left exactly where the reader scrolled it.
func (m *storyModel) selectTopComment() {
	i := m.nodeInView()
	if i < 0 {
		i = m.nodeUpTo(m.vp.YOffset())
	}
	if i < 0 {
		i = 0
	}
	m.cursor = i
	m.renderContent()
}

// nodeUpTo is nodeFrom from the other end: the last comment starting at or
// above the given line, or -1 when every comment starts below it.
func (m *storyModel) nodeUpTo(line int) int {
	for i := len(m.lineOf) - 1; i >= 0; i-- {
		if m.lineOf[i] <= line {
			return i
		}
	}
	return -1
}

func (m *storyModel) ensureCursorVisible() {
	if m.cursor >= len(m.lineOf) {
		return
	}
	target := m.lineOf[m.cursor]
	switch {
	case target < m.vp.YOffset():
		m.vp.SetYOffset(target)
	case target > m.vp.YOffset()+m.vp.Height()-4:
		m.vp.SetYOffset(target - m.vp.Height() + 4)
	}
}

// renderContent rebuilds the viewport content (header plus visible comment
// tree) and records the line offset of every visible node.
func (m *storyModel) renderContent() {
	if m.vp.Width() <= 0 || m.tree == nil {
		return
	}
	m.nodes = m.tree.visible()
	m.lineOf = m.lineOf[:0]

	now := time.Now()
	var b strings.Builder
	line := 0
	write := func(s string) {
		b.WriteString(s)
		line += strings.Count(s, "\n")
	}

	write(m.header(now))

	for i, n := range m.nodes {
		m.lineOf = append(m.lineOf, line)
		write(m.renderNode(n, i == m.cursor, now))
	}
	if len(m.nodes) == 0 && len(m.queue) == 0 && !m.loading {
		write(styleMeta.Render("no comments yet") + "\n")
	}

	m.vp.SetContent(b.String())
}

func (m *storyModel) header(now time.Time) string {
	w := m.vp.Width()
	var b strings.Builder
	b.WriteString(styleHeaderTitle.Width(w).Render(m.story.Title))
	b.WriteString("\n")
	if m.story.URL != "" {
		b.WriteString(ansi.Truncate(styledLink(styleLink, m.story.URL, m.story.URL), w, "…") + linkReset)
		b.WriteString("\n")
	}
	meta := fmt.Sprintf("▲ %d · by %s · %s · %s",
		m.story.Score, m.story.By, relAge(m.story.Time, now), pluralize(m.story.Descendants, "comment"))
	b.WriteString(stylePoints.Render(ansi.Truncate(meta, w, "…")))
	b.WriteString("\n")
	if len(m.past) > 0 {
		b.WriteString("\n")
		b.WriteString(styleMeta.Render("previously on HN:"))
		b.WriteString("\n")
		for i, p := range m.past {
			row := styleCollapsed.Render(fmt.Sprintf("  [%d]", i+1)) +
				styleMeta.Render(fmt.Sprintf(" %s · ▲ %d · %s",
					pluralize(p.Comments, "comment"), p.Points, relAge(p.Time, now)))
			b.WriteString(hyperlink(hnItemURL(p.ID), row))
			b.WriteString("\n")
		}
	}
	if m.story.Text != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Width(w).Render(htmltext.ConvertLinked(m.story.Text)))
		b.WriteString("\n")
	}
	b.WriteString(styleRule.Render(strings.Repeat("─", max(1, w))))
	b.WriteString("\n\n")
	return b.String()
}

func (m *storyModel) renderNode(n *commentNode, selected bool, now time.Time) string {
	// Guides stop at the depth the terminal can afford: past that the reply
	// would have nowhere left to wrap. A "⋯" in place of the first guide
	// says the comment sits deeper than the indentation shows.
	depth := min(n.depth, maxGuideDepth(m.vp.Width()))
	indentWidth := depth * 2
	guide := ""
	switch {
	case depth < n.depth:
		guide = styleGuide.Render("⋯ " + strings.Repeat("▏ ", depth-1))
	case depth > 0:
		guide = styleGuide.Render(strings.Repeat("▏ ", depth))
	}

	arrow := "▾"
	if n.collapsed {
		arrow = "▸"
	}
	metaStyle := styleMeta
	if selected {
		metaStyle = styleMetaSel
	}

	var meta string
	if n.placeholder {
		// Kept only to hold its replies, so it shows a marker and no body.
		label := "[dead]"
		if n.item.Deleted {
			label = "[deleted]"
		}
		meta = metaStyle.Render(arrow + " " + label)
	} else {
		meta = metaStyle.Render(fmt.Sprintf("%s %s · %s", arrow, n.item.By, relAge(n.item.Time, now)))
		if m.newSince > 0 && n.item.Time > m.newSince {
			meta += styleRise.Render(" ● new")
		}
	}
	if n.collapsed {
		if hidden := subtreeSize(n); hidden > 0 {
			meta += styleCollapsed.Render(fmt.Sprintf("  [+%s]", pluralize(hidden, "reply")))
		}
	}

	block := meta
	if !n.collapsed && !n.placeholder {
		// The indentation is already capped, so what is left is a column
		// worth reading — except on a terminal narrower than that cap, where
		// the terminal wins.
		wrapWidth := max(1, m.vp.Width()-indentWidth-1)
		block += "\n" + lipgloss.NewStyle().Width(wrapWidth).Render(n.text)
	}

	return prefixLines(block, guide) + "\n\n"
}

// prefixLines prepends prefix to every line of s.
func prefixLines(s, prefix string) string {
	if prefix == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// bar is the header line: the same shape as the other views, keeping the
// story identified and the p/H/W destinations offered once the header
// inside the thread has scrolled away.
func (m storyModel) bar() string {
	left := styleLogo.Render("HN") + styleTabActive.Render("Story")
	return barWithFlex(left, styleMeta.Render(m.story.Title), m.vp.Width(), viewStory)
}

func (m storyModel) View() string {
	var status string
	switch {
	case m.err != nil:
		status = styleError.Render("✗ comments failed to load: " + m.err.Error())
	case m.loading || len(m.queue) > 0:
		status = styleMeta.Render(fmt.Sprintf("%s loading comments… %d/%d",
			m.spinner.View(), m.tree.count, m.story.Descendants))
	default:
		count := 0
		if m.tree != nil {
			count = m.tree.count
		}
		status = styleMeta.Render(pluralize(count, "comment") + " loaded")
		if m.warn != "" {
			status += styleError.Render("  ⚠ " + m.warn)
		}
	}
	return m.bar() + "\n" + m.vp.View() + "\n" + status
}
