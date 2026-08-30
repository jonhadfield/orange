package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/htmltext"
)

// searchMode is what a query is matched against.
type searchMode int

const (
	searchStories searchMode = iota
	searchComments
)

func (m searchMode) String() string {
	if m == searchComments {
		return "comments"
	}
	return "stories"
}

type searchResultsMsg struct {
	query    string
	mode     searchMode
	items    []hn.Item
	comments []hn.CommentResult
	err      error
}

// searchModel is the story search: a query line and the stories that matched
// it, ranked by relevance rather than by date.
type searchModel struct {
	client  *hn.Client
	keys    keyMap
	spinner spinner.Model
	input   textinput.Model

	// query is what produced the results below, which is not what the
	// input holds while it is being edited.
	query    string
	mode     searchMode
	results  []hn.Item
	comments []hn.CommentResult
	cursor   int
	typing   bool
	loading  bool
	err      error
	width    int
	height   int
}

func newSearchModel(client *hn.Client, keys keyMap) searchModel {
	in := textinput.New()
	in.Placeholder = "search stories, e.g. sqlite"
	in.Prompt = "/ "
	in.CharLimit = 100
	return searchModel{
		client:  client,
		keys:    keys,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
		input:   in,
	}
}

func (m *searchModel) setSize(w, h int) {
	m.width, m.height = w, h
	m.input.SetWidth(max(10, w-4))
}

// capturing reports whether the query line has the keyboard, which it does
// until the reader runs the search or leaves it.
func (m searchModel) capturing() bool { return m.typing }

// begin opens the search with the query line focused, keeping whatever was
// searched for last so it can be edited rather than retyped.
func (m searchModel) begin() (searchModel, tea.Cmd) {
	m.typing = true
	m.input.SetValue(m.query)
	m.input.CursorEnd()
	m.input.Focus()
	return m, textinput.Blink
}

func (m searchModel) visibleRows() int {
	return max(1, (m.height-tabBarHeight+1)/rowHeight)
}

// count is how many rows the current mode has to show.
func (m searchModel) count() int {
	if m.mode == searchComments {
		return len(m.comments)
	}
	return len(m.results)
}

// selected is the story the cursor is on. A comment result stands in for
// the story it was written under: opening a comment on its own would show a
// reply with nothing around it, so what is offered is its thread.
func (m searchModel) selected() (hn.Item, bool) {
	if m.mode == searchComments {
		if m.cursor < len(m.comments) {
			c := m.comments[m.cursor]
			return hn.Item{ID: c.StoryID, Type: "story", Title: c.StoryTitle}, true
		}
		return hn.Item{}, false
	}
	if m.cursor < len(m.results) {
		return m.results[m.cursor], true
	}
	return hn.Item{}, false
}

// openTarget is the story id that has to be fetched before opening, for a
// result that carries only an id and a title rather than a whole story. A
// story result is already complete, so it reports false and is opened
// directly.
func (m searchModel) openTarget() (int, bool) {
	if m.mode != searchComments {
		return 0, false
	}
	it, ok := m.selected()
	return it.ID, ok
}

// run searches for what is in the query line.
func (m searchModel) run() (searchModel, tea.Cmd) {
	query := strings.TrimSpace(m.input.Value())
	m.typing = false
	m.input.Blur()
	if query == "" {
		return m, nil
	}
	m.query, m.loading, m.err = query, true, nil
	m.results, m.comments, m.cursor = nil, nil, 0
	client, mode := m.client, m.mode
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		if mode == searchComments {
			comments, err := client.SearchComments(ctx, query)
			return searchResultsMsg{query: query, mode: mode, comments: comments, err: err}
		}
		items, err := client.Search(ctx, query)
		return searchResultsMsg{query: query, mode: mode, items: items, err: err}
	})
}

