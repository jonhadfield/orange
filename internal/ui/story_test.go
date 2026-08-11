package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
)

func keyPress(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func ctrlPress(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c, Mod: tea.ModCtrl}
}

// headerVisible reports whether the selected comment's header is on screen,
// which is what makes the highlight something the reader can actually see.
func headerVisible(m storyModel) bool {
	line := m.lineOf[m.cursor]
	return line >= m.vp.YOffset() && line < m.vp.YOffset()+m.vp.Height()
}

// newScrollTestModel builds a story view holding more comments than fit on
// screen, so the viewport can be scrolled away from the selection.
func newScrollTestModel(t *testing.T, n int) storyModel {
	t.Helper()
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.story = hn.Item{ID: 100, Title: "a story", Descendants: n}
	m.tree = newCommentTree(100)
	items := make([]hn.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, hn.Item{ID: i, Type: "comment", Parent: 100, By: "someone", Text: "a comment body"})
	}
	m.tree.add(items)
	(&m).setSize(80, 14)
	if len(m.nodes) != n {
		t.Fatalf("rendered %d nodes, want %d", len(m.nodes), n)
	}
	return m
}

// onScreen reports whether the selected node starts within the viewport.
func onScreen(m storyModel) bool {
	line := m.lineOf[m.cursor]
	return line >= m.vp.YOffset() && line < m.vp.YOffset()+m.vp.Height()
}

func TestDownMatchesBinding(t *testing.T) {
	// Guards the harness itself: the rest of these tests are meaningless if
	// the synthesised key does not match the binding.
	if !keyMatchesDown(t) {
		t.Fatal(`KeyPressMsg for "j" does not match the Down binding`)
	}
}

func keyMatchesDown(t *testing.T) bool {
	t.Helper()
	m := newScrollTestModel(t, 5)
	before := m.cursor
	m, _ = m.handleKey(keyPress("j"))
	return m.cursor != before
}

func TestScrollMatchesBinding(t *testing.T) {
	// Guards the harness: the scroll tests are meaningless if the
	// synthesised ctrl+d does not match the binding.
	m := newScrollTestModel(t, 40)
	before := m.vp.YOffset()
	m, _ = m.handleKey(ctrlPress('d'))
	if m.vp.YOffset() == before {
		t.Fatal("ctrl+d did not scroll; the synthesised key does not match the binding")
	}
}

func TestScrollingHighlightsTheCommentBeingRead(t *testing.T) {
	m := newScrollTestModel(t, 60)
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}

	step := max(1, m.vp.Height()/2)
	want := m.vp.YOffset() + step

	m, _ = m.handleKey(ctrlPress('d'))

	if m.cursor == 0 {
		t.Error("selection stayed on the first comment after scrolling")
	}
	if !headerVisible(m) {
		t.Errorf("selected comment %d starts at line %d, outside the view [%d,%d): "+
			"the highlight would not be visible",
			m.cursor, m.lineOf[m.cursor], m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
	}
	// It must be the topmost comment on screen.
	if m.cursor > 0 && m.lineOf[m.cursor-1] >= m.vp.YOffset() {
		t.Errorf("selected comment %d is not the topmost one in view", m.cursor)
	}
	// The scroll must not be fought by re-centring on the selection.
	if m.vp.YOffset() != want {
		t.Errorf("Y offset = %d after scrolling, want %d: the view was moved to chase the selection",
			m.vp.YOffset(), want)
	}
}

func TestRepeatedScrollingKeepsAdvancingTheSelection(t *testing.T) {
	m := newScrollTestModel(t, 80)

	seen := []int{m.cursor}
	for range 4 {
		m, _ = m.handleKey(ctrlPress('d'))
		seen = append(seen, m.cursor)
		if !headerVisible(m) {
			t.Fatalf("selected comment %d is off screen after scrolling", m.cursor)
		}
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Fatalf("selection did not advance while scrolling down: %v", seen)
		}
	}

	// And back up again.
	m, _ = m.handleKey(ctrlPress('u'))
	if m.cursor >= seen[len(seen)-1] {
		t.Errorf("cursor = %d after scrolling up, want less than %d", m.cursor, seen[len(seen)-1])
	}
	if !headerVisible(m) {
		t.Errorf("selected comment %d is off screen after scrolling up", m.cursor)
	}
}

