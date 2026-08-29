package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

var allViews = []view{viewFeeds, viewStory, viewPulse, viewWatched, viewHiring}

func viewName(v view) string {
	return map[view]string{
		viewFeeds: "feeds", viewStory: "story", viewPulse: "pulse",
		viewWatched: "watched", viewHiring: "hiring",
	}[v]
}

// filledModel is a root model with every view holding more rows than fit, so
// the layout is under pressure in all of them.
func filledModel(t *testing.T, w, h int, v view) Model {
	t.Helper()
	m := New(hn.NewClient("http://unused.invalid"), nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)

	st := m.feeds.state()
	for i := 1; i <= 60; i++ {
		st.ids = append(st.ids, i)
		st.items = append(st.items, hn.Item{
			ID: i, Type: "story", By: "someone", Score: 100 + i, Descendants: 42,
			Title: fmt.Sprintf("Story number %d about something reasonably long", i),
			URL:   "https://example.com/a/b",
		})
	}
	for i := 1; i <= 30; i++ {
		m.pulse.rows = append(m.pulse.rows, pulseRow{
			item:  hn.Item{ID: i, Score: 90 + i, Title: fmt.Sprintf("Pulse story %d with a headline", i)},
			dRank: i % 5, dScore: i, dComments: i,
		})
	}
	for i := 1; i <= 20; i++ {
		m.watched.rows = append(m.watched.rows, watchedRow{
			item:     hn.Item{ID: i, Score: 210, Descendants: 340, Title: fmt.Sprintf("A watched discussion %d", i)},
			state:    store.WatchState{LastReadAt: 1, LastComments: 300},
			newCount: i,
		})
	}
	m.story = storyWithThread(m.story, 25)
	for i := 1; i <= 20; i++ {
		m.hiring.posts = append(m.hiring.posts, hiringPost{
			item:       hn.Item{ID: i, By: "employer", Type: "comment"},
			headline:   fmt.Sprintf("Company %d | Remote | Golang | full-time", i),
			text:       "Body text for the post, long enough to wrap on a narrow terminal.",
			textLinked: "Body text for the post, long enough to wrap on a narrow terminal.",
		})
	}
	(&m.hiring).recompute()

	m.view = v
	(&m).applyLayout()
	// The sizes were already applied when the window size arrived, so the
	// story tree needs laying out now that it has been filled in.
	m.story.renderContent()
	return m
}

// storyWithThread loads a story model with one deeply nested thread, which is
// what puts the reply indentation under pressure.
func storyWithThread(m storyModel, depth int) storyModel {
	m.story = hn.Item{
		ID: 9, By: "op", Score: 321, Descendants: depth, URL: "https://example.com/x",
		Title: "A story with a moderately long title to test the header bar",
	}
	m.tree = newCommentTree(9)
	items := make([]hn.Item, 0, depth)
	parent := 9
	for i := 100; i < 100+depth; i++ {
		items = append(items, hn.Item{ID: i, Type: "comment", Parent: parent, By: "user",
			Text: "A nested reply with enough words in it that the wrapping actually matters."})
		parent = i
	}
	m.tree.add(items)
	return m
}

func lines(s string) []string { return strings.Split(s, "\n") }

// TestFrameFitsTheTerminal is the layout contract: whatever the view and
// whatever the size, the frame stays inside the terminal. A frame that is a
// line too tall or a column too wide scrolls the terminal and smears the
// display.
func TestFrameFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 24}, {80, 24}, {60, 20}, {40, 16}, {24, 10}, {30, 6}, {20, 4}} {
		w, h := size[0], size[1]
		for _, v := range allViews {
			for _, showAll := range []bool{false, true} {
				m := filledModel(t, w, h, v)
				m.help.ShowAll = showAll
				m.notice = "something worth saying about what just happened"
				(&m).applyLayout()

				content := m.View().Content
				if got := len(lines(content)); got > h {
					t.Errorf("%s w=%d h=%d showAll=%v: %d lines, want at most %d",
						viewName(v), w, h, showAll, got, h)
				}
				for i, l := range lines(content) {
					if lipgloss.Width(l) > w {
						t.Errorf("%s w=%d h=%d showAll=%v: line %d is %d columns wide, want at most %d:\n%q",
							viewName(v), w, h, showAll, i, lipgloss.Width(l), w, l)
						break
					}
				}
			}
		}
	}
}

