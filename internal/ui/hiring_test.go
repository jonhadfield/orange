package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

// hiringWith builds a hiring model holding the given headlines, sized and
// laid out as if the thread had loaded.
func hiringWith(headlines ...string) hiringModel {
	m := newHiringModel(nil, newKeyMap())
	m.setSize(100, 20)
	for i, h := range headlines {
		m.posts = append(m.posts, hiringPost{
			item:       hn.Item{ID: i + 1, Type: "comment", By: "employer"},
			headline:   h,
			text:       h,
			textLinked: h,
			textLow:    strings.ToLower(h),
		})
	}
	m.recompute()
	m.renderContent()
	return m
}

// filterTo types a filter and rebuilds the visible list.
func filterTo(m hiringModel, q string) hiringModel {
	m.input.SetValue(q)
	m.recompute()
	m.renderContent()
	return m
}

// visibleHeadlines is what the reader would actually see.
func visibleHeadlines(m hiringModel) []string {
	out := make([]string, 0, len(m.visible))
	for _, i := range m.visible {
		out = append(out, m.posts[i].headline)
	}
	return out
}

// TestHiringFilterMatchesEveryTerm: the filter is an AND across
// space-separated terms, case-insensitively. An OR would look similar on a
// one-word filter and be wrong on every longer one.
func TestHiringFilterMatchesEveryTerm(t *testing.T) {
	posts := []string{
		"Acme | Go Engineer | REMOTE | full-time",
		"Globex | Rust Engineer | Berlin | ONSITE",
		"Initech | Go Engineer | Berlin | remote-friendly",
		"Umbrella | Python | New York | onsite",
	}
	tests := []struct {
		name, query string
		want        []string
	}{
		{"empty filter shows everything", "", posts},
		{"whitespace is not a term", "   ", posts},
		{"one term", "rust", []string{posts[1]}},
		{"case folds both ways", "REMOTE", []string{posts[0], posts[2]}},
		{"lowercase query matches uppercase post", "onsite", []string{posts[1], posts[3]}},
		{"two terms are an AND, not an OR", "go berlin", []string{posts[2]}},
		{"term order does not matter", "berlin go", []string{posts[2]}},
		{"three terms", "go remote acme", []string{posts[0]}},
		{"a term matching nothing rules the post out", "go tokyo", nil},
		{"no match at all", "cobol", nil},
		{"matches across the whole line, not just the company", "full-time", []string{posts[0]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := filterTo(hiringWith(posts...), tt.query)
			got := visibleHeadlines(m)
			if len(got) != len(tt.want) {
				t.Fatalf("filter %q matched %d posts, want %d:\n got: %v\nwant: %v",
					tt.query, len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("filter %q, post %d = %q, want %q", tt.query, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestHiringFilterKeepsTheCursorInRange: narrowing the list under a cursor
// that was near the bottom must not leave it pointing past the end.
func TestHiringFilterKeepsTheCursorInRange(t *testing.T) {
	m := hiringWith(
		"Acme | Go | REMOTE",
		"Globex | Rust | Berlin",
		"Initech | Go | Berlin",
	)
	m.cursor = 2
	m = filterTo(m, "rust")

	if len(m.visible) != 1 {
		t.Fatalf("filter matched %d posts, want 1", len(m.visible))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d after the list shrank to 1, want 0", m.cursor)
	}
	if _, ok := m.selected(); !ok {
		t.Error("selected() found nothing with a non-empty list")
	}

	// And widening again leaves it somewhere valid.
	m = filterTo(m, "")
	if m.cursor >= len(m.visible) {
		t.Errorf("cursor = %d with %d visible", m.cursor, len(m.visible))
	}
}

// TestHiringFilterOnAnEmptyResultSelectsNothing: with no matches there is
// nothing to select, and selected() must say so rather than index an empty
// slice.
func TestHiringFilterOnAnEmptyResultSelectsNothing(t *testing.T) {
	m := filterTo(hiringWith("Acme | Go | REMOTE"), "cobol")
	if len(m.visible) != 0 {
		t.Fatalf("got %d matches, want none", len(m.visible))
	}
	if _, ok := m.selected(); ok {
		t.Error("selected() returned a post from an empty list")
	}
}

// manyPosts builds enough posts that the list is taller than the viewport,
// which is what the cursor/offset arithmetic exists for.
func manyPosts(n int) hiringModel {
	heads := make([]string, n)
	for i := range heads {
		heads[i] = fmt.Sprintf("Company %d | Engineer | Remote | full-time", i)
	}
	return hiringWith(heads...)
}

// TestHiringPostLineLookups covers the mapping between a post index and the
// content line it starts on, in both directions.
func TestHiringPostLineLookups(t *testing.T) {
	m := manyPosts(20)
	if len(m.lineOf) != 20 {
		t.Fatalf("lineOf has %d entries, want one per post", len(m.lineOf))
	}
	// Collapsed posts are a header plus a blank line, and the lines have to
	// increase or none of the lookups below mean anything.
	for i := 1; i < len(m.lineOf); i++ {
		if m.lineOf[i] <= m.lineOf[i-1] {
			t.Fatalf("lineOf is not increasing at %d: %v", i, m.lineOf[:i+1])
		}
	}

	// postFrom finds the first post at or below a line.
	if got, want := m.postFrom(m.lineOf[5]), 5; got != want {
		t.Errorf("postFrom(line of post 5) = %d, want %d", got, want)
	}
	if got := m.postFrom(m.lineOf[5] - 1); got != 5 {
		t.Errorf("postFrom(just above post 5) = %d, want 5", got)
	}
	if got := m.postFrom(m.lineOf[19] + 100); got != -1 {
		t.Errorf("postFrom(past the end) = %d, want -1", got)
	}

	// postUpTo is the same from the other side.
	if got := m.postUpTo(m.lineOf[5]); got != 5 {
		t.Errorf("postUpTo(line of post 5) = %d, want 5", got)
	}
	if got := m.postUpTo(m.lineOf[5] + 1); got != 5 {
		t.Errorf("postUpTo(just below post 5) = %d, want 5", got)
	}
	if got := m.postUpTo(m.lineOf[0] - 1); got != -1 {
		t.Errorf("postUpTo(above everything) = %d, want -1", got)
	}
}

// TestHiringCursorVisibility: the cursor and the viewport are separate, and
// ensureCursorVisible is what keeps them together.
func TestHiringCursorVisibility(t *testing.T) {
	m := manyPosts(40)

	// A cursor at the top is on screen already.
	m.cursor = 0
	m.vp.SetYOffset(0)
	if m.cursorOffScreen() {
		t.Error("the first post is off screen with the viewport at the top")
	}

	// Move the cursor far down: it is off screen until the viewport follows.
	m.cursor = 30
	if !m.cursorOffScreen() {
		t.Fatal("post 30 is on screen with the viewport still at the top")
	}
	m.ensureCursorVisible()
	if m.cursorOffScreen() {
		t.Errorf("post 30 still off screen after ensureCursorVisible: line %d, offset %d, height %d",
			m.lineOf[30], m.vp.YOffset(), m.vp.Height())
	}

	// And back up again.
	m.cursor = 1
	m.ensureCursorVisible()
	if m.cursorOffScreen() {
		t.Errorf("post 1 off screen after scrolling back up: line %d, offset %d",
			m.lineOf[1], m.vp.YOffset())
	}
}

// TestHiringSelectTopPost is what free scrolling relies on: the reader
// scrolls, and the selection moves to what they are looking at rather than
// the viewport jumping back to the selection.
func TestHiringSelectTopPost(t *testing.T) {
	m := manyPosts(40)
	before := m.vp.YOffset()
	if before != 0 {
		t.Fatalf("viewport starts at %d, want 0", before)
	}

	// Scroll so post 10 is at the top, then adopt it.
	m.vp.SetYOffset(m.lineOf[10])
	m.selectTopPost()

	if m.cursor != 10 {
		t.Errorf("cursor = %d after scrolling post 10 to the top, want 10", m.cursor)
	}
	if got := m.vp.YOffset(); got != m.lineOf[10] {
		t.Errorf("selectTopPost moved the viewport to %d, want it left at %d", got, m.lineOf[10])
	}
}

// TestHiringSelectTopPostPastTheEnd: scrolled below every post header, there
// is nothing "in view", and it has to fall back rather than pick -1.
func TestHiringSelectTopPostPastTheEnd(t *testing.T) {
	m := manyPosts(40)
	m.vp.SetYOffset(m.lineOf[39] + 50)
	m.selectTopPost()

	if m.cursor < 0 || m.cursor >= len(m.visible) {
		t.Errorf("cursor = %d with %d posts, want a valid index", m.cursor, len(m.visible))
	}
}