// Inside a comment taller than the whole screen no header is visible, so the
// selection stays on the comment actually being read.
func TestScrollingInsideAHugeCommentKeepsItSelected(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.story = hn.Item{ID: 100, Title: "a story"}
	m.tree = newCommentTree(100)
	huge := strings.Repeat("an enormous comment that wraps over very many lines indeed. ", 200)
	m.tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100, By: "someone", Text: huge},
		{ID: 2, Type: "comment", Parent: 100, By: "someone", Text: "short"},
	})
	(&m).setSize(80, 14)

	// Land deep inside the first comment, far from either header.
	m.vp.SetYOffset(m.lineOf[1] - 60)
	m, _ = m.handleKey(ctrlPress('d'))

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0: the comment being read", m.cursor)
	}
}

func wheel(b tea.MouseButton) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{Button: b, X: 10, Y: 10}
}

func TestWheelScrollsAndMovesTheSelection(t *testing.T) {
	m := newScrollTestModel(t, 60)
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}

	// Enough notches to carry the view past the first comment.
	for range 5 {
		var mm storyModel
		mm, _ = m.Update(wheel(tea.MouseWheelDown))
		m = mm
	}

	if m.vp.YOffset() == 0 {
		t.Fatal("wheel down did not scroll the view")
	}
	if m.cursor == 0 {
		t.Error("wheel down scrolled but left the selection on the first comment")
	}
	if !headerVisible(m) {
		t.Errorf("selected comment %d starts at line %d, outside the view [%d,%d)",
			m.cursor, m.lineOf[m.cursor], m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
	}

	down, downOffset := m.cursor, m.vp.YOffset()
	for range 5 {
		var mm storyModel
		mm, _ = m.Update(wheel(tea.MouseWheelUp))
		m = mm
	}
	if m.vp.YOffset() >= downOffset {
		t.Errorf("Y offset = %d after wheeling up, want less than %d", m.vp.YOffset(), downOffset)
	}
	if m.cursor >= down {
		t.Errorf("cursor = %d after wheeling up, want less than %d", m.cursor, down)
	}
}

func TestWheelUpStopsAtTheTop(t *testing.T) {
	m := newScrollTestModel(t, 20)
	for range 10 {
		var mm storyModel
		mm, _ = m.Update(wheel(tea.MouseWheelUp))
		m = mm
	}
	if got := m.vp.YOffset(); got != 0 {
		t.Errorf("Y offset = %d at the top, want 0", got)
	}
}

func TestDownAfterScrollContinuesFromWhatIsOnScreen(t *testing.T) {
	m := newScrollTestModel(t, 60)
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}

	// Free scroll well past the selection, as ctrl+d does: the viewport
	// moves and the selection deliberately stays put.
	m.vp.SetYOffset(40)
	if onScreen(m) {
		t.Fatal("selection is still on screen; the test did not scroll far enough")
	}

	m, _ = m.handleKey(keyPress("j"))

	if m.cursor == 1 {
		t.Error("cursor moved to the comment after the pre-scroll selection, not the one on screen")
	}
	if !onScreen(m) {
		t.Errorf("cursor %d is at line %d, outside the view [%d,%d)",
			m.cursor, m.lineOf[m.cursor], m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
	}
	// It should be the topmost comment on screen, so the one before it must
	// start above the viewport.
	if m.cursor > 0 && m.lineOf[m.cursor-1] >= m.vp.YOffset() {
		t.Errorf("cursor %d is not the topmost comment in view", m.cursor)
	}
}

