package ui

import (
	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/htmltext"
)

// commentNode is one comment in a story's thread tree.
type commentNode struct {
	item      hn.Item
	text      string // body, converted from HTML once when the node is added
	depth     int
	children  []*commentNode
	collapsed bool
	// placeholder marks a deleted or dead comment. It is kept in the tree
	// only so its replies stay attached, and carries no body of its own.
	placeholder bool
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
// that were actually added. Deleted and dead comments are retained as
// placeholders rather than dropped, so the replies below them survive;
// comments whose parent is genuinely absent are skipped.
func (t *commentTree) add(items []hn.Item) []hn.Item {
	var added []hn.Item
	for _, it := range items {
		if it.Type != "comment" {
			continue
		}
		if _, ok := t.byID[it.ID]; ok {
			continue
		}
		n := &commentNode{item: it, placeholder: it.Deleted || it.Dead}
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
		if !n.placeholder {
			// Converting once here keeps HTML handling off the render
			// path, which runs on every keystroke.
			n.text = htmltext.ConvertLinked(it.Text)
			t.count++
		}
		added = append(added, it)
	}
	return added
}

// visible flattens the tree depth-first, skipping the children of collapsed
// nodes and any placeholder whose subtree holds nothing worth showing.
func (t *commentTree) visible() []*commentNode {
	var out []*commentNode
	var walk func(nodes []*commentNode)
	walk = func(nodes []*commentNode) {
		for _, n := range nodes {
			if n.placeholder && !hasContent(n) {
				continue
			}
			out = append(out, n)
			if !n.collapsed {
				walk(n.children)
			}
		}
	}
	walk(t.roots)
	return out
}

// hasContent reports whether n or any of its descendants is a real comment
// rather than a removal placeholder.
func hasContent(n *commentNode) bool {
	if !n.placeholder {
		return true
	}
	for _, c := range n.children {
		if hasContent(c) {
			return true
		}
	}
	return false
}

// subtreeSize returns the number of real loaded descendants of n, ignoring
// removal placeholders.
func subtreeSize(n *commentNode) int {
	total := 0
	for _, c := range n.children {
		if !c.placeholder {
			total++
		}
		total += subtreeSize(c)
	}
	return total
}
