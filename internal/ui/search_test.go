package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

func searchWith(results ...hn.Item) searchModel {
	m := newSearchModel(nil, newKeyMap())
	m.setSize(100, 24)
	m.query = "sqlite"
	m.results = results
	return m
}

func result(id int, title string) hn.Item {
	return hn.Item{ID: id, Type: "story", Title: title, By: "someone", Score: 10 * id}
}

// TestSearchQueryLineOwnsTheKeyboard: while a query is being typed, the
// letters have to reach the input rather than moving a cursor underneath it.
// j and k are the sharp case — both are ordinary letters and both are
// movement keys once the search has run.
func TestSearchQueryLineOwnsTheKeyboard(t *testing.T) {
	m, _ := newSearchModel(nil, newKeyMap()).begin()
	if !m.capturing() {
		t.Fatal("begin did not focus the query line")
	}
	for _, c := range []string{"j", "k", "q", "p", "W"} {
		m, _ = m.handleKey(keyPress(c))
	}
	if got := m.input.Value(); got != "jkqpW" {
		t.Errorf("typed query = %q, want %q — a key was taken by the list", got, "jkqpW")
	}
	if m.cursor != 0 {
		t.Errorf("cursor moved to %d while typing", m.cursor)
	}
}

// TestSearchRunsOnEnter: enter is what turns a query into a search, and it
// hands the keyboard back.
func TestSearchRunsOnEnter(t *testing.T) {
	m, _ := newSearchModel(nil, newKeyMap()).begin()
	for _, c := range strings.Split("sqlite", "") {
		m, _ = m.handleKey(keyPress(c))
	}
	m, cmd := m.handleKey(keyPress("enter"))

	if cmd == nil {
		t.Fatal("enter produced no command, so nothing was searched")
	}
	if m.capturing() {
		t.Error("the query line still has the keyboard after enter")
	}
	if m.query != "sqlite" {
		t.Errorf("query = %q, want sqlite", m.query)
	}
	if !m.loading {
		t.Error("the search is not marked as running")
	}
}

// TestSearchEmptyQueryDoesNothing: enter on an empty line must not start a
// search, which would return an arbitrary page of stories.
func TestSearchEmptyQueryDoesNothing(t *testing.T) {
	m, _ := newSearchModel(nil, newKeyMap()).begin()
	m, cmd := m.handleKey(keyPress("enter"))
	if cmd != nil {
		t.Error("an empty query started a search")
	}
	if m.loading {
		t.Error("an empty query set the model loading")
	}
}

// TestSearchEscapeKeepsThePreviousResults: leaving the query line without
// running it must not throw away what is already on screen.
func TestSearchEscapeKeepsThePreviousResults(t *testing.T) {
	m := searchWith(result(1, "First"), result(2, "Second"))
	m, _ = m.begin()
	for _, c := range strings.Split("xyz", "") {
		m, _ = m.handleKey(keyPress(c))
	}
	m, _ = m.handleKey(keyPress("esc"))

	if m.capturing() {
		t.Error("esc did not leave the query line")
	}
	if m.query != "sqlite" {
		t.Errorf("query = %q, want the previous one kept", m.query)
	}
	if len(m.results) != 2 {
		t.Errorf("results were discarded: %d left", len(m.results))
	}
	if got := m.input.Value(); got != "sqlite" {
		t.Errorf("the query line kept the abandoned text %q", got)
	}
}

// TestSearchResultsArriveAndMove covers the list once a search has landed.
func TestSearchResultsArriveAndMove(t *testing.T) {
	m := newSearchModel(nil, newKeyMap())
	m.setSize(100, 24)
	m.query, m.loading = "sqlite", true

	m, _ = m.Update(searchResultsMsg{query: "sqlite", items: []hn.Item{
		result(1, "First"), result(2, "Second"), result(3, "Third"),
	}})
	if m.loading {
		t.Error("the model is still loading after results arrived")
	}
	if len(m.results) != 3 {
		t.Fatalf("got %d results, want 3", len(m.results))
	}

	m, _ = m.handleKey(keyPress("j"))
	if got, _ := m.selected(); got.ID != 2 {
		t.Errorf("j selected story %d, want 2", got.ID)
	}
	m, _ = m.handleKey(keyPress("G"))
	if got, _ := m.selected(); got.ID != 3 {
		t.Errorf("G selected story %d, want 3", got.ID)
	}
	m, _ = m.handleKey(keyPress("g"))
	if got, _ := m.selected(); got.ID != 1 {
		t.Errorf("g selected story %d, want 1", got.ID)
	}
}

