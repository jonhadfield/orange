// Package ui implements the orange terminal interface: a story
// list across the six HN feeds and a threaded comment reader, plus the
// pulse (live front page), watched-stories, and who-is-hiring views.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pkg/browser"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

type view int

const (
	viewFeeds view = iota
	viewStory
	viewPulse
	viewWatched
	viewHiring
	viewSearch
)

type browserOpenedMsg struct{ err error }

type itemLoadedMsg struct {
	item hn.Item
	err  error
}

// Model is the root application model.
type Model struct {
	client   *hn.Client
	st       *store.Store
	keys     keyMap
	help     help.Model
	feeds    feedsModel
	story    storyModel
	pulse    pulseModel
	watched  watchedModel
	hiring   hiringModel
	search   searchModel
	view     view
	prevView view // where esc from the story view returns to
	width    int
	height   int
	notice   string
	// the size last handed to the child views, so they are only re-sized
	// when the space available to them actually changes
	sizedW, sizedH int
}

// New builds the root model backed by the given HN client and local state
// store (st may be nil; watching is then disabled).
func New(client *hn.Client, st *store.Store) Model {
	keys := newKeyMap()
	m := Model{
		client:  client,
		st:      st,
		keys:    keys,
		help:    help.New(),
		feeds:   newFeedsModel(client, keys),
		story:   newStoryModel(client, keys),
		pulse:   newPulseModel(client, keys),
		watched: newWatchedModel(client, st, keys),
		hiring:  newHiringModel(client, keys),
		search:  newSearchModel(client, keys),
	}
	// A warning printed before the alternate screen takes over scrolls past
	// unseen, so a state file that had to be set aside is said here instead.
	if st != nil {
		if moved, ok := st.Recovered(); ok {
			m.notice = "watch list was unreadable and has been started again; the old file is at " + moved
		}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	// The palette is chosen from the terminal background, which v2 reports
	// in reply to this request rather than resolving per colour at render
	// time as AdaptiveColor used to.
	return tea.Batch(m.feeds.init(), tea.RequestBackgroundColor)
}

// Update handles the message and then re-applies the layout, because the
// footer is not a fixed height: a notice or the full help overlay takes
// rows away from the view above, and it has to be told.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	(&next).applyLayout()
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
		// The spinners copied their style when they were built, so they
		// need the rebuilt one handing back to them.
		m.feeds.spinner.Style = stylePoints
		m.story.spinner.Style = stylePoints
		m.pulse.spinner.Style = stylePoints
		m.watched.spinner.Style = stylePoints
		m.hiring.spinner.Style = stylePoints
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case browserOpenedMsg:
		if msg.err != nil {
			m.notice = "browser: " + msg.err.Error()
		}
		return m, nil

	case storeErrMsg:
		// The watch list is right in memory but did not reach the disk, so
		// say so rather than letting it look as though it was saved.
		m.notice = "watch list not saved: " + msg.err.Error()
		return m, nil

	case openItemMsg:
		client, id := m.client, msg.id
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			defer cancel()
			items, err := client.ItemsFresh(ctx, []int{id})
			if err != nil || len(items) == 0 {
				return itemLoadedMsg{err: fmt.Errorf("item %d unavailable: %w", id, err)}
			}
			return itemLoadedMsg{item: items[0]}
		}

	case itemLoadedMsg:
		if msg.err != nil {
			m.notice = "could not load story: " + msg.err.Error()
			return m, nil
		}
		return m.openStory(msg.item, m.prevView)

	case feedIDsMsg, feedItemsMsg:
		var cmd tea.Cmd
		m.feeds, cmd = m.feeds.Update(msg)
		return m, cmd

	case commentsMsg, pastMsg:
		var cmd tea.Cmd
		m.story, cmd = m.story.Update(msg)
		return m, cmd

	case pulseTickMsg:
		if m.view != viewPulse {
			return m, nil // stop refreshing once the view is left
		}
		var cmd tea.Cmd
		m.pulse, cmd = m.pulse.Update(msg)
		return m, cmd

	case pulseDataMsg:
		var cmd tea.Cmd
		m.pulse, cmd = m.pulse.Update(msg)
		return m, cmd

	case watchedDataMsg:
		var cmd tea.Cmd
		m.watched, cmd = m.watched.Update(msg)
		return m, cmd

	case hiringThreadMsg, hiringPostsMsg:
		var cmd tea.Cmd
		m.hiring, cmd = m.hiring.Update(msg)
		return m, cmd

	case spinner.TickMsg:
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.feeds, cmd = m.feeds.Update(msg)
		cmds = append(cmds, cmd)
		m.story, cmd = m.story.Update(msg)
		cmds = append(cmds, cmd)
		m.pulse, cmd = m.pulse.Update(msg)
		cmds = append(cmds, cmd)
		m.watched, cmd = m.watched.Update(msg)
		cmds = append(cmds, cmd)
		m.hiring, cmd = m.hiring.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}
	return m.delegate(msg)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// The hiring filter input owns the keyboard while active.
	if m.view == viewHiring && m.hiring.capturing() {
		var cmd tea.Cmd
		m.hiring, cmd = m.hiring.Update(msg)
		return m, cmd
	}
	// As does the search query line.
	if m.view == viewSearch && m.search.capturing() {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}

	m.notice = ""
	switch {
	case key.Matches(msg, m.keys.Quit):
		// The only synchronous write left. Every other Save runs off the
		// update loop, so a change made just before quitting may still be
		// in memory, and there is no later frame to do it on.
		if m.st != nil {
			_ = m.st.Save()
		}
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, m.keys.Back):
		if m.help.ShowAll {
			m.help.ShowAll = false
			return m, nil
		}
		switch m.view {
		case viewStory:
			m.view = m.prevView
		case viewPulse, viewWatched, viewHiring, viewSearch:
			m.view = viewFeeds
		}
		return m, nil

	case key.Matches(msg, m.keys.Filter) && m.view != viewSearch && m.view != viewHiring && m.view != viewStory:
		m.view = viewSearch
		var cmd tea.Cmd
		m.search, cmd = m.search.begin()
		return m, cmd

	// The p/H/W destination views are reachable from anywhere, matching
	// the hints in the top bar.
	case key.Matches(msg, m.keys.Pulse) && m.view != viewPulse:
		m.view = viewPulse
		var cmd tea.Cmd
		m.pulse, cmd = m.pulse.start()
		return m, cmd

	case key.Matches(msg, m.keys.Hiring) && m.view != viewHiring:
		m.view = viewHiring
		var cmd tea.Cmd
		m.hiring, cmd = m.hiring.start()
		return m, cmd

	case key.Matches(msg, m.keys.Watched) && m.view != viewWatched:
		m.view = viewWatched
		var cmd tea.Cmd
		m.watched, cmd = m.watched.start()
		return m, cmd

	case key.Matches(msg, m.keys.Watch) && m.view != viewWatched && m.view != viewHiring:
		return m.toggleWatch()

	case key.Matches(msg, m.keys.Open) && (m.view == viewFeeds || m.view == viewPulse || m.view == viewWatched || m.view == viewSearch):
		if it, ok := m.current(); ok {
			return m.openStory(it, m.view)
		}
		return m, nil

	case key.Matches(msg, m.keys.OpenURL):
		if it, ok := m.current(); ok {
			u := it.URL
			if u == "" {
				u = hnItemURL(it.ID) // e.g. Ask HN posts have no external link
			}
			return m, openInBrowser(u)
		}
		return m, nil

	case key.Matches(msg, m.keys.OpenHN):
		if it, ok := m.current(); ok {
			return m, openInBrowser(hnItemURL(it.ID))
		}
		return m, nil
	}
	return m.delegate(msg)
}

