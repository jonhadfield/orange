package ui

import "github.com/jonhadfield/orange/internal/hn"

// commentNode is one comment in a story's thread tree.
type commentNode struct {
	item      hn.Item
	depth     int
	children  []*commentNode
	collapsed bool
}

// commentTree holds the comments loaded so far for one story, indexed by ID
// so replies can be attached as they arrive from the API.
type commentTree struct {
	rootID int
	roots  []*commentNode
	byID   map[int]*commentNode
	count  int
}

func newCommentTree(rootID int) *commentTree {
	return &commentTree{rootID: rootID, byID: make(map[int]*commentNode)}
}

// add attaches fetched comments beneath their parents, returning the items
// that were actually added. Deleted, dead, and orphaned comments (whose
// parent was itself dropped) are skipped.
func (t *commentTree) add(items []hn.Item) []hn.Item {
	var added []hn.Item
	for _, it := range items {
		if it.Deleted || it.Dead || it.Type != "comment" {
			continue
		}
		if _, ok := t.byID[it.ID]; ok {
			continue
		}
		n := &commentNode{item: it}
		switch parent, ok := t.byID[it.Parent]; {
		case it.Parent == t.rootID:
			t.roots = append(t.roots, n)
		case ok:
			n.depth = parent.depth + 1
			parent.children = append(parent.children, n)
		default:
			continue
		}
		t.byID[it.ID] = n
		t.count++
		added = append(added, it)
	}
	return added
}

// visible flattens the tree depth-first, skipping the children of collapsed
// nodes.
func (t *commentTree) visible() []*commentNode {
	var out []*commentNode
	var walk func(nodes []*commentNode)
	walk = func(nodes []*commentNode) {
		for _, n := range nodes {
			out = append(out, n)
			if !n.collapsed {
				walk(n.children)
			}
		}
	}
	walk(t.roots)
	return out
}

// subtreeSize returns the number of loaded descendants of n.
func subtreeSize(n *commentNode) int {
	total := 0
	for _, c := range n.children {
		total += 1 + subtreeSize(c)
	}
	return total
}
