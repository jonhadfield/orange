package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/htmltext"
)

const commentBatchSize = 24

type commentsMsg struct {
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
	queue    []int // comment IDs waiting to be fetched
	past     []hn.PastDiscussion
	newSince int64 // comments after this time are marked new (0 = off)
	loading  bool
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
		vp:      viewport.New(0, 0),
	}
}

func (m *storyModel) setSize(w, h int) {
	m.vp.Width = w
	m.vp.Height = max(1, h-1) // reserve one line for the status footer
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
	m.queue = append([]int(nil), it.Kids...)
	m.past = nil
	m.newSince = newSince
	m.cursor = 0
	m.err = nil
	m.vp.SetYOffset(0)
	(&m).renderContent()
	cmd := (&m).nextBatch()
	return m, tea.Batch(cmd, m.fetchPast(), m.spinner.Tick)
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

// nextBatch takes the next slice of queued comment IDs and fetches them.
func (m *storyModel) nextBatch() tea.Cmd {
	if len(m.queue) == 0 {
		m.loading = false
		return nil
	}
	n := min(commentBatchSize, len(m.queue))
	ids := append([]int(nil), m.queue[:n]...)
	m.queue = m.queue[n:]
	m.loading = true
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

	case commentsMsg:
		if m.tree == nil || msg.storyID != m.story.ID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		added := m.tree.add(msg.items)
		for _, it := range added {
			m.queue = append(m.queue, it.Kids...)
		}
		(&m).renderContent()
		(&m).ensureCursorVisible()
		return m, (&m).nextBatch()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m storyModel) handleKey(msg tea.KeyMsg) (storyModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.nodes)-1 {
			m.cursor++
			(&m).renderContent()
			(&m).ensureCursorVisible()
		} else {
			m.vp.ScrollDown(1)
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			(&m).renderContent()
			(&m).ensureCursorVisible()
		} else {
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
		// Free scrolling for comments taller than the screen; the
		// selection stays where it is.
		m.vp.SetYOffset(m.vp.YOffset + max(1, m.vp.Height/2))
	case key.Matches(msg, m.keys.ScrollUp):
		m.vp.SetYOffset(max(0, m.vp.YOffset-max(1, m.vp.Height/2)))
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

func (m *storyModel) ensureCursorVisible() {
	if m.cursor >= len(m.lineOf) {
		return
	}
	target := m.lineOf[m.cursor]
	switch {
	case target < m.vp.YOffset:
		m.vp.SetYOffset(target)
	case target > m.vp.YOffset+m.vp.Height-4:
		m.vp.SetYOffset(target - m.vp.Height + 4)
	}
}

// renderContent rebuilds the viewport content (header plus visible comment
// tree) and records the line offset of every visible node.
func (m *storyModel) renderContent() {
	if m.vp.Width <= 0 || m.tree == nil {
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
	w := m.vp.Width
	var b strings.Builder
	b.WriteString(styleHeaderTitle.Width(w).Render(m.story.Title))
	b.WriteString("\n")
	if m.story.URL != "" {
		b.WriteString(ansi.Truncate(styledLink(styleLink, m.story.URL, m.story.URL), w, "…") + linkReset)
		b.WriteString("\n")
	}
	meta := fmt.Sprintf("▲ %d · by %s · %s · %s",
		m.story.Score, m.story.By, relAge(m.story.Time, now), pluralize(m.story.Descendants, "comment"))
	b.WriteString(stylePoints.Render(meta))
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
	indentWidth := n.depth * 2
	guide := ""
	if n.depth > 0 {
		guide = styleGuide.Render(strings.Repeat("▏ ", n.depth))
	}

	arrow := "▾"
	if n.collapsed {
		arrow = "▸"
	}
	metaStyle := styleMeta
	if selected {
		metaStyle = styleMetaSel
	}
	meta := metaStyle.Render(fmt.Sprintf("%s %s · %s", arrow, n.item.By, relAge(n.item.Time, now)))
	if m.newSince > 0 && n.item.Time > m.newSince {
		meta += styleRise.Render(" ● new")
	}
	if n.collapsed {
		if hidden := subtreeSize(n); hidden > 0 {
			meta += styleCollapsed.Render(fmt.Sprintf("  [+%s]", pluralize(hidden, "reply")))
		}
	}

	block := meta
	if !n.collapsed {
		wrapWidth := max(20, m.vp.Width-indentWidth-1)
		text := htmltext.ConvertLinked(n.item.Text)
		block += "\n" + lipgloss.NewStyle().Width(wrapWidth).Render(text)
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
	}
	return m.vp.View() + "\n" + status
}
