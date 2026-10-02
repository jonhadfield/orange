package ui

import "testing"

func TestIndexFromAndUpTo(t *testing.T) {
	lineOf := []int{0, 5, 12, 20}
	if got := indexFrom(lineOf, 12); got != 2 {
		t.Errorf("indexFrom(12) = %d, want 2", got)
	}
	if got := indexFrom(lineOf, 13); got != 3 {
		t.Errorf("indexFrom(13) = %d, want 3", got)
	}
	if got := indexFrom(lineOf, 100); got != -1 {
		t.Errorf("indexFrom(100) = %d, want -1", got)
	}
	if got := indexUpTo(lineOf, 12); got != 2 {
		t.Errorf("indexUpTo(12) = %d, want 2", got)
	}
	if got := indexUpTo(lineOf, 11); got != 1 {
		t.Errorf("indexUpTo(11) = %d, want 1", got)
	}
	if got := indexUpTo(lineOf, -1); got != -1 {
		t.Errorf("indexUpTo(-1) = %d, want -1", got)
	}
}

func TestIndexInViewAndCursorOffScreen(t *testing.T) {
	lineOf := []int{0, 5, 12, 20}
	if got := indexInView(lineOf, 5, 15); got != 1 {
		t.Errorf("indexInView = %d, want 1", got)
	}
	if got := indexInView(lineOf, 13, 15); got != -1 {
		t.Errorf("indexInView inside a tall entry = %d, want -1", got)
	}
	if !cursorLineOffScreen(lineOf, 0, 5, 10) {
		t.Error("cursor at line 0 should be off screen when view starts at 5")
	}
	if cursorLineOffScreen(lineOf, 1, 5, 10) {
		t.Error("cursor at line 5 should be on screen")
	}
}
