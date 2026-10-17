package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
)

// TestSearchResultsLandAfterLeaving: results that arrive after the reader
// has left the search view still belong to the search. Dropping them left
// the view stuck on "searching…" when it was opened again, with r disabled
// because the search still counted as running.
func TestSearchResultsLandAfterLeaving(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith()
	m.search.query, m.search.loading = "sqlite", true

	m = press(t, m, "esc")
	m = update(t, m, searchResultsMsg{query: "sqlite", mode: m.search.mode, items: []hn.Item{result(1, "SQLite tips")}})

	if m.search.loading {
		t.Error("the search is still marked as running after its results arrived")
	}
	if len(m.search.results) != 1 {
		t.Errorf("search has %d results, want the 1 that arrived", len(m.search.results))
	}
}

// TestSearchSpinnerTurns: the search spinner's ticks have to reach it, or
// it shows one still frame for the whole request and looks hung.
func TestSearchSpinnerTurns(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith()
	m.search.query, m.search.loading = "sqlite", true

	before := m.search.spinner.View()
	next, cmd := m.Update(m.search.spinner.Tick())
	if got := next.(Model).search.spinner.View(); got == before {
		t.Error("the search spinner did not move on a tick")
	}
	if cmd == nil {
		t.Error("the search spinner did not schedule its next frame")
	}
}

// TestFeedsSpinnerRestsWhenIdle: with nothing loading, the feeds spinner
// must stop asking for frames. Otherwise it ticks twelve times a second for
// as long as orange is open, and the program never goes idle.
func TestFeedsSpinnerRestsWhenIdle(t *testing.T) {
	m := newFeedsModel(nil, newKeyMap())
	if _, cmd := m.Update(m.spinner.Tick()); cmd != nil {
		t.Error("the feeds spinner kept ticking with nothing loading")
	}
}

// TestFeedsSpinnerRestartsWithALoad: once it has stopped, starting a load
// has to start it again, or "loading more…" shows a still frame.
func TestFeedsSpinnerRestartsWithALoad(t *testing.T) {
	m := newFeedsModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.setSize(100, 24)
	st := m.state()
	for i := 1; i <= 2*storyPageSize; i++ {
		st.ids = append(st.ids, i)
	}
	for i := 1; i <= storyPageSize; i++ {
		st.items = append(st.items, hn.Item{ID: i, Type: "story", Title: "story"})
	}
	st.next = storyPageSize
	st.cursor = storyPageSize - 1

	_, cmd := m.handleKey(keyPress("j"))
	if cmd == nil {
		t.Fatal("reaching the end of the list did not load more")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Error("loading more did not restart the spinner alongside the fetch")
	}
}
