package ui

import "testing"

// TestSelectedCommentStaysOnAShortTerminal: the view scrolls to keep a few
// lines of the selected comment below its header. On a terminal shorter
// than that margin, the scroll overshot the header and left the selection
// off the top of the screen, after which j jumped about unpredictably.
func TestSelectedCommentStaysOnAShortTerminal(t *testing.T) {
	for _, h := range []int{3, 4, 5, 7} {
		m := newScrollTestModel(t, 30)
		(&m).setSize(80, h)
		for range 12 {
			m, _ = m.handleKey(keyPress("j"))
			if !onScreen(m) {
				t.Fatalf("height %d (viewport %d): comment %d is off screen at line %d, offset %d",
					h, m.vp.Height(), m.cursor, m.lineOf[m.cursor], m.vp.YOffset())
			}
		}
	}
}

// TestSelectedPostStaysOnAShortTerminal is the same for Hiring posts.
func TestSelectedPostStaysOnAShortTerminal(t *testing.T) {
	for _, h := range []int{4, 5, 6} {
		m := manyPosts(30)
		m.setSize(100, h)
		for range 12 {
			m, _ = m.handleKey(keyPress("j"))
			line := m.lineOf[m.cursor]
			if line < m.vp.YOffset() || line >= m.vp.YOffset()+m.vp.Height() {
				t.Fatalf("height %d (viewport %d): post %d is off screen at line %d, offset %d",
					h, m.vp.Height(), m.cursor, line, m.vp.YOffset())
			}
		}
	}
}

// TestFollowOffsetKeepsItsMarginWhenThereIsRoom: on an ordinary terminal
// the margin is unchanged, so nothing moves for most readers.
func TestFollowOffsetKeepsItsMarginWhenThereIsRoom(t *testing.T) {
	if got := followOffset(30, 0, 20, 4); got != 14 {
		t.Errorf("followOffset(30, 0, 20, 4) = %d, want 14", got)
	}
	if got := followOffset(10, 0, 20, 4); got != 0 {
		t.Errorf("a line already in view moved the offset to %d", got)
	}
	if got := followOffset(5, 9, 20, 4); got != 5 {
		t.Errorf("a line above the view gave offset %d, want 5", got)
	}
}
