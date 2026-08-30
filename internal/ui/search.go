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
)

type searchResultsMsg struct {
	query string
	items []hn.Item
	err   error
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
	query   string
	results []hn.Item
	cursor  int
	typing  bool
	loading bool
	err     error
	width   int
	height  int
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

func (m searchModel) selected() (hn.Item, bool) {
	if m.cursor < len(m.results) {
		return m.results[m.cursor], true
	}
	return hn.Item{}, false
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
	m.results, m.cursor = nil, 0
	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := client.Search(ctx, query)
		return searchResultsMsg{query: query, items: items, err: err}
	})
}

func (m searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case searchResultsMsg:
		// A slower earlier search must not overwrite a newer one.
		if msg.query != m.query {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.results, m.cursor = msg.items, 0
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			if m.cursor < len(m.results)-1 {
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
	case key.Matches(msg, m.keys.Filter):
		return m.begin()
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.results)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		if len(m.results) > 0 {
			m.cursor = len(m.results) - 1
		}
	case key.Matches(msg, m.keys.ScrollDown):
		m.cursor = scrollCursor(m.cursor, len(m.results), m.visibleRows(), 1)
	case key.Matches(msg, m.keys.ScrollUp):
		m.cursor = scrollCursor(m.cursor, len(m.results), m.visibleRows(), -1)
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
			styleMeta.Render(pluralize(len(m.results), "result"))
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
	case len(m.results) == 0:
		b.WriteString(styleMeta.Render("nothing matched " + m.query))
	default:
		b.WriteString(m.rows())
	}
	return b.String()
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