func TestUpAfterScrollContinuesFromWhatIsOnScreen(t *testing.T) {
	m := newScrollTestModel(t, 60)
	m.cursor = 55
	(&m).renderContent()
	(&m).ensureCursorVisible()

	// Scroll back up, away from the selection.
	m.vp.SetYOffset(0)
	if onScreen(m) {
		t.Fatal("selection is still on screen; the test did not scroll far enough")
	}

	m, _ = m.handleKey(keyPress("k"))

	if m.cursor == 54 {
		t.Error("cursor moved to the comment before the pre-scroll selection, not the one on screen")
	}
	if !onScreen(m) {
		t.Errorf("cursor %d is at line %d, outside the view [%d,%d)",
			m.cursor, m.lineOf[m.cursor], m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
	}
}

func TestDownWithoutScrollingStepsOneComment(t *testing.T) {
	m := newScrollTestModel(t, 60)

	// The selection is visible, so ordinary stepping must be untouched.
	for want := 1; want <= 3; want++ {
		m, _ = m.handleKey(keyPress("j"))
		if m.cursor != want {
			t.Fatalf("cursor = %d after %d presses, want %d", m.cursor, want, want)
		}
		if !onScreen(m) {
			t.Fatalf("cursor %d scrolled out of view during ordinary stepping", m.cursor)
		}
	}

	m, _ = m.handleKey(keyPress("k"))
	if m.cursor != 2 {
		t.Errorf("cursor = %d after moving up, want 2", m.cursor)
	}
}

// The last comment can be taller than the screen, which is the reason Down
// scrolls instead of stopping once the selection is on the final comment.
func TestDownAtTheEndRevealsTheRestOfALongComment(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.story = hn.Item{ID: 100, Title: "a story"}
	m.tree = newCommentTree(100)
	long := strings.Repeat("a long trailing comment that wraps over many lines. ", 60)
	m.tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100, By: "someone", Text: "short"},
		{ID: 2, Type: "comment", Parent: 100, By: "someone", Text: long},
	})
	(&m).setSize(80, 14)

	m.cursor = len(m.nodes) - 1
	(&m).renderContent()
	(&m).ensureCursorVisible()

	before := m.vp.YOffset()
	m, _ = m.handleKey(keyPress("j"))

	if m.cursor != len(m.nodes)-1 {
		t.Errorf("cursor = %d on the last comment, want it to stay at %d", m.cursor, len(m.nodes)-1)
	}
	if m.vp.YOffset() <= before {
		t.Errorf("Y offset = %d, want it past %d: Down should reveal more of the long comment",
			m.vp.YOffset(), before)
	}
}

// Scrolling deep inside one very long comment leaves no comment starting on
// screen. Down should reach the next comment, which is just below the fold,
// rather than crawling a line at a time or snapping back.
func TestDownInsideALongCommentGoesToTheNextComment(t *testing.T) {
	m := newStoryModel(hn.NewClient("http://unused.invalid"), newKeyMap())
	m.story = hn.Item{ID: 100, Title: "a story"}
	m.tree = newCommentTree(100)
	long := strings.Repeat("a very long comment that wraps over a great many lines. ", 120)
	m.tree.add([]hn.Item{
		{ID: 1, Type: "comment", Parent: 100, By: "someone", Text: long},
		{ID: 2, Type: "comment", Parent: 100, By: "someone", Text: "short"},
	})
	(&m).setSize(80, 14)

	// Park the view in the middle of the first comment's body, past its
	// header but well before the second comment starts.
	m.vp.SetYOffset(m.lineOf[1] - 30)
	before := m.vp.YOffset()

	m, _ = m.handleKey(keyPress("j"))

	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1: the next comment below the fold", m.cursor)
	}
	if !onScreen(m) {
		t.Errorf("cursor %d at line %d is outside the view [%d,%d)",
			m.cursor, m.lineOf[m.cursor], m.vp.YOffset(), m.vp.YOffset()+m.vp.Height())
	}
	if m.vp.YOffset() <= before {
		t.Errorf("Y offset = %d, want it past %d", m.vp.YOffset(), before)
	}
}
