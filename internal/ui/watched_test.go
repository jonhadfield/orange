package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

func TestWatchedKeepsItemsOnPartialFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "one", 2, 100)
	st.Toggle(2, "two", 3, 100)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m, _ = m.start()
	m, _ = m.Update(watchedDataMsg{
		items: []hn.Item{
			{ID: 1, Type: "story", Title: "one", Descendants: 5, Score: 10},
		},
		err: &hn.PartialError{Fetched: 1, Requested: 2, Err: errors.New("timeout")},
	})

	if m.err != nil {
		t.Fatalf("partial failure became hard error: %v", m.err)
	}
	if m.warn == "" {
		t.Fatal("partial failure produced no warning")
	}
	if len(m.rows) != 1 || m.rows[0].item.ID != 1 {
		t.Fatalf("rows = %+v, want the story that arrived", m.rows)
	}
	if view := stripStyles(m.View()); !strings.Contains(view, "one") {
		t.Errorf("partial load hid the arrived story:\n%s", view)
	}
}

func TestWatchedTotalFailureShowsRetryAdvice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "one", 0, 1)

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m, _ = m.start()
	m, _ = m.Update(watchedDataMsg{err: errors.New("dial failed")})

	if m.err == nil {
		t.Fatal("total failure was not recorded")
	}
	view := stripStyles(m.View())
	if !strings.Contains(view, "press r") {
		t.Errorf("failure view does not name the retry key:\n%s", view)
	}
}

func TestWatchedSortsByUnreadCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// LastComments are the watermarks; Descendants on the fetch decide +N.
	st.Toggle(1, "quiet recently", 5, 300) // 0 new, read last
	st.Toggle(2, "a few new", 8, 200)      // 2 new
	st.Toggle(3, "many new", 1, 100)       // 9 new

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m, _ = m.Update(watchedDataMsg{items: []hn.Item{
		{ID: 1, Type: "story", Title: "quiet recently", Descendants: 5, Score: 1},
		{ID: 2, Type: "story", Title: "a few new", Descendants: 10, Score: 2},
		{ID: 3, Type: "story", Title: "many new", Descendants: 10, Score: 3},
	}})

	if len(m.rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(m.rows))
	}
	got := []int{m.rows[0].item.ID, m.rows[1].item.ID, m.rows[2].item.ID}
	want := []int{3, 2, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (most unread first)", got, want)
		}
	}
	if m.rows[0].newCount != 9 || m.rows[1].newCount != 2 || m.rows[2].newCount != 0 {
		t.Errorf("newCounts = %d,%d,%d, want 9,2,0", m.rows[0].newCount, m.rows[1].newCount, m.rows[2].newCount)
	}
}

func TestUnwatchRefetchesRemainingCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "one", 1, 100)
	st.Toggle(2, "two", 2, 100)

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m.rows = []watchedRow{
		{item: hn.Item{ID: 1, Title: "one", Descendants: 5}, state: store.WatchState{ID: 1, LastComments: 1}, newCount: 4},
		{item: hn.Item{ID: 2, Title: "two", Descendants: 3}, state: store.WatchState{ID: 2, LastComments: 2}, newCount: 1},
	}

	m, cmd := m.Update(keyPress("w"))
	if cmd == nil {
		t.Fatal("unwatch produced no command")
	}
	if len(m.rows) != 1 || m.rows[0].item.ID != 2 {
		t.Fatalf("rows after unwatch = %+v, want only story 2", m.rows)
	}
	if !m.loading {
		t.Error("unwatch did not start a count refresh")
	}
	if st.IsWatched(1) {
		t.Error("story 1 is still watched")
	}

	// Simulate the refresh: remaining story picks up newer comment counts.
	m, _ = m.Update(watchedDataMsg{items: []hn.Item{
		{ID: 2, Type: "story", Title: "two", Descendants: 9, Score: 10},
	}})
	if m.loading {
		t.Error("refresh left the model loading")
	}
	if len(m.rows) != 1 || m.rows[0].newCount != 7 {
		t.Fatalf("refreshed row = %+v, want newCount 7", m.rows)
	}
}
