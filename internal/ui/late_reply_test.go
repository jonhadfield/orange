package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
)

// TestLateStoryDoesNotPullTheReaderBack: a story that was asked for and
// then left behind must not take over the screen when it finally arrives.
// Here a search result is opened and the reader presses esc before the
// fetch returns.
func TestLateStoryDoesNotPullTheReaderBack(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith(result(7, "A found story"))

	next, cmd := m.Update(keyPress("enter"))
	next, _ = next.Update(cmd())
	late := itemLoadedMsg{seq: next.(Model).openSeq, item: hn.Item{ID: 7, Type: "story", Title: "A found story"}}

	next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if v := next.(Model).view; v != viewFeeds {
		t.Fatalf("esc went to %v, want the feeds", v)
	}
	next, _ = next.Update(late)
	if v := next.(Model).view; v != viewFeeds {
		t.Errorf("the late story moved the reader to %v, want them left on the feeds", v)
	}
}

// TestLateRefreshDoesNotReplaceANewerStory: r in a story asks for it again.
// If the reader opens a different story before that reply lands, the reply
// is out of date and must not replace what is now on screen.
func TestLateRefreshDoesNotReplaceANewerStory(t *testing.T) {
	m := newTestModel(t)
	st := m.feeds.state()
	st.ids = []int{1, 2}
	st.items = []hn.Item{
		{ID: 1, Type: "story", Title: "first"},
		{ID: 2, Type: "story", Title: "second"},
	}

	m = press(t, m, "enter") // open story 1
	m.story.loading = false  // its comments have finished loading
	next, cmd := m.Update(keyPress("r"))
	if cmd == nil {
		t.Fatal("r produced no command")
	}
	next, _ = next.Update(cmd())
	late := itemLoadedMsg{seq: next.(Model).openSeq, item: hn.Item{ID: 1, Type: "story", Title: "first"}}

	m = next.(Model)
	m = press(t, m, "esc")
	m = press(t, m, "j")
	m = press(t, m, "enter") // open story 2
	if m.story.story.ID != 2 {
		t.Fatalf("opened story %d, want 2", m.story.story.ID)
	}
	m = update(t, m, late)
	if m.story.story.ID != 2 {
		t.Errorf("the late refresh of story 1 replaced story 2")
	}
}

// TestCurrentStoryReplyStillOpens: the guard must not drop the reply the
// reader is still waiting for.
func TestCurrentStoryReplyStillOpens(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	m.search = searchWith(result(7, "A found story"))

	next, cmd := m.Update(keyPress("enter"))
	next, _ = next.Update(cmd())
	next, _ = next.Update(itemLoadedMsg{seq: next.(Model).openSeq, item: hn.Item{ID: 7, Type: "story", Title: "A found story"}})
	if got := next.(Model); got.view != viewStory || got.story.story.ID != 7 {
		t.Errorf("view %v, story %d: want story 7 opened", got.view, got.story.story.ID)
	}
}

// TestOldBatchesDoNotUnbalanceAReopenedStory: reopening a story while its
// comment batches are still in flight must not let those old batches count
// against the new load. Counting them drives inflight below zero, which
// ends the spinner and re-enables r while comments are still arriving.
func TestOldBatchesDoNotUnbalanceAReopenedStory(t *testing.T) {
	it := hn.Item{ID: 100, Title: "a story", Descendants: 2, Kids: []int{1, 2}}
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m, _ = m.open(it, 0)
	(&m).setSize(80, 24)
	m, _ = m.Update(treeMsg{storyID: 100, gen: m.gen})
	if m.inflight == 0 {
		t.Fatal("no comment batch was started")
	}
	old := commentsMsg{storyID: 100, gen: m.gen, items: []hn.Item{
		{ID: 1, Type: "comment", Parent: 100, By: "a", Text: "x"},
	}}

	m, _ = m.open(it, 0)
	m, _ = m.Update(old)
	if m.inflight != 0 {
		t.Errorf("inflight = %d after an old batch landed, want 0", m.inflight)
	}
	if !m.loading {
		t.Error("an old batch ended the new load")
	}
}