// TestFooterGrowthTakesRowsFromTheView guards the sizing path: the views are
// told how much room the footer left them, so opening the help overlay
// shortens the list rather than having its last rows clipped away.
func TestFooterGrowthTakesRowsFromTheView(t *testing.T) {
	m := filledModel(t, 100, 24, viewFeeds)
	before := m.feeds.visibleRows()

	next, _ := m.Update(keyPress("?"))
	m = next.(Model)
	if !m.help.ShowAll {
		t.Fatal("? did not open the full help")
	}
	after := m.feeds.visibleRows()
	if after >= before {
		t.Errorf("feed rows = %d with the help overlay open, want fewer than %d", after, before)
	}

	// And the rows the view does render are all still on screen.
	rendered := len(lines(strings.TrimRight(m.feeds.View(), "\n")))
	if budget := m.contentHeight(); rendered > budget {
		t.Errorf("feed view rendered %d lines into a %d line budget", rendered, budget)
	}
}

// TestHalfPageScrollWorksInEveryList covers the ctrl+d/ctrl+u binding that
// the help offers on the list views, which used to be advertised there
// without being handled.
func TestHalfPageScrollWorksInEveryList(t *testing.T) {
	cursors := map[view]func(Model) int{
		viewFeeds:   func(m Model) int { return m.feeds.state().cursor },
		viewPulse:   func(m Model) int { return m.pulse.cursor },
		viewWatched: func(m Model) int { return m.watched.cursor },
	}
	for v, cursor := range cursors {
		m := filledModel(t, 100, 24, v)

		next, _ := m.Update(ctrlPress('d'))
		m = next.(Model)
		down := cursor(m)
		if down == 0 {
			t.Errorf("%s: ctrl+d left the cursor at the top", viewName(v))
		}

		next, _ = m.Update(ctrlPress('u'))
		m = next.(Model)
		if up := cursor(m); up >= down {
			t.Errorf("%s: ctrl+u moved the cursor from %d to %d, want back up", viewName(v), down, up)
		}
	}
}

// TestHelpOffersOnlyWhatTheViewHandles keeps the key hints honest: a key
// listed on a page has to do something on that page.
func TestHelpOffersOnlyWhatTheViewHandles(t *testing.T) {
	// Phrases rather than bare keys, because "/" also appears inside key
	// names such as "j/k" and "enter/l".
	absent := map[view][]string{
		// Only the story list switches feeds, and only the hiring browser
		// filters.
		viewStory:   {"/ filter", "feed"},
		viewPulse:   {"/ filter", "feed"},
		viewWatched: {"/ filter", "feed"},
		// The story list has nothing to fold, expand or filter.
		viewFeeds: {"/ filter", "fold", "expand"},
		// Watching is not offered for individual job posts ("W watched
		// stories" is the destination view, which does work here).
		viewHiring: {"watch/unwatch", "unwatch", "feed"},
	}
	for v, keys := range absent {
		m := filledModel(t, 200, 40, v)
		for _, showAll := range []bool{false, true} {
			m.help.ShowAll = showAll
			text := stripStyles(m.footer())
			for _, k := range keys {
				if strings.Contains(text, k) {
					t.Errorf("%s help (showAll=%v) offers %q, which the view ignores:\n%s",
						viewName(v), showAll, k, text)
				}
			}
		}
	}
}

// TestFilterInputOwnsTheKeyBar: while the hiring filter is being typed it
// has the keyboard, so the bar has to stop offering keys that go nowhere.
func TestFilterInputOwnsTheKeyBar(t *testing.T) {
	m := filledModel(t, 100, 24, viewHiring)
	next, _ := m.Update(keyPress("/"))
	m = next.(Model)
	if !m.hiring.capturing() {
		t.Fatal("/ did not start the filter")
	}
	bar := stripStyles(m.footer())
	for _, gone := range []string{"expand", "open post"} {
		if strings.Contains(bar, gone) {
			t.Errorf("key bar still offers %q while the filter is being typed:\n%s", gone, bar)
		}
	}
	if !strings.Contains(bar, "esc") {
		t.Errorf("key bar does not say how to leave the filter:\n%s", bar)
	}
}