// TestSearchIgnoresAStaleResult: a slower earlier search must not land on
// top of a newer one, or typing quickly leaves the wrong list on screen.
func TestSearchIgnoresAStaleResult(t *testing.T) {
	m := newSearchModel(nil, newKeyMap())
	m.setSize(100, 24)
	m.query = "postgres" // the newer search

	m, _ = m.Update(searchResultsMsg{query: "sqlite", items: []hn.Item{result(9, "Stale")}})

	if len(m.results) != 0 {
		t.Errorf("a result for an older query was accepted: %+v", m.results)
	}
}

// TestSearchFailureIsShown: the same failure screen the feed list has, for
// the same reason — it is what a reader meets when the network is down.
func TestSearchFailureIsShown(t *testing.T) {
	m := newSearchModel(nil, newKeyMap())
	m.setSize(100, 24)
	m.query, m.loading = "sqlite", true

	m, _ = m.Update(searchResultsMsg{query: "sqlite", err: errors.New("dial tcp: no such host")})
	if m.err == nil {
		t.Fatal("the failure was not recorded")
	}
	view := stripStyles(m.View())
	for _, want := range []string{"search failed", "dial tcp: no such host", "press r"} {
		if !strings.Contains(view, want) {
			t.Errorf("the failure screen does not mention %q:\n%s", want, view)
		}
	}
}

// TestSearchEmptyAndInitialStatesExplainThemselves: a reader who has not
// searched yet, and one whose search matched nothing, both need telling
// which is which.
func TestSearchEmptyAndInitialStatesExplainThemselves(t *testing.T) {
	fresh := newSearchModel(nil, newKeyMap())
	fresh.setSize(100, 24)
	if view := stripStyles(fresh.View()); !strings.Contains(view, "press / ") {
		t.Errorf("a fresh search does not say how to start one:\n%s", view)
	}

	empty := newSearchModel(nil, newKeyMap())
	empty.setSize(100, 24)
	empty.query = "nothingmatchesthis"
	if view := stripStyles(empty.View()); !strings.Contains(view, "nothing matched") {
		t.Errorf("an empty result set is not explained:\n%s", view)
	}
}

// TestSlashOpensSearchFromTheStoryList wires the whole thing together: the
// key a reader presses, from the view they are on.
func TestSlashOpensSearchFromTheStoryList(t *testing.T) {
	m := newTestModel(t)
	if m.view != viewFeeds {
		t.Fatalf("the model did not start on the story list")
	}

	next, _ := m.Update(keyPress("/"))
	root := next.(Model)
	if root.view != viewSearch {
		t.Fatalf("/ did not open the search, view = %v", root.view)
	}
	if !root.search.capturing() {
		t.Error("the search opened without focusing the query line")
	}

	// And the key bar says what the query line responds to.
	bar := stripStyles(root.footer())
	if !strings.Contains(bar, "enter") || !strings.Contains(bar, "esc") {
		t.Errorf("the key bar does not describe the query line:\n%s", bar)
	}
}

// TestEscapeLeavesSearchForTheStoryList: the way out has to work once the
// query line no longer has the keyboard.
func TestEscapeLeavesSearchForTheStoryList(t *testing.T) {
	m := newTestModel(t)
	next, _ := m.Update(keyPress("/"))
	root := next.(Model)

	// Leave the query line first, then the view.
	next, _ = root.Update(keyPress("esc"))
	root = next.(Model)
	if root.view != viewSearch {
		t.Fatal("the first esc left the view rather than the query line")
	}
	next, _ = root.Update(keyPress("esc"))
	if got := next.(Model).view; got != viewFeeds {
		t.Errorf("the second esc left view = %v, want the story list", got)
	}
}

// TestSearchResultCanBeWatchedAndOpened: a result is a story like any
// other, so the root model has to see it as the current item.
func TestSearchResultCanBeWatchedAndOpened(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith(result(7, "A found story"))

	it, ok := m.current()
	if !ok {
		t.Fatal("the root model sees no current item on the search view")
	}
	if it.ID != 7 || it.Title != "A found story" {
		t.Errorf("current item = %+v, want the selected result", it)
	}
}

// TestEnterOpensASearchResult is the whole point of the feature, and the
// step a model test cannot see: the root decides which views enter opens a
// story from, and a view missing from that list simply does nothing.
func TestEnterOpensASearchResult(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith(result(7, "A found story"), result(8, "Another"))

	next, cmd := m.Update(keyPress("enter"))
	root := next.(Model)

	if root.view != viewStory {
		t.Fatalf("enter left the view on %v, want the story view", root.view)
	}
	if root.story.story.ID != 7 {
		t.Errorf("opened story %d, want the selected result 7", root.story.story.ID)
	}
	if cmd == nil {
		t.Error("opening produced no command, so the thread will not load")
	}
	// And esc comes back to the results rather than the story list.
	back, _ := root.Update(keyPress("esc"))
	if got := back.(Model).view; got != viewSearch {
		t.Errorf("esc from the story went to %v, want back to the search", got)
	}
}
