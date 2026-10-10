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

// pulseTickMsg is the refresh timer firing. gen says which timer it was:
// only the most recently scheduled one may refresh.
type pulseTickMsg struct{ gen int }

// pulseModel is the auto-refreshing live view of the front page, showing
// rank movement and score/comment velocity between refreshes.
type pulseModel struct {
	client   *hn.Client
	keys     keyMap
	spinner  spinner.Model
	prev     map[int]pulseSample
	baseline bool
	// readings counts the front pages seen. Movement is the difference
	// between two of them, so the first has nothing to show.
	readings    int
	rows        []pulseRow
	cursor      int
	loading     bool
	warn        string
	err         error
	width       int
	height      int
	refreshedAt time.Time
	// tickGen numbers the refresh timers. Scheduling one makes any older
	// timer stale, so a reading asked for by r or by re-entering Pulse
	// replaces the pending timer instead of starting a second loop.
	tickGen int
	// tickPending is whether the current timer has yet to fire. It is
	// cleared when the timer is dropped because Pulse was not on screen,
	// which is how resume knows the loop has stopped.
	tickPending bool
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

// visibleRows is how many one-line rows fit below the header.
func (m pulseModel) visibleRows() int { return max(1, m.height-tabBarHeight) }

func (m pulseModel) selected() (hn.Item, bool) {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].item, true
	}
	return hn.Item{}, false
}

// scheduleTick starts the timer for the next refresh, retiring any other.
func (m *pulseModel) scheduleTick() tea.Cmd {
	m.tickGen++
	m.tickPending = true
	gen := m.tickGen
	return tea.Tick(pulseInterval, func(time.Time) tea.Msg { return pulseTickMsg{gen: gen} })
}

// dropTick is the root model discarding a timer that fired while Pulse was
// not on screen. Nothing refreshes in the background, so if it was the
// current timer the refresh loop has stopped until resume.
func (m pulseModel) dropTick(msg pulseTickMsg) pulseModel {
	if msg.gen == m.tickGen {
		m.tickPending = false
	}
	return m
}

// resume restarts refreshing on returning to Pulse, if its timer was
// dropped while the reader was away. A timer still pending is left to fire.
func (m pulseModel) resume() (pulseModel, tea.Cmd) {
	if m.tickPending {
		return m, nil
	}
	return m.start()
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
		if msg.gen != m.tickGen {
			return m, nil // retired by a later timer
		}
		m.tickPending = false
		if m.loading {
			// The reading under way schedules the next timer.
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(m.fetch(), m.spinner.Tick)

	case pulseDataMsg:
		m.loading = false
		had := len(m.rows) > 0
		m.warn, m.err = applyFetchErr(msg.err, len(msg.items) > 0, had, "stories")
		if m.err != nil {
			return m, m.scheduleTick()
		}
		if len(msg.items) == 0 {
			// A soft failure with nothing new: keep the previous reading
			// on screen rather than blanking the view.
			return m, m.scheduleTick()
		}
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
		m.readings++
		m.rows = rows
		if m.cursor >= len(rows) {
			m.cursor = max(0, len(rows)-1)
		}
		m.refreshedAt = time.Now()
		return m, m.scheduleTick()

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
		case key.Matches(msg, m.keys.ScrollDown):
			m.cursor = scrollCursor(m.cursor, len(m.rows), m.visibleRows(), 1)
		case key.Matches(msg, m.keys.ScrollUp):
			m.cursor = scrollCursor(m.cursor, len(m.rows), m.visibleRows(), -1)
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
	// Every row is still on a first reading, so the movement column is a
	// row of dots and there is nothing to explain it. Say so, rather than
	// leaving a reader to wonder whether the front page is simply quiet.
	if m.readings == 1 && !m.loading {
		status = fmt.Sprintf("first reading · movement appears after the next, in %ds",
			int(pulseInterval.Seconds()))
	}
	header := styleLogo.Render("HN") + styleTabActive.Render("Pulse")
	flex := styleMeta.Render(status)
	if m.warn != "" && !m.loading {
		flex = styleError.Render("⚠ " + m.warn)
	}
	b.WriteString(barWithFlex(header, flex, m.width, viewPulse))
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(styleError.Render("✗ pulse unavailable: " + m.err.Error()))
		b.WriteString("\n\n" + styleMeta.Render("check your connection, then press r to try again"))
	case len(m.rows) == 0:
		b.WriteString(styleMeta.Render(m.spinner.View() + " taking the first reading…"))
	default:
		visible := m.visibleRows()
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
	cur := strings.Repeat(" ", cursorMarkWidth)
	if i == m.cursor {
		cur = styleCursorBar.Render(cursorMark)
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
