package ui

import "testing"

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
