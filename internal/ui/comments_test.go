package ui

import (
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

func visibleIDs(t *commentTree) []int {
	var ids []int
	for _, n := range t.visible() {
		ids = append(ids, n.item.ID)
	}
	return ids
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCommentTreeBuild(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100},
		{ID: 2, Type: "comment", Parent: 100},
	})
	tree.add([]hn.Item{
		{ID: 3, Type: "comment", Parent: 1},
		{ID: 4, Type: "comment", Parent: 3},
	})

	if got, want := visibleIDs(tree), []int{1, 3, 4, 2}; !equalInts(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}
	if tree.count != 4 {
		t.Errorf("count = %d, want 4", tree.count)
	}
	if d := tree.byID[4].depth; d != 2 {
		t.Errorf("depth of node 4 = %d, want 2", d)
	}
}

// Modelled on HN story 49223082, where dead comment 49223963 has two live
// replies. Dropping the parent used to drop the whole subtree with it.
func TestCommentTreeKeepsRepliesToRemovedComments(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100},
		{ID: 2, Type: "comment", Parent: 100, Dead: true},
	})
	tree.add([]hn.Item{
		{ID: 3, Type: "comment", Parent: 2, Text: "a live reply"},
		{ID: 4, Type: "comment", Parent: 3, Text: "deeper still"},
	})

	// The dead parent shows as a placeholder so the thread keeps its shape,
	// and everything beneath it survives.
	if got, want := visibleIDs(tree), []int{1, 2, 3, 4}; !equalInts(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}
	if !tree.byID[2].placeholder {
		t.Error("node 2 should be a placeholder")
	}
	// Placeholders are structure, not content, so they are not counted.
	if tree.count != 3 {
		t.Errorf("count = %d, want 3 (placeholder excluded)", tree.count)
	}
	if d := tree.byID[4].depth; d != 2 {
		t.Errorf("depth of node 4 = %d, want 2", d)
	}
}

func TestCommentTreeHidesChildlessRemovedComments(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100},
		{ID: 2, Type: "comment", Parent: 100, Deleted: true},
		{ID: 3, Type: "comment", Parent: 100, Dead: true},
	})
	// A removal with nothing under it is pure noise and stays hidden.
	if got, want := visibleIDs(tree), []int{1}; !equalInts(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}

	// So does one whose only descendants are themselves removals.
	tree.add([]hn.Item{{ID: 4, Type: "comment", Parent: 3, Dead: true}})
	if got, want := visibleIDs(tree), []int{1}; !equalInts(got, want) {
		t.Errorf("visible with all-removed subtree = %v, want %v", got, want)
	}
}

func TestCommentTreeSkipsTrueOrphans(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{{ID: 1, Type: "comment", Parent: 100}})
	// Parent 999 was never fetched, so there is nowhere to attach this.
	tree.add([]hn.Item{{ID: 2, Type: "comment", Parent: 999}})

	if got, want := visibleIDs(tree), []int{1}; !equalInts(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}
}

func TestCommentTreeConvertsTextOnce(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{{ID: 1, Type: "comment", Parent: 100, Text: "a &amp; b"}})
	if got, want := tree.byID[1].text, "a & b"; got != want {
		t.Errorf("converted text = %q, want %q", got, want)
	}
}

func TestCommentTreeCollapse(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100},
		{ID: 2, Type: "comment", Parent: 100},
	})
	tree.add([]hn.Item{
		{ID: 3, Type: "comment", Parent: 1},
		{ID: 4, Type: "comment", Parent: 3},
	})

	n1 := tree.byID[1]
	if got := subtreeSize(n1); got != 2 {
		t.Errorf("subtreeSize(1) = %d, want 2", got)
	}

	n1.collapsed = true
	if got, want := visibleIDs(tree), []int{1, 2}; !equalInts(got, want) {
		t.Errorf("visible after collapse = %v, want %v", got, want)
	}

	n1.collapsed = false
	if got, want := visibleIDs(tree), []int{1, 3, 4, 2}; !equalInts(got, want) {
		t.Errorf("visible after expand = %v, want %v", got, want)
	}
}

func TestCommentTreeIgnoresDuplicates(t *testing.T) {
	tree := newCommentTree(100)
	tree.add([]hn.Item{{ID: 1, Type: "comment", Parent: 100}})
	tree.add([]hn.Item{{ID: 1, Type: "comment", Parent: 100}})
	if tree.count != 1 {
		t.Errorf("count after duplicate add = %d, want 1", tree.count)
	}
}
