package hn

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	var gotQuery, gotTags, gotHits string
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.Query().Get("query")
		gotTags = r.URL.Query().Get("tags")
		gotHits = r.URL.Query().Get("hitsPerPage")
		w.Write([]byte(`{"hits":[
			{"objectID":"100","title":"SQLite is great","author":"someone","url":"https://sqlite.org","points":250,"num_comments":80,"created_at_i":1000},
			{"objectID":"200","title":"More SQLite","author":"another","url":"","points":10,"num_comments":0,"created_at_i":2000},
			{"objectID":"not-a-number","title":"broken","author":"x"}
		]}`))
	})

	got, err := c.Search(context.Background(), "  sqlite  ")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotQuery != "sqlite" {
		t.Errorf("query sent = %q, want it trimmed to %q", gotQuery, "sqlite")
	}
	// Comments would come back out of their threads, which the reader
	// cannot act on.
	if gotTags != "story" {
		t.Errorf("tags = %q, want story only", gotTags)
	}
	if gotHits == "" {
		t.Error("no hitsPerPage was sent")
	}

	// The unparseable id is skipped rather than becoming item 0.
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(got), got)
	}
	first := got[0]
	if first.ID != 100 || first.Title != "SQLite is great" || first.By != "someone" {
		t.Errorf("first result = %+v", first)
	}
	if first.Score != 250 || first.Descendants != 80 || first.Time != 1000 {
		t.Errorf("first result metadata = %+v", first)
	}
	if first.Type != "story" {
		t.Errorf("Type = %q, want story", first.Type)
	}
	if first.URL != "https://sqlite.org" {
		t.Errorf("URL = %q", first.URL)
	}
}

// TestSearchEmptyQuery: an empty query must not become a request. Algolia
// would answer it with an arbitrary page of stories, which is not what the
// reader asked for.
func TestSearchEmptyQuery(t *testing.T) {
	called := false
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"hits":[]}`))
	})
	for _, q := range []string{"", "   ", "\t\n"} {
		got, err := c.Search(context.Background(), q)
		if err != nil {
			t.Errorf("Search(%q) = %v", q, err)
		}
		if got != nil {
			t.Errorf("Search(%q) returned %d results", q, len(got))
		}
	}
	if called {
		t.Error("an empty query reached the network")
	}
}

// TestSearchStripsControlCharacters: results are rendered into a terminal
// like anything else from the API, so they go through the same stripping.
// The escape arrives as a JSON escape, which is how it would in practice,
// since HN escapes what it serves.
func TestSearchStripsControlCharacters(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":[
			{"objectID":"1","title":"a\u001b[31mtitle","author":"someone","url":"https://x.test/","points":1}
		]}`))
	})
	got, err := c.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	for _, f := range []struct{ name, val string }{
		{"Title", got[0].Title}, {"By", got[0].By}, {"URL", got[0].URL},
	} {
		for _, r := range f.val {
			if r < 0x20 || r == 0x7f {
				t.Errorf("%s = %q, still contains %#U", f.name, f.val, r)
				break
			}
		}
	}
	if !strings.Contains(got[0].Title, "title") {
		t.Errorf("stripping removed more than the control characters: %q", got[0].Title)
	}
}

func TestSearchPropagatesFailure(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream is unhappy", http.StatusInternalServerError)
	})
	if _, err := c.Search(context.Background(), "sqlite"); err == nil {
		t.Error("a failing search returned no error")
	}
}
