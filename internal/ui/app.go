// Package ui implements the orange terminal interface: a story
// list across the six HN feeds and a threaded comment reader, plus the
// pulse (live front page), watched-stories, and who-is-hiring views.
package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	view     view
	prevView view // where esc from the story view returns to
	width    int
	height   int
	notice   string
}

// New builds the root model backed by the given HN client and local state
// store (st may be nil; watching is then disabled).
func New(client *hn.Client, st *store.Store) Model {
	keys := newKeyMap()
	return Model{
		client:  client,
		st:      st,
		keys:    keys,
		help:    help.New(),
		feeds:   newFeedsModel(client, keys),
		story:   newStoryModel(client, keys),
		pulse:   newPulseModel(client, keys),
		watched: newWatchedModel(client, st, keys),
		hiring:  newHiringModel(client, keys),
	}
}

func (m Model) Init() tea.Cmd { return m.feeds.init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		content := max(1, msg.Height-2)
		m.feeds.setSize(msg.Width, content)
		m.story.setSize(msg.Width, content)
		m.pulse.setSize(msg.Width, content)
		m.watched.setSize(msg.Width, content)
		m.hiring.setSize(msg.Width, content)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case browserOpenedMsg:
		if msg.err != nil {
			m.notice = "browser: " + msg.err.Error()
		}
		return m, nil

	case openItemMsg:
		client, id := m.client, msg.id
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			defer cancel()
			it, err := client.Item(ctx, id)
			return itemLoadedMsg{item: it, err: err}
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

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The hiring filter input owns the keyboard while active.
	if m.view == viewHiring && m.hiring.capturing() {
		var cmd tea.Cmd
		m.hiring, cmd = m.hiring.Update(msg)
		return m, cmd
	}

	m.notice = ""
	switch {
	case key.Matches(msg, m.keys.Quit):
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
		case viewPulse, viewWatched, viewHiring:
			m.view = viewFeeds
		}
		return m, nil

	case key.Matches(msg, m.keys.Pulse) && m.view == viewFeeds:
		m.view = viewPulse
		var cmd tea.Cmd
		m.pulse, cmd = m.pulse.start()
		return m, cmd

	case key.Matches(msg, m.keys.Hiring) && m.view == viewFeeds:
		m.view = viewHiring
		var cmd tea.Cmd
		m.hiring, cmd = m.hiring.start()
		return m, cmd

	case key.Matches(msg, m.keys.Watched) && m.view == viewFeeds:
		m.view = viewWatched
		var cmd tea.Cmd
		m.watched, cmd = m.watched.start()
		return m, cmd

	case key.Matches(msg, m.keys.Watch) && m.view != viewWatched && m.view != viewHiring:
		return m.toggleWatch()

	case key.Matches(msg, m.keys.Open) && (m.view == viewFeeds || m.view == viewPulse || m.view == viewWatched):
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
func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	}
	return m, cmd
}

// openStory switches to the story view. For watched stories the previous
// read position highlights new comments, and the read marker advances.
func (m Model) openStory(it hn.Item, from view) (tea.Model, tea.Cmd) {
	if from != viewStory {
		m.prevView = from
	}
	m.view = viewStory
	var newSince int64
	if m.st != nil {
		if ws, ok := m.st.Get(it.ID); ok {
			newSince = ws.LastReadAt
			if err := m.st.MarkRead(it.ID, it.Descendants, time.Now().Unix()); err != nil {
				m.notice = "state: " + err.Error()
			}
		}
	}
	var cmd tea.Cmd
	m.story, cmd = m.story.open(it, newSince)
	return m, cmd
}

func (m Model) toggleWatch() (tea.Model, tea.Cmd) {
	it, ok := m.current()
	if !ok {
		return m, nil
	}
	if m.st == nil {
		m.notice = "watching unavailable (state file could not be opened)"
		return m, nil
	}
	watching, err := m.st.Toggle(it.ID, it.Title, it.Descendants, time.Now().Unix())
	switch {
	case err != nil:
		m.notice = "state: " + err.Error()
	case watching:
		m.notice = "watching: " + it.Title
	default:
		m.notice = "stopped watching: " + it.Title
	}
	return m, nil
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
	default:
		return m.feeds.selected()
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
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
	default:
		content = m.feeds.View()
	}

	bottom := m.help.View(m.keys)
	if m.notice != "" {
		bottom = styleError.Render(m.notice) + "\n" + bottom
	}

	contentHeight := max(1, m.height-lipgloss.Height(bottom)-1)
	content = lipgloss.NewStyle().
		Height(contentHeight).
		MaxHeight(contentHeight).
		MaxWidth(m.width).
		Render(content)
	return content + "\n" + bottom
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
