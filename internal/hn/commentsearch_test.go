package hn

import (
	"context"
	"net/http"
	"testing"
)

func TestSearchComments(t *testing.T) {
	var gotTags string
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotTags = r.URL.Query().Get("tags")
		w.Write([]byte(`{"hits":[
			{"objectID":"500","author":"someone","comment_text":"<p>SQLite is lovely","story_id":100,"story_title":"A story about databases","created_at_i":1000},
			{"objectID":"501","author":"other","comment_text":"more","story_id":0,"story_title":"orphan","created_at_i":2000},
			{"objectID":"bad","author":"x","comment_text":"y","story_id":7}
		]}`))
	})

	got, err := c.SearchComments(context.Background(), "  sqlite ")
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if gotTags != "comment" {
		t.Errorf("tags = %q, want comment", gotTags)
	}
	// The orphan has no story to open, and the unparseable id is skipped.
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(got), got)
	}
	c0 := got[0]
	if c0.ID != 500 || c0.Author != "someone" || c0.StoryID != 100 {
		t.Errorf("result = %+v", c0)
	}
	if c0.StoryTitle != "A story about databases" {
		t.Errorf("StoryTitle = %q", c0.StoryTitle)
	}
	if c0.Text != "<p>SQLite is lovely" {
		t.Errorf("Text = %q, want the HTML as served", c0.Text)
	}
	if c0.Time != 1000 {
		t.Errorf("Time = %d", c0.Time)
	}
}

// TestSearchCommentsWithoutAStoryAreDropped is the rule that matters: a
// comment orange cannot open is not worth offering.
func TestSearchCommentsWithoutAStoryAreDropped(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":[
			{"objectID":"1","author":"a","comment_text":"x","story_id":0,"story_title":""}
		]}`))
	})
	got, err := c.SearchComments(context.Background(), "x")
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a comment with no story was offered: %+v", got)
	}
}

func TestSearchCommentsEmptyQuery(t *testing.T) {
	called := false
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"hits":[]}`))
	})
	if got, err := c.SearchComments(context.Background(), "   "); err != nil || got != nil {
		t.Errorf("SearchComments(blank) = %v, %v", got, err)
	}
	if called {
		t.Error("a blank query reached the network")
	}
}

// TestSearchCommentsStripsControlCharacters: comment text, authors and
// story titles are all rendered into a terminal.
func TestSearchCommentsStripsControlCharacters(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":[
			{"objectID":"1","author":"a\u001bb","comment_text":"c\u001bd","story_id":9,"story_title":"e\u001bf"}
		]}`))
	})
	got, err := c.SearchComments(context.Background(), "x")
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	for _, f := range []struct{ name, val string }{
		{"Author", got[0].Author}, {"Text", got[0].Text}, {"StoryTitle", got[0].StoryTitle},
	} {
		for _, r := range f.val {
			if r < 0x20 || r == 0x7f {
				t.Errorf("%s = %q, still contains %#U", f.name, f.val, r)
				break
			}
		}
	}
}
