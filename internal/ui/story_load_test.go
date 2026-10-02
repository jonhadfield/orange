package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

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
