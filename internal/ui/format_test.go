package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestPluralize covers the words the views actually pass it. "reply" was
// the one that broke: a naive trailing s made "[+4 replys]" appear under a
// folded comment.
func TestPluralize(t *testing.T) {
	tests := []struct {
		n    int
		word string
		want string
	}{
		{1, "comment", "1 comment"},
		{0, "comment", "0 comments"},
		{2, "comment", "2 comments"},
		{1, "reply", "1 reply"},
		{2, "reply", "2 replies"},
		{4, "reply", "4 replies"},
		{1, "post", "1 post"},
		{3, "post", "3 posts"},
		{2, "result", "2 results"},
		// A vowel before the y keeps it: days, not daies.
		{2, "day", "2 days"},
		{2, "key", "2 keys"},
		// Nothing to trim off a one-letter word.
		{2, "y", "2 ys"},
	}
	for _, tt := range tests {
		if got := pluralize(tt.n, tt.word); got != tt.want {
			t.Errorf("pluralize(%d, %q) = %q, want %q", tt.n, tt.word, got, tt.want)
		}
	}
}

// TestWrapTextKeepsCodeIndented: a code line too long for the column used
// to wrap back to column 0, in among the rest of the block, so it no longer
// read as code. Continuations keep the line's indent, two further in so a
// wrapped line is not mistaken for the next one.
func TestWrapTextKeepsCodeIndented(t *testing.T) {
	got := wrapText("intro\n    result = compute(alpha, beta, gamma)\n    done()", 24)
	want := "intro\n" +
		"    result =\n" +
		"      compute(alpha,\n" +
		"      beta, gamma)\n" +
		"    done()"
	if got != want {
		t.Errorf("wrapText =\n%s\nwant\n%s", got, want)
	}
}

// TestWrapTextLeavesProseAlone: text with no indent must wrap exactly as it
// did before, since that is nearly every comment.
func TestWrapTextLeavesProseAlone(t *testing.T) {
	prose := "the quick brown fox jumps over the lazy dog\n\nand again, the quick brown fox"
	if got, want := wrapText(prose, 16), lipgloss.Wrap(prose, 16, ""); got != want {
		t.Errorf("wrapText =\n%q\nwant lipgloss's own wrap\n%q", got, want)
	}
}

// TestWrapTextFitsEveryLine: nothing it returns may be wider than asked,
// including on a column too narrow for the indent.
func TestWrapTextFitsEveryLine(t *testing.T) {
	in := "        deeply_indented_identifier_that_is_long\n\tx\ty"
	for _, w := range []int{4, 8, 12, 30} {
		for _, line := range strings.Split(wrapText(in, w), "\n") {
			if lipgloss.Width(line) > w {
				t.Errorf("width %d: line %q is %d wide", w, line, lipgloss.Width(line))
			}
		}
	}
}