// TestFullHelpListsTheWayOut is the other half of that: the overlay is the
// reference, so it has to reach the end rather than being cut off part-way —
// including on a terminal too narrow for the columns it would rather use,
// where the way out is exactly what a dropped column would take with it.
func TestFullHelpListsTheWayOut(t *testing.T) {
	for _, w := range []int{100, 60, 40} {
		for _, v := range allViews {
			m := filledModel(t, w, 30, v)
			m.help.ShowAll = true
			text := stripStyles(m.footer())
			for _, want := range []string{"quit", "help", "watch"} {
				if !strings.Contains(text, want) {
					t.Errorf("%s full help at w=%d is missing %q:\n%s", viewName(v), w, want, text)
				}
			}
		}
	}
}

// TestTabBarKeepsTheActiveFeed: the tab bar is how the reader knows which
// feed they are on, so the active tab is the last thing to go when the
// terminal is too narrow for all six.
func TestTabBarKeepsTheActiveFeed(t *testing.T) {
	for _, w := range []int{80, 50, 34, 24, 16} {
		for i, f := range feedOrder {
			m := filledModel(t, w, 20, viewFeeds)
			m.feeds.active = i
			bar := stripStyles(m.feeds.tabBar())
			if !strings.Contains(bar, feedNames[f]) {
				t.Errorf("w=%d: tab bar does not show the active feed %q:\n%q", w, feedNames[f], bar)
			}
			if got := lipgloss.Width(m.feeds.tabBar()); got > w {
				t.Errorf("w=%d: tab bar is %d columns wide", w, got)
			}
		}
	}
}

// TestStoryHeaderSurvivesScrolling: the story view keeps a bar of its own, so
// scrolling into a long thread does not lose which story it is or how to
// reach the other views.
func TestStoryHeaderSurvivesScrolling(t *testing.T) {
	m := filledModel(t, 100, 24, viewStory)
	for range 20 {
		next, _ := m.Update(ctrlPress('d'))
		m = next.(Model)
	}
	if m.story.vp.YOffset() == 0 {
		t.Fatal("the thread did not scroll")
	}
	content := stripStyles(m.View().Content)
	top := lines(content)[0]
	for _, want := range []string{"Story", "Pulse", "Hiring", "Watched"} {
		if !strings.Contains(top, want) {
			t.Errorf("story header bar is missing %q after scrolling:\n%q", want, top)
		}
	}
}

// TestDeepRepliesKeepRoomToRead: the reply indentation gives way before the
// comment text does, so a deep thread stays readable on a narrow terminal
// instead of being squeezed off the right-hand edge.
func TestDeepRepliesKeepRoomToRead(t *testing.T) {
	for _, w := range []int{100, 60, 40, 24} {
		m := filledModel(t, w, 30, viewStory)
		for i, l := range lines(m.story.vp.View()) {
			if lipgloss.Width(l) > w {
				t.Errorf("w=%d: thread line %d is %d columns wide:\n%q", w, i, lipgloss.Width(l), l)
				break
			}
		}
		// The deepest reply still has a column of text worth reading.
		body := m.story.renderNode(m.story.nodes[len(m.story.nodes)-1], false, timeZero())
		for _, l := range lines(stripStyles(body)) {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if lipgloss.Width(l) > w {
				t.Errorf("w=%d: deepest reply line is %d columns wide:\n%q", w, lipgloss.Width(l), l)
			}
		}
	}
}

// stripStyles removes ANSI escape sequences so tests can match on text.
func stripStyles(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		for i < len(s) && s[i] != 'm' && s[i] != '\\' && s[i] != 0x07 {
			i++
		}
	}
	return b.String()
}

func timeZero() time.Time { return time.Unix(0, 0) }
