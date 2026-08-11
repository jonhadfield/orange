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
)

const (
	pulseCount    = 30
	pulseInterval = 45 * time.Second
)

type pulseSample struct {
	rank     int
	score    int
	comments int
}

type pulseRow struct {
	item      hn.Item
	dRank     int
	dScore    int
	dComments int
	isNew     bool
}

type pulseDataMsg struct {
	items []hn.Item
	err   error
}

type pulseTickMsg struct{}

// pulseModel is the auto-refreshing live view of the front page, showing
// rank movement and score/comment velocity between refreshes.
type pulseModel struct {
	client      *hn.Client
	keys        keyMap
	spinner     spinner.Model
	prev        map[int]pulseSample
	baseline    bool
	rows        []pulseRow
	cursor      int
	loading     bool
	err         error
	width       int
	height      int
	refreshedAt time.Time
}

func newPulseModel(client *hn.Client, keys keyMap) pulseModel {
	return pulseModel{
		client:  client,
		keys:    keys,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(stylePoints)),
	}
}

func (m *pulseModel) setSize(w, h int) { m.width, m.height = w, h }

func (m pulseModel) start() (pulseModel, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	m.loading = true
	return m, tea.Batch(m.fetch(), m.spinner.Tick)
}

func (m pulseModel) fetch() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		ids, err := client.FeedIDs(ctx, hn.FeedTop)
		if err != nil {
			return pulseDataMsg{err: err}
		}
		if len(ids) > pulseCount {
			ids = ids[:pulseCount]
		}
		items, err := client.ItemsFresh(ctx, ids)
		return pulseDataMsg{items: items, err: err}
	}
}

func (m pulseModel) selected() (hn.Item, bool) {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].item, true
	}
	return hn.Item{}, false
}

func schedulePulseTick() tea.Cmd {
	return tea.Tick(pulseInterval, func(time.Time) tea.Msg { return pulseTickMsg{} })
}

func (m pulseModel) Update(msg tea.Msg) (pulseModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case pulseTickMsg:
		if m.loading {
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(m.fetch(), m.spinner.Tick)

	case pulseDataMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, schedulePulseTick()
		}
		m.err = nil
		next := make(map[int]pulseSample, len(msg.items))
		rows := make([]pulseRow, 0, len(msg.items))
		for i, it := range msg.items {
			next[it.ID] = pulseSample{rank: i, score: it.Score, comments: it.Descendants}
			row := pulseRow{item: it}
			if p, ok := m.prev[it.ID]; ok {
				row.dRank = p.rank - i
				row.dScore = it.Score - p.score
				row.dComments = it.Descendants - p.comments
			} else if m.baseline {
				row.isNew = true
			}
			rows = append(rows, row)
		}
		m.prev = next
		m.baseline = true
		m.rows = rows
		if m.cursor >= len(rows) {
			m.cursor = max(0, len(rows)-1)
		}
		m.refreshedAt = time.Now()
		return m, schedulePulseTick()

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
				m.loading = true
				return m, tea.Batch(m.fetch(), m.spinner.Tick)
			}
		}
	}
	return m, nil
}

func (m pulseModel) View() string {
	var b strings.Builder
	status := fmt.Sprintf("front page, every %ds · r refreshes now", int(pulseInterval.Seconds()))
	if m.loading {
		status = m.spinner.View() + " refreshing…"
	} else if !m.refreshedAt.IsZero() {
		status = fmt.Sprintf("refreshed %s · every %ds · r refreshes now",
			relAge(m.refreshedAt.Unix(), time.Now()), int(pulseInterval.Seconds()))
	}
	header := styleLogo.Render("HN") + styleTabActive.Render("Pulse") + " " + styleMeta.Render(status)
	b.WriteString(barWithHints(header, m.width, viewPulse))
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(styleError.Render("✗ pulse unavailable: " + m.err.Error()))
	case len(m.rows) == 0:
		b.WriteString(styleMeta.Render(m.spinner.View() + " taking the first reading…"))
	default:
		visible := max(1, m.height-2)
		start := 0
		if m.cursor >= visible {
			start = m.cursor - visible + 1
		}
		for i := start; i < min(start+visible, len(m.rows)); i++ {
			b.WriteString(ansi.Truncate(m.renderRow(i), m.width, "…"))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m pulseModel) renderRow(i int) string {
	r := m.rows[i]
	cur := "  "
	if i == m.cursor {
		cur = styleCursorBar.Render("▍ ")
	}
	move := styleMeta.Render("  · ")
	switch {
	case r.isNew:
		move = styleCollapsed.Render("NEW ")
	case r.dRank > 0:
		move = styleRise.Render(fmt.Sprintf("↑%-3d", r.dRank))
	case r.dRank < 0:
		move = styleFall.Render(fmt.Sprintf("↓%-3d", -r.dRank))
	}
	titleStyle := styleTitle
	if i == m.cursor {
		titleStyle = styleTitleSel
	}
	line := fmt.Sprintf("%s%s %s%s %s",
		cur,
		styleMeta.Render(fmt.Sprintf("%2d.", i+1)),
		move,
		stylePoints.Render(fmt.Sprintf("▲ %-4d", r.item.Score)),
		titleStyle.Render(r.item.Title))
	if r.dScore > 0 {
		line += styleRise.Render(fmt.Sprintf(" +%d", r.dScore))
	}
	if r.dComments > 0 {
		line += styleMeta.Render(fmt.Sprintf(" +%dc", r.dComments))
	}
	return line
}
