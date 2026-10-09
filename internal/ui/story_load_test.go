package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

func pastStory(t *testing.T) storyModel {
	t.Helper()
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m, _ = m.open(hn.Item{
		ID: 100, Title: "a story", URL: "https://example.com/post",
		By: "op", Score: 10, Descendants: 0, Time: 1_700_000_000,
	}, 0)
	(&m).setSize(80, 24)
	return m
}

func TestStoryTreeMsgStartsFirebaseReconcile(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m, _ = m.open(hn.Item{
		ID: 100, Title: "a story", Descendants: 2,
		Kids: []int{1, 2},
	}, 0)
	(&m).setSize(80, 24)

	m, cmd := m.Update(treeMsg{
		storyID: 100,
		items: []hn.Item{
			{ID: 1, Type: "comment", Parent: 100, By: "a", Text: "from algolia"},
		},
	})
	if cmd == nil {
		t.Fatal("treeMsg did not start the Firebase reconcile")
	}
	if m.tree.count != 1 {
		t.Errorf("tree count = %d, want 1", m.tree.count)
	}
	if !m.queued[1] || !m.queued[2] {
		t.Errorf("story kids were not queued: %#v", m.queued)
	}
	if m.inflight == 0 {
		t.Error("no Firebase batch was started")
	}
}

func TestStoryCommentsMsgKeepsPartialBatch(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m, _ = m.open(hn.Item{ID: 100, Title: "a story", Descendants: 3, Kids: []int{1}}, 0)
	(&m).setSize(80, 24)
	// Simulate the Algolia pass finishing so the queue is live.
	m, _ = m.Update(treeMsg{storyID: 100, err: errors.New("algolia down")})
	m.inflight = 1

	m, _ = m.Update(commentsMsg{
		storyID: 100,
		items: []hn.Item{
			{ID: 1, Type: "comment", Parent: 100, By: "a", Text: "kept", Kids: []int{2}},
		},
		err: &hn.PartialError{Fetched: 1, Requested: 2, Err: errors.New("timeout")},
	})

	if m.err != nil {
		t.Fatalf("partial failure became hard error: %v", m.err)
	}
	if m.warn == "" {
		t.Fatal("partial failure produced no warning")
	}
	if m.tree.count != 1 {
		t.Errorf("tree count = %d, want 1 kept comment", m.tree.count)
	}
	if !m.queued[2] {
		t.Error("kids of the kept comment were not queued")
	}
	if view := stripStyles(m.View()); !strings.Contains(view, "failed to load") {
		t.Errorf("status does not show the warning:\n%s", view)
	}
}

func TestStoryIgnoresStaleCommentsMsg(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m, _ = m.open(hn.Item{ID: 100, Title: "a story", Descendants: 1, Kids: []int{1}}, 0)
	(&m).setSize(80, 24)
	m, _ = m.Update(treeMsg{storyID: 100})
	before := m.tree.count
	m.inflight = 1

	m, cmd := m.Update(commentsMsg{
		storyID: 999,
		items:   []hn.Item{{ID: 1, Type: "comment", Parent: 100, Text: "stale"}},
	})
	if cmd != nil {
		t.Error("stale commentsMsg started more work")
	}
	if m.tree.count != before {
		t.Errorf("stale commentsMsg mutated the tree: count %d → %d", before, m.tree.count)
	}
	if m.inflight != 1 {
		t.Errorf("stale commentsMsg changed inflight to %d", m.inflight)
	}
}

// TestPastDiscussionsAppearInHeader is the "Previously on HN" feature the
// reader meets when opening a link that has been submitted before: the list
// lands in the story header, numbered so 1–5 can open them.
func TestPastDiscussionsAppearInHeader(t *testing.T) {
	m := pastStory(t)

	m, _ = m.Update(pastMsg{
		storyID: 100,
		list: []hn.PastDiscussion{
			{ID: 200, Comments: 450, Points: 300, Time: 1_600_000_000},
			{ID: 201, Comments: 12, Points: 40, Time: 1_650_000_000},
		},
	})
	if len(m.past) != 2 {
		t.Fatalf("past = %d entries, want 2", len(m.past))
	}

	view := stripStyles(m.View())
	for _, want := range []string{"previously on HN", "[1]", "[2]", "450 comments", "12 comments"} {
		if !strings.Contains(view, want) {
			t.Errorf("header missing %q:\n%s", want, view)
		}
	}
}

// TestPastDiscussionsIgnoreStaleOrEmpty: a late reply for another story, or
// a failed lookup that arrives as an empty list, must not invent a header
// section or overwrite one that is already on screen.
func TestPastDiscussionsIgnoreStaleOrEmpty(t *testing.T) {
	m := pastStory(t)
	m, _ = m.Update(pastMsg{
		storyID: 100,
		list:    []hn.PastDiscussion{{ID: 200, Comments: 10, Points: 5}},
	})

	m, _ = m.Update(pastMsg{storyID: 999, list: []hn.PastDiscussion{{ID: 300}}})
	if len(m.past) != 1 || m.past[0].ID != 200 {
		t.Fatalf("stale pastMsg mutated past: %+v", m.past)
	}

	m, _ = m.Update(pastMsg{storyID: 100})
	if len(m.past) != 1 {
		t.Fatalf("empty pastMsg cleared the list: %+v", m.past)
	}
}

// TestPastDigitOpensDiscussion: the numbers in the header are keys. Pressing
// one has to ask the app for that story, and a number with no matching row
// has to do nothing.
func TestPastDigitOpensDiscussion(t *testing.T) {
	m := pastStory(t)
	m, _ = m.Update(pastMsg{
		storyID: 100,
		list: []hn.PastDiscussion{
			{ID: 200, Comments: 450, Points: 300},
			{ID: 201, Comments: 12, Points: 40},
		},
	})

	m, cmd := m.handleKey(keyPress("1"))
	if cmd == nil {
		t.Fatal("1 produced no command")
	}
	msg := cmd()
	open, ok := msg.(openItemMsg)
	if !ok {
		t.Fatalf("1 produced %T, want openItemMsg", msg)
	}
	if open.id != 200 {
		t.Errorf("open id = %d, want 200", open.id)
	}

	_, cmd = m.handleKey(keyPress("2"))
	if cmd == nil {
		t.Fatal("2 produced no command")
	}
	if open := cmd().(openItemMsg); open.id != 201 {
		t.Errorf("2 opened %d, want 201", open.id)
	}

	_, cmd = m.handleKey(keyPress("3"))
	if cmd != nil {
		t.Errorf("3 with only two past discussions produced %T", cmd())
	}
}

// TestPastFetchSkippedWithoutURL: Ask HN and other text posts have nothing
// to look up, so opening one must not start a past-discussions request.
func TestPastFetchSkippedWithoutURL(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.story = hn.Item{ID: 100, Title: "Ask HN: anything", Type: "story"}
	if cmd := m.fetchPast(); cmd != nil {
		t.Fatal("fetchPast started a lookup for a story with no URL")
	}
}
