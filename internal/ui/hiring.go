package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	thread hn.Item
	err    error
}

type hiringPostsMsg struct {
	threadID int
	items    []hn.Item
	err      error
}

// hiringModel browses the latest monthly "Ask HN: Who is hiring?" thread
// with keyword filtering over the job posts.
type hiringModel struct {
	client  *hn.Client
	keys    keyMap
	spinner spinner.Model
	vp      viewport.Model
	input   textinput.Model

	thread    hn.Item
	queue     []int
	posts     []hiringPost
	visible   []int // indexes into posts after filtering
	expanded  map[int]bool
	cursor    int
	lineOf    []int
	filtering bool
	loading   bool
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
		vp:       viewport.New(0, 0),
		input:    in,
		expanded: map[int]bool{},
	}
}

func (m *hiringModel) setSize(w, h int) {
	m.vp.Width = w
	m.vp.Height = max(1, h-3)
	m.input.Width = max(10, w-4)
	m.renderContent()
}

// capturing reports whether keystrokes belong to the filter input.
func (m hiringModel) capturing() bool { return m.filtering }

func (m hiringModel) start() (hiringModel, tea.Cmd) {
	if m.thread.ID != 0 || m.loading {
		return m, nil // already loaded this session
	}
	m.loading = true
	m.err = nil
	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		id, err := client.LatestHiringThread(ctx)
		if err != nil {
			return hiringThreadMsg{err: err}
		}
		th, err := client.Item(ctx, id)
		return hiringThreadMsg{thread: th, err: err}
	})
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

	case hiringThreadMsg:
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			return m, nil
		}
		m.thread = msg.thread
		m.queue = append([]int(nil), msg.thread.Kids...)
		return m, (&m).nextBatch()

	case hiringPostsMsg:
		if msg.threadID != m.thread.ID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
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
		(&m).recompute()
		(&m).renderContent()
		return m, (&m).nextBatch()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m hiringModel) handleKey(msg tea.KeyMsg) (hiringModel, tea.Cmd) {
	if m.filtering {
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
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
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			(&m).renderContent()
			(&m).ensureCursorVisible()
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
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
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
		m.input.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Refresh):
		if !m.loading {
			m.thread = hn.Item{}
			m.posts = nil
			m.visible = nil
			m.queue = nil
			m.expanded = map[int]bool{}
			m.cursor = 0
			m.lineOf = m.lineOf[:0]
			return m.start() // keeps the current filter text
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

func (m *hiringModel) ensureCursorVisible() {
	if m.cursor >= len(m.lineOf) {
		return
	}
	target := m.lineOf[m.cursor]
	switch {
	case target < m.vp.YOffset:
		m.vp.SetYOffset(target)
	case target > m.vp.YOffset+m.vp.Height-3:
		m.vp.SetYOffset(target - m.vp.Height + 3)
	}
}

func (m *hiringModel) renderContent() {
	if m.vp.Width <= 0 {
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
		b.WriteString(ansi.Truncate(head, m.vp.Width, "…") + "\n")
		line++

		if m.expanded[p.item.ID] {
			body := lipgloss.NewStyle().Width(max(20, m.vp.Width-4)).Render(p.textLinked)
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
	header := styleLogo.Render("HN") + styleTabActive.Render("Hiring") + " " +
		styleHeaderTitle.Render(title) + "  " + styleMeta.Render(counts)

	var body string
	switch {
	case m.err != nil:
		body = styleError.Render("✗ hiring thread unavailable: " + m.err.Error())
	case len(m.posts) == 0:
		body = styleMeta.Render(m.spinner.View() + " finding the latest hiring thread…")
	case len(m.visible) == 0:
		body = styleMeta.Render("no posts match — edit the filter with /")
	default:
		body = m.vp.View()
	}

	footer := styleMeta.Render("/ filter · enter expand · o open post on HN")
	if m.filtering {
		footer = m.input.View()
	} else if q := strings.TrimSpace(m.input.Value()); q != "" {
		footer = styleCollapsed.Render("filter: "+q) + styleMeta.Render("  (/ to edit)")
	}

	return ansi.Truncate(header, m.vp.Width, "…") + "\n\n" + body + "\n" + footer
}