// delegate routes a message to the active view's model.
func (m Model) delegate(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case viewFeeds:
		m.feeds, cmd = m.feeds.Update(msg)
	case viewStory:
		m.story, cmd = m.story.Update(msg)
	case viewPulse:
		m.pulse, cmd = m.pulse.Update(msg)
	case viewWatched:
		m.watched, cmd = m.watched.Update(msg)
	case viewHiring:
		m.hiring, cmd = m.hiring.Update(msg)
	case viewSearch:
		m.search, cmd = m.search.Update(msg)
	}
	return m, cmd
}

// openStory switches to the story view. For watched stories the previous
// read position highlights new comments, and the read marker advances.
func (m Model) openStory(it hn.Item, from view) (Model, tea.Cmd) {
	if from != viewStory {
		m.prevView = from
	}
	m.view = viewStory
	var save tea.Cmd
	var newSince int64
	if m.st != nil {
		if ws, ok := m.st.Get(it.ID); ok {
			newSince = ws.LastReadAt
			m.st.MarkRead(it.ID, it.Descendants, time.Now().Unix())
			save = saveStore(m.st)
		}
	}
	var cmd tea.Cmd
	m.story, cmd = m.story.open(it, newSince)
	return m, tea.Batch(cmd, save)
}

func (m Model) toggleWatch() (Model, tea.Cmd) {
	it, ok := m.current()
	if !ok {
		return m, nil
	}
	if m.st == nil {
		m.notice = storeUnavailable("watching")
		return m, nil
	}
	// The list changes now and is written afterwards, so the keypress is
	// answered at once. A write that then fails replaces this notice.
	if m.st.Toggle(it.ID, it.Title, it.Descendants, time.Now().Unix()) {
		m.notice = "watching: " + it.Title
	} else {
		m.notice = "stopped watching: " + it.Title
	}
	return m, saveStore(m.st)
}

