package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

func TestToggleWatchFromFeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	m := New(hn.NewClient("http://unused.invalid"), st)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	feed := m.feeds.feed()
	m.feeds, _ = m.feeds.Update(feedIDsMsg{feed: feed, ids: []int{42}})
	m.feeds, _ = m.feeds.Update(feedItemsMsg{feed: feed, offset: 0, items: []hn.Item{
		{ID: 42, Type: "story", Title: "Watch me", Descendants: 4, Score: 10},
	}})

	next, cmd := m.Update(keyPress("w"))
	m = next.(Model)
	if !strings.Contains(m.notice, "watching: Watch me") {
		t.Errorf("notice = %q, want watching confirmation", m.notice)
	}
	if !st.IsWatched(42) {
		t.Fatal("story was not added to the watch list")
	}
	if cmd == nil {
		t.Fatal("toggle produced no save command")
	}

	next, cmd = m.Update(keyPress("w"))
	m = next.(Model)
	if !strings.Contains(m.notice, "stopped watching") {
		t.Errorf("notice = %q, want stopped-watching confirmation", m.notice)
	}
	if st.IsWatched(42) {
		t.Fatal("story was not removed from the watch list")
	}
	if cmd == nil {
		t.Fatal("untoggle produced no save command")
	}
}

func TestToggleWatchWithoutStoreExplains(t *testing.T) {
	m := newTestModel(t)
	m.feeds.state().items = []hn.Item{{ID: 1, Type: "story", Title: "x"}}
	next, cmd := m.Update(keyPress("w"))
	m = next.(Model)
	if cmd != nil {
		t.Error("toggle with no store produced a save command")
	}
	if !strings.Contains(m.notice, "unavailable") {
		t.Errorf("notice = %q, want unavailable explanation", m.notice)
	}
}
