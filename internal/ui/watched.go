package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

type watchedRow struct {
	item     hn.Item
	state    store.WatchState
	newCount int
}

type watchedDataMsg struct {
	items []hn.Item
	err   error
}

// watchedModel lists watched stories with the number of comments posted
// since each was last read.
type watchedModel struct {
	client  *hn.Client
	st      *store.Store
	keys    keyMap
	spinner spinner.Model
	rows    []watchedRow
	cursor  int
	loading bool
	err     error
	width   int
	height  int
}

func newWatchedModel(client *hn.Client, st *store.Store, keys keyMap) watchedModel {
	return watchedModel{
		client:  client,
		st:      st,
		keys:    keys,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
	}
}

func (m *watchedModel) setSize(w, h int) { m.width, m.height = w, h }

func (m watchedModel) start() (watchedModel, tea.Cmd) {
	m.err = nil
	m.rows = nil
	if m.st == nil {
		m.err = fmt.Errorf("watch list unavailable (state file could not be opened)")
		return m, nil
	}
	states := m.st.All()
	if len(states) == 0 {
		m.loading = false
		return m, nil
	}
	ids := make([]int, len(states))
	for i, ws := range states {
		ids[i] = ws.ID
	}
	m.loading = true
	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := client.ItemsFresh(ctx, ids)
		return watchedDataMsg{items: items, err: err}
	})
}

func (m watchedModel) selected() (watchedRow, bool) {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor], true
	}
	return watchedRow{}, false
}

func (m watchedModel) Update(msg tea.Msg) (watchedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case watchedDataMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rows = m.rows[:0]
		for _, it := range msg.items {
			ws, ok := m.st.Get(it.ID)
			if !ok {
				continue
			}
			m.rows = append(m.rows, watchedRow{
				item:     it,
				state:    ws,
				newCount: max(0, it.Descendants-ws.LastComments),
			})
		}
		if m.cursor >= len(m.rows) {
			m.cursor = max(0, len(m.rows)-1)
		}
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case tea.MouseWheelUp:
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
		case key.Matches(msg, m.keys.Bottom):
			if len(m.rows) > 0 {
				m.cursor = len(m.rows) - 1
			}
		case key.Matches(msg, m.keys.Refresh):
			if !m.loading {
				return m.start()
			}
		case key.Matches(msg, m.keys.Watch):
			if row, ok := m.selected(); ok && m.st != nil {
				_, _ = m.st.Toggle(row.item.ID, row.item.Title, row.item.Descendants, time.Now().Unix())
				m.rows = append(m.rows[:m.cursor], m.rows[m.cursor+1:]...)
				if m.cursor >= len(m.rows) {
					m.cursor = max(0, len(m.rows)-1)
				}
			}
		}
	}
	return m, nil
}

func (m watchedModel) View() string {
	var b strings.Builder
	b.WriteString(barWithHints(styleLogo.Render("HN")+styleTabActive.Render("Watched"), m.width, viewWatched))
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(styleError.Render("✗ " + m.err.Error()))
	case m.loading && len(m.rows) == 0:
		b.WriteString(styleMeta.Render(m.spinner.View() + " checking watched stories…"))
	case len(m.rows) == 0:
		b.WriteString(styleMeta.Render("nothing watched yet — press w on a story to watch its discussion"))
	default:
		b.WriteString(m.renderRows())
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m watchedModel) renderRows() string {
	visible := max(1, (m.height-2)/rowHeight)
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	now := time.Now()
	var b strings.Builder
	for i := start; i < min(start+visible, len(m.rows)); i++ {
		r := m.rows[i]
		sel := i == m.cursor

		cur := "  "
		if sel {
			cur = styleCursorBar.Render("▍ ")
		}
		badge := styleMeta.Render("  no new  ")
		if r.newCount > 0 {
			badge = styleRise.Render(fmt.Sprintf("+%-3d new  ", r.newCount))
		}
		titleStyle := styleTitle
		if sel {
			titleStyle = styleTitleSel
		}
		line1 := cur + badge + titleStyle.Render(r.item.Title)
		line2 := "            " + styleMeta.Render(fmt.Sprintf("▲ %d · %s · last read %s",
			r.item.Score, pluralize(r.item.Descendants, "comment"), relAge(r.state.LastReadAt, now)))

		b.WriteString(ansi.Truncate(line1, m.width, "…") + "\n")
		b.WriteString(ansi.Truncate(line2, m.width, "…") + "\n\n")
	}
	return b.String()
}
