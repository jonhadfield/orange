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
	view := stripStyles(empty.View())
	if !strings.Contains(view, "no stories matched") {
		t.Errorf("an empty result set is not explained:\n%s", view)
	}
	// And it points at the other half of the search rather than dead-ending.
	if !strings.Contains(view, "tab") {
		t.Errorf("an empty result set does not offer the other mode:\n%s", view)
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

func comment(id, storyID int, text, story string) hn.CommentResult {
	return hn.CommentResult{
		ID: id, Author: "someone", Text: text,
		StoryID: storyID, StoryTitle: story, Time: 1,
	}
}

// TestSearchTabSwitchesWhatIsSearched: tab changes what the query is
// matched against, the way it changes which feed is shown on the story
// list, and re-runs the search rather than leaving the old answer up.
func TestSearchTabSwitchesWhatIsSearched(t *testing.T) {
	m := searchWith(result(1, "A story"))
	if m.mode != searchStories {
		t.Fatal("search did not start on stories")
	}

	m, cmd := m.handleKey(keyPress("tab"))
	if m.mode != searchComments {
		t.Error("tab did not switch to comments")
	}
	if cmd == nil {
		t.Error("tab did not re-run the query in the new mode")
	}
	// The old answers must go: they are the wrong kind now.
	if len(m.results) != 0 {
		t.Errorf("story results survived the switch: %+v", m.results)
	}

	m, _ = m.handleKey(keyPress("tab"))
	if m.mode != searchStories {
		t.Error("tab did not switch back to stories")
	}
}

// TestSearchTabWithoutAQueryDoesNotSearch: tab before anything has been
// typed changes the mode and nothing else.
func TestSearchTabWithoutAQueryDoesNotSearch(t *testing.T) {
	m := newSearchModel(nil, newKeyMap())
	m.setSize(100, 24)
	m, cmd := m.handleKey(keyPress("tab"))
	if m.mode != searchComments {
		t.Error("tab did not switch mode")
	}
	if cmd != nil {
		t.Error("tab searched for nothing")
	}
}

// TestCommentResultOpensItsThread is the decision the feature rests on: a
// comment on its own is a reply with nothing around it, so what a row
// offers to open is the story it was written under.
func TestCommentResultOpensItsThread(t *testing.T) {
	m := searchWith()
	m.mode = searchComments
	m.comments = []hn.CommentResult{
		comment(500, 100, "<p>SQLite is lovely", "A story about databases"),
		comment(501, 200, "<p>another", "A different story"),
	}

	it, ok := m.selected()
	if !ok {
		t.Fatal("no comment selected")
	}
	if it.ID != 100 {
		t.Errorf("selected item is %d, want the story 100 rather than the comment", it.ID)
	}
	if it.Title != "A story about databases" {
		t.Errorf("title = %q, want the story's", it.Title)
	}

	m, _ = m.handleKey(keyPress("j"))
	if it, _ := m.selected(); it.ID != 200 {
		t.Errorf("after j the item is %d, want story 200", it.ID)
	}
}

// TestCommentRowsShowTheCommentAndItsThread: a comment has no title, so
// the text is the line to read, with the story underneath.
func TestCommentRowsShowTheCommentAndItsThread(t *testing.T) {
	m := searchWith()
	m.mode = searchComments
	m.query = "sqlite"
	m.comments = []hn.CommentResult{
		comment(500, 100, "<p>SQLite is <i>lovely</i> and fast", "A story about databases"),
	}

	view := stripStyles(m.View())
	// The HTML is rendered, not shown raw.
	if strings.Contains(view, "<p>") || strings.Contains(view, "<i>") {
		t.Errorf("the comment HTML was not rendered:\n%s", view)
	}
	if !strings.Contains(view, "SQLite is _lovely_ and fast") {
		t.Errorf("the comment text is not shown:\n%s", view)
	}
	if !strings.Contains(view, "in A story about databases") {
		t.Errorf("the row does not say which thread it came from:\n%s", view)
	}
	if !strings.Contains(view, "in comments") {
		t.Errorf("the header does not say what was searched:\n%s", view)
	}
}

// TestSearchIgnoresAResultForTheOtherMode: switching mode while a search is
// in flight must not land the wrong kind of answer.
func TestSearchIgnoresAResultForTheOtherMode(t *testing.T) {
	m := searchWith()
	m.query, m.mode = "sqlite", searchComments

	m, _ = m.Update(searchResultsMsg{
		query: "sqlite", mode: searchStories, items: []hn.Item{result(1, "A story")},
	})
	if len(m.results) != 0 {
		t.Errorf("a story result landed while searching comments: %+v", m.results)
	}
}

// TestCommentWithNoTextStillRenders: a comment stripped to nothing must not
// leave a blank row with no explanation.
func TestCommentWithNoTextStillRenders(t *testing.T) {
	m := searchWith()
	m.mode, m.query = searchComments, "x"
	m.comments = []hn.CommentResult{comment(1, 2, "", "The story")}
	if view := stripStyles(m.View()); !strings.Contains(view, "(no text)") {
		t.Errorf("an empty comment renders as a blank row:\n%s", view)
	}
}

// TestOpeningACommentFetchesItsStory: the search result carries only the
// story's id and title, so opening it directly would show a header with no
// author, no score and an age of 1970. It has to be fetched first.
func TestOpeningACommentFetchesItsStory(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith()
	m.search.mode = searchComments
	m.search.comments = []hn.CommentResult{
		comment(500, 100, "<p>text", "A story about databases"),
	}

	next, cmd := m.Update(keyPress("enter"))
	root := next.(Model)

	// The view does not change until the story arrives.
	if root.view != viewSearch {
		t.Errorf("view = %v, want it to wait on the search until the story loads", root.view)
	}
	if cmd == nil {
		t.Fatal("enter produced no command, so nothing was fetched")
	}
	msg := cmd()
	open, ok := msg.(openItemMsg)
	if !ok {
		t.Fatalf("enter produced %T, want an openItemMsg asking for the story", msg)
	}
	if open.id != 100 {
		t.Errorf("fetching story %d, want 100", open.id)
	}
	// And coming back from that story returns to the search.
	if root.prevView != viewSearch {
		t.Errorf("prevView = %v, want the search to be returned to", root.prevView)
	}
}

// TestOpeningAStoryResultDoesNotFetch: a story result is already whole, so
// it opens without a round trip.
func TestOpeningAStoryResultDoesNotFetch(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith(result(7, "A found story"))

	next, _ := m.Update(keyPress("enter"))
	if got := next.(Model).view; got != viewStory {
		t.Errorf("view = %v, want the story opened directly", got)
	}
}
