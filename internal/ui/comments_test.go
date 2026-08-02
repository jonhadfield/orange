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

func TestCommentTreeSkipsDeletedDeadAndOrphans(t *testing.T) {
	tree := newCommentTree(100)
	added := tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100},
		{ID: 2, Type: "comment", Parent: 100, Deleted: true},
		{ID: 3, Type: "comment", Parent: 100, Dead: true},
	})
	if len(added) != 1 || added[0].ID != 1 {
		t.Fatalf("added = %v, want just item 1", added)
	}
	// Children of a dropped comment are orphans and must be skipped too.
	tree.add([]hn.Item{{ID: 4, Type: "comment", Parent: 2}})

	if got, want := visibleIDs(tree), []int{1}; !equalInts(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
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