// storeErrMsg carries a failed write back to the update loop, since the
// write no longer happens where the keypress is handled.
type storeErrMsg struct{ err error }

// saveStore writes the watch list off the update loop. Bubble Tea runs
// Update on one goroutine, and the write is an atomic rewrite with an fsync
// in it: on a network home directory or an encrypted volume, doing that in
// the input path is a visible stall.
func saveStore(st *store.Store) tea.Cmd {
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		if err := st.Save(); err != nil {
			return storeErrMsg{err}
		}
		return nil
	}
}

// current is the item that o/c/w act on in the active view.
func (m Model) current() (hn.Item, bool) {
	switch m.view {
	case viewStory:
		return m.story.story, true
	case viewPulse:
		return m.pulse.selected()
	case viewWatched:
		row, ok := m.watched.selected()
		return row.item, ok
	case viewHiring:
		return m.hiring.selected()
	case viewSearch:
		return m.search.selected()
	default:
		return m.feeds.selected()
	}
}

// storeUnavailable explains that there is no watch state to work with, and
// names the file it would have come from: without the path there is nothing
// the reader can go and fix, and it is documented nowhere else.
func storeUnavailable(what string) string {
	p, err := store.DefaultPath()
	if err != nil {
		return what + " unavailable (state file could not be opened)"
	}
	return what + " unavailable: could not open " + p
}

// footer is the bottom chrome: the contextual key bar, with any notice
// above it. Every line is truncated here because the help bubble gives up
// on truncating once an item lands near the edge and overshoots instead,
// which would wrap the bar and push the view off the top of the screen.
func (m Model) footer() string {
	bottom := m.help.View(m.helpKeys())
	if m.notice != "" {
		bottom = styleError.Render(m.notice) + "\n" + bottom
	}
	lines := strings.Split(bottom, "\n")
	// On a very short terminal the footer has to give way too, or there
	// would be no frame left for the view above it.
	if limit := max(1, m.height-2); len(lines) > limit {
		lines = lines[:limit]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

// contentHeight is the number of rows left for the active view once the
// footer has taken its share.
func (m Model) contentHeight() int {
	return max(1, m.height-lipgloss.Height(m.footer())-1)
}

// applyLayout hands the current content size to every view, so the one on
// screen renders exactly the rows it is going to be shown.
func (m *Model) applyLayout() {
	if m.width == 0 {
		return
	}
	h := m.contentHeight()
	if m.width == m.sizedW && h == m.sizedH {
		return
	}
	m.sizedW, m.sizedH = m.width, h
	m.feeds.setSize(m.width, h)
	m.story.setSize(m.width, h)
	m.pulse.setSize(m.width, h)
	m.watched.setSize(m.width, h)
	m.hiring.setSize(m.width, h)
	m.search.setSize(m.width, h)
}

func (m Model) View() tea.View {
	// v2 makes the alternate screen a property of the view rather than a
	// program option, so it is declared on every frame.
	v := tea.NewView("")
	v.AltScreen = true
	// Report mouse events so the wheel and trackpad scroll the view. The
	// cost is that the terminal no longer handles click-drag selection
	// itself; most terminals still allow it while holding alt or shift.
	v.MouseMode = tea.MouseModeCellMotion
	if m.width == 0 {
		return v
	}
	var content string
	switch m.view {
	case viewStory:
		content = m.story.View()
	case viewPulse:
		content = m.pulse.View()
	case viewWatched:
		content = m.watched.View()
	case viewHiring:
		content = m.hiring.View()
	case viewSearch:
		content = m.search.View()
	default:
		content = m.feeds.View()
	}

	bottom := m.footer()
	contentHeight := max(1, m.height-lipgloss.Height(bottom)-1)
	content = lipgloss.NewStyle().
		Height(contentHeight).
		MaxHeight(contentHeight).
		MaxWidth(m.width).
		Render(content)
	v.Content = content + "\n" + bottom
	return v
}

func hnItemURL(id int) string {
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%d", id)
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return browserOpenedMsg{err: errors.New("nothing to open")}
		}
		return browserOpenedMsg{err: browser.OpenURL(url)}
	}
}