func (m searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case searchResultsMsg:
		// A slower earlier search must not overwrite a newer one, and
		// switching mode makes the answer in flight the wrong kind.
		if msg.query != m.query || msg.mode != m.mode {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.results, m.comments, m.cursor = msg.items, msg.comments, 0
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			if m.cursor < m.count()-1 {
				m.cursor++
			}
		case tea.MouseWheelUp:
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m searchModel) handleKey(msg tea.KeyPressMsg) (searchModel, tea.Cmd) {
	// While the query line is being typed it owns the keyboard, so that
	// letters reach it rather than the list underneath.
	if m.typing {
		switch msg.String() {
		case "enter":
			return m.run()
		case "esc":
			m.typing = false
			m.input.Blur()
			m.input.SetValue(m.query)
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.NextFeed), key.Matches(msg, m.keys.PrevFeed):
		// tab changes what the query is matched against, as it changes
		// which feed is shown on the story list.
		if m.mode == searchComments {
			m.mode = searchStories
		} else {
			m.mode = searchComments
		}
		m.results, m.comments, m.cursor = nil, nil, 0
		if m.query != "" {
			m.input.SetValue(m.query)
			return m.run()
		}
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		return m.begin()
	case key.Matches(msg, m.keys.Down):
		if m.cursor < m.count()-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		if m.count() > 0 {
			m.cursor = m.count() - 1
		}
	case key.Matches(msg, m.keys.ScrollDown):
		m.cursor = scrollCursor(m.cursor, m.count(), m.visibleRows(), 1)
	case key.Matches(msg, m.keys.ScrollUp):
		m.cursor = scrollCursor(m.cursor, m.count(), m.visibleRows(), -1)
	case key.Matches(msg, m.keys.Refresh):
		if !m.loading && m.query != "" {
			m.input.SetValue(m.query)
			return m.run()
		}
	}
	return m, nil
}

func (m searchModel) View() string {
	var b strings.Builder

	left := styleLogo.Render("HN") + styleTabActive.Render("Search")
	flex := ""
	switch {
	case m.typing:
		flex = styleMeta.Render("type a query, enter searches")
	case m.query == "":
		flex = styleMeta.Render("press / to search")
	case m.loading:
		flex = styleMeta.Render(m.spinner.View() + " searching…")
	default:
		flex = styleHeaderTitle.Render(m.query) + "  " +
			styleMeta.Render(fmt.Sprintf("%d in %s · tab switches",
				m.count(), m.mode))
	}
	b.WriteString(barWithFlex(left, flex, m.width, viewSearch))
	b.WriteString("\n")

	if m.typing {
		b.WriteString(m.input.View() + "\n")
	} else {
		b.WriteString("\n")
	}

	switch {
	case m.err != nil:
		b.WriteString(styleError.Render("✗ search failed"))
		b.WriteString("\n" + styleMeta.Render(m.err.Error()))
		b.WriteString("\n\n" + styleMeta.Render("check your connection, then press r to try again"))
	case m.loading:
		b.WriteString(styleMeta.Render(m.spinner.View() + " searching…"))
	case m.query == "":
		b.WriteString(styleMeta.Render("nothing searched for yet — press / and type"))
	case m.count() == 0:
		b.WriteString(styleMeta.Render(fmt.Sprintf(
			"no %s matched %s — tab searches the other", m.mode, m.query)))
	default:
		if m.mode == searchComments {
			b.WriteString(m.commentRows())
		} else {
			b.WriteString(m.rows())
		}
	}
	return b.String()
}

// commentRows renders matched comments. A comment has no title, so the text
// itself is the line to read, with the story it belongs to underneath —
// that is what opening the row gives you.
func (m searchModel) commentRows() string {
	visible := m.visibleRows()
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	indent := strings.Repeat(" ", cursorMarkWidth)

	now := time.Now()
	var b strings.Builder
	for i := start; i < min(start+visible, len(m.comments)); i++ {
		c := m.comments[i]
		sel := i == m.cursor

		cur := strings.Repeat(" ", cursorMarkWidth)
		if sel {
			cur = styleCursorBar.Render(cursorMark)
		}
		titleStyle := styleTitle
		if sel {
			titleStyle = styleTitleSel
		}
		// The comment is HTML and may run to paragraphs; one line of it is
		// enough to recognise, and the thread has the rest.
		text := strings.Join(strings.Fields(htmltext.Convert(c.Text)), " ")
		if text == "" {
			text = "(no text)"
		}
		meta := fmt.Sprintf("by %s · %s · in %s", c.Author, relAge(c.Time, now), c.StoryTitle)

		b.WriteString(ansi.Truncate(cur+titleStyle.Render(text), m.width, "…") + "\n")
		b.WriteString(ansi.Truncate(indent+styleMeta.Render(meta), m.width, "…") + "\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m searchModel) rows() string {
	visible := m.visibleRows()
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	indent := strings.Repeat(" ", cursorMarkWidth+badgeWidth+1)

	now := time.Now()
	var b strings.Builder
	for i := start; i < min(start+visible, len(m.results)); i++ {
		it := m.results[i]
		sel := i == m.cursor

		cur := strings.Repeat(" ", cursorMarkWidth)
		if sel {
			cur = styleCursorBar.Render(cursorMark)
		}
		titleStyle := styleTitle
		if sel {
			titleStyle = styleTitleSel
		}
		line1 := cur + stylePoints.Render(fmt.Sprintf("▲ %-4d", it.Score)) + " " +
			titleStyle.Render(it.Title)
		if d := domain(it.URL); d != "" {
			line1 += " " + styledLink(styleLink, it.URL, "("+d+")")
			line1 += "\x1b]8;;\x1b\\"
		}
		meta := fmt.Sprintf("by %s · %s · %s", it.By, relAge(it.Time, now),
			pluralize(it.Descendants, "comment"))
		b.WriteString(ansi.Truncate(line1, m.width, "…") + "\n")
		b.WriteString(ansi.Truncate(indent+styleMeta.Render(meta), m.width, "…") + "\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
