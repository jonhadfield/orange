package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

// runAll runs a command and every command batched inside it, returning the
// messages they produce.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// TestReopenedStorySeesNewReplies: coming back to a story has to show the
// replies posted since it was last open. The comments already seen are in
// the item cache with their old reply lists, so reading them from there
// would hide every newer reply that the search index has not caught up on.
func TestReopenedStorySeesNewReplies(t *testing.T) {
	var replies atomic.Value
	replies.Store("[]")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/item/1.json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"id":1,"type":"comment","parent":100,"by":"a","text":"hi","kids":%s}`, replies.Load())
	}))
	defer srv.Close()

	it := hn.Item{ID: 100, Type: "story", Title: "a story", Descendants: 1, Kids: []int{1}}
	load := func(m storyModel) storyModel {
		m, _ = m.open(it, 0)
		m, cmd := m.Update(treeMsg{storyID: 100, gen: m.gen, err: fmt.Errorf("index down")})
		for _, msg := range runAll(cmd) {
			m, _ = m.Update(msg)
		}
		return m
	}

	m := newStoryModel(hn.NewClient(srv.URL), newKeyMap())
	(&m).setSize(80, 24)
	m = load(m)
	if m.queued[2] {
		t.Fatal("reply 2 was queued before it existed")
	}

	replies.Store("[2]")
	m = load(m)
	if !m.queued[2] {
		t.Error("the reopened story did not pick up reply 2: comment 1 came from the cache")
	}
}

// TestFreshStoryAddsNewTopLevelComments: the story a feed row opens can be
// many minutes old. The fresh copy fetched on opening lists the top-level
// comments posted since, and those have to be loaded too, whether it lands
// before or after the search index's tree.
func TestFreshStoryAddsNewTopLevelComments(t *testing.T) {
	stale := hn.Item{ID: 100, Type: "story", Title: "a story", Descendants: 1, Kids: []int{1}}
	fresh := stale
	fresh.Descendants, fresh.Kids = 2, []int{1, 2}

	for _, freshFirst := range []bool{false, true} {
		m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
		(&m).setSize(80, 24)
		m, _ = m.open(stale, 0)
		tree := treeMsg{storyID: 100, gen: m.gen}
		item := storyItemMsg{gen: m.gen, item: fresh}
		if freshFirst {
			m, _ = m.Update(item)
			m, _ = m.Update(tree)
		} else {
			m, _ = m.Update(tree)
			m, _ = m.Update(item)
		}
		if !m.queued[2] {
			t.Errorf("freshFirst=%v: the new top-level comment 2 was not loaded", freshFirst)
		}
		if m.story.Descendants != 2 {
			t.Errorf("freshFirst=%v: header shows %d comments, want the fresh 2", freshFirst, m.story.Descendants)
		}
	}
}

// watchedTestModel builds a root model with a store watching story 42,
// last read when it had 100 comments.
func watchedTestModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "watched.json"))
	if err != nil {
		t.Fatal(err)
	}
	st.Toggle(42, "Watched story", 100, 1)
	m := New(hn.NewClient("http://unused.invalid"), st)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model), st
}

// TestReadMarkerUsesTheFreshCommentCount: opening a watched story records
// how many comments there were, and "+N new" is measured from that. The
// count on a feed row can be well out of date, so the marker has to take
// the count from the fresh story, or comments already read show up as new.
func TestReadMarkerUsesTheFreshCommentCount(t *testing.T) {
	m, st := watchedTestModel(t)
	st0 := m.feeds.state()
	st0.ids = []int{42}
	st0.items = []hn.Item{{ID: 42, Type: "story", Title: "Watched story", Descendants: 100}}

	m = press(t, m, "enter")
	m = update(t, m, storyItemMsg{gen: m.story.gen, item: hn.Item{ID: 42, Type: "story", Title: "Watched story", Descendants: 160}})

	ws, _ := st.Get(42)
	if ws.LastComments != 160 {
		t.Errorf("read marker recorded %d comments, want the fresh 160", ws.LastComments)
	}
}

// TestWatchedListCatchesUpAfterReading: back on the Watched list after
// reading a story, its row must no longer claim unread comments.
func TestWatchedListCatchesUpAfterReading(t *testing.T) {
	m, _ := watchedTestModel(t)
	m = press(t, m, "W")
	m = update(t, m, watchedDataMsg{items: []hn.Item{
		{ID: 42, Type: "story", Title: "Watched story", Descendants: 112},
	}})
	if n := m.watched.rows[0].newCount; n != 12 {
		t.Fatalf("row shows +%d new, want +12", n)
	}

	m = press(t, m, "enter")
	m = press(t, m, "esc")
	if m.view != viewWatched {
		t.Fatalf("esc went to %v, want Watched", m.view)
	}
	if n := m.watched.rows[0].newCount; n != 0 {
		t.Errorf("row still shows +%d new for the story just read", n)
	}
}

// TestWatchingACommentResultAsksForTheStory: a comment search result knows
// its story's id and title but not how many comments it has, so watching
// from there would record 0 and call the whole thread new.
func TestWatchingACommentResultAsksForTheStory(t *testing.T) {
	m, st := watchedTestModel(t)
	m.view = viewSearch
	m.search = searchWith()
	m.search.mode = searchComments
	m.search.comments = []hn.CommentResult{comment(500, 7, "<p>text", "Some story")}

	m = press(t, m, "w")
	if st.IsWatched(7) {
		t.Error("watching a comment result watched its story with no comment count")
	}
	if !strings.Contains(m.notice, "open") {
		t.Errorf("notice = %q, want it to say to open the story first", m.notice)
	}
}
