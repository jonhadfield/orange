package hn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func newAlgoliaClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("http://unused.invalid")
	c.algoliaURL = srv.URL
	return c
}

func TestPastDiscussions(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"hits":[
			{"objectID":"100","title":"current","url":"https://example.com/post","points":10,"num_comments":5,"created_at_i":1000},
			{"objectID":"200","title":"older big","url":"https://example.com/post","points":300,"num_comments":450,"created_at_i":2000},
			{"objectID":"300","title":"older small","url":"https://example.com/post/","points":50,"num_comments":20,"created_at_i":3000},
			{"objectID":"400","title":"different url","url":"https://example.com/other","points":90,"num_comments":90,"created_at_i":4000}
		]}`))
	})

	got, err := c.PastDiscussions(context.Background(), "https://example.com/post", 100)
	if err != nil {
		t.Fatalf("PastDiscussions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d discussions, want 2 (excluding current story and other URLs): %+v", len(got), got)
	}
	// Sorted most-commented first; trailing-slash URLs match too.
	if got[0].ID != 200 || got[1].ID != 300 {
		t.Errorf("order = [%d %d], want [200 300]", got[0].ID, got[1].ID)
	}
	if got[0].Comments != 450 || got[0].Points != 300 {
		t.Errorf("hit fields = %+v, want comments 450, points 300", got[0])
	}
}

func TestLatestHiringThread(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search_by_date" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"hits":[
			{"objectID":"901","title":"Ask HN: Who wants to be hired? (July 2026)"},
			{"objectID":"902","title":"Ask HN: Who is hiring? (July 2026)"},
			{"objectID":"903","title":"Ask HN: Who is hiring? (June 2026)"}
		]}`))
	})

	id, err := c.LatestHiringThread(context.Background())
	if err != nil {
		t.Fatalf("LatestHiringThread: %v", err)
	}
	if id != 902 {
		t.Errorf("id = %d, want 902 (first matching title)", id)
	}
}

func TestLatestHiringThreadNotFound(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":[]}`))
	})
	if _, err := c.LatestHiringThread(context.Background()); err == nil {
		t.Error("got nil error, want error when no hiring thread exists")
	}
}

func TestItemTreeFlattensParentsBeforeChildren(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/items/100" {
			http.NotFound(w, r)
			return
		}
		// The shape Algolia actually returns: the story at the root, with
		// comments nested under "children" and nullable author/text.
		w.Write([]byte(`{
			"id":100,"type":"story","title":"story","parent_id":null,
			"author":"op","text":null,"created_at_i":1000,
			"children":[
				{"id":1,"type":"comment","parent_id":100,"author":"a","text":"first","created_at_i":1001,"children":[
					{"id":3,"type":"comment","parent_id":1,"author":"c","text":"reply","created_at_i":1003,"children":[]}
				]},
				{"id":2,"type":"comment","parent_id":100,"author":"b","text":"second","created_at_i":1002,"children":[]}
			]}`))
	})

	got, err := c.ItemTree(context.Background(), 100)
	if err != nil {
		t.Fatalf("ItemTree: %v", err)
	}
	// Depth-first with parents ahead of their children, so a single pass
	// can build the tree without ever holding an unattachable node.
	var ids []int
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if want := []int{1, 3, 2}; !equal(ids, want) {
		t.Fatalf("ItemTree order = %v, want %v", ids, want)
	}
	if got[0].Parent != 100 || got[1].Parent != 1 {
		t.Errorf("parents = %d, %d; want 100, 1", got[0].Parent, got[1].Parent)
	}
	if got[0].By != "a" || got[0].Text != "first" || got[0].Time != 1001 {
		t.Errorf("first comment = %+v, want author a / text first / time 1001", got[0])
	}
	if len(got[0].Kids) != 1 || got[0].Kids[0] != 3 {
		t.Errorf("kids of comment 1 = %v, want [3]", got[0].Kids)
	}
	for _, it := range got {
		if it.ID == 100 {
			t.Error("ItemTree included the story root, which is not a comment")
		}
	}
}

func TestItemTreeMarksStrippedCommentsDeleted(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":100,"type":"story","children":[
			{"id":1,"type":"comment","parent_id":100,"author":null,"text":null,"created_at_i":1,"children":[
				{"id":2,"type":"comment","parent_id":1,"author":"a","text":"kept","created_at_i":2,"children":[]}
			]}
		]}`))
	})

	got, err := c.ItemTree(context.Background(), 100)
	if err != nil {
		t.Fatalf("ItemTree: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if !got[0].Deleted {
		t.Error("comment with null author and text should be marked deleted")
	}
	if got[1].Deleted {
		t.Error("comment with author and text should not be marked deleted")
	}
}

func TestItemTreeMissingItem(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	})
	if _, err := c.ItemTree(context.Background(), 100); err == nil {
		t.Error("ItemTree for an unknown item: got nil error, want error")
	}
}

// TestCanonicalURL pins what counts as the same page. The pairs that must
// match are the ones HN actually produces when the same page is submitted
// years apart; the pairs that must not are the ones where merging them
// would show a discussion of something else.
func TestCanonicalURL(t *testing.T) {
	same := [][2]string{
		{"http://example.com/post", "https://example.com/post"},
		{"https://www.example.com/post", "https://example.com/post"},
		{"https://EXAMPLE.com/post", "https://example.com/post"},
		{"https://example.com/post/", "https://example.com/post"},
		{"https://example.com/post#section", "https://example.com/post"},
		{"https://example.com/post?utm_source=hn&utm_medium=web", "https://example.com/post"},
		{"https://example.com/post?fbclid=abc", "https://example.com/post"},
		{"https://example.com/post?id=7&utm_campaign=x", "https://example.com/post?id=7"},
		{"https://example.com/post?a=1&b=2", "https://example.com/post?b=2&a=1"},
		{"https://example.com:443/post", "https://example.com/post"},
		{"http://example.com:80/post", "https://example.com/post"},
		{"  https://example.com/post  ", "https://example.com/post"},
		{"http://WWW.Example.COM/post/?utm_source=x#top", "https://example.com/post"},
	}
	for _, p := range same {
		if a, b := canonicalURL(p[0]), canonicalURL(p[1]); a != b {
			t.Errorf("canonicalURL(%q) = %q, canonicalURL(%q) = %q, want the same", p[0], a, p[1], b)
		}
	}

	different := [][2]string{
		{"https://example.com/post", "https://example.com/other"},
		{"https://example.com/post", "https://example.org/post"},
		// Paths are case-sensitive; only the host is not.
		{"https://example.com/Post", "https://example.com/post"},
		// A meaningful parameter is not a tracking one.
		{"https://example.com/post?id=7", "https://example.com/post?id=8"},
		{"https://example.com/post?id=7", "https://example.com/post"},
		// A non-default port is part of the address.
		{"https://example.com:8443/post", "https://example.com/post"},
		// www is only dropped from the front of the host.
		{"https://www.example.com/post", "https://example.com/www.post"},
	}
	for _, p := range different {
		if a, b := canonicalURL(p[0]), canonicalURL(p[1]); a == b {
			t.Errorf("canonicalURL(%q) and canonicalURL(%q) both = %q, want different", p[0], p[1], a)
		}
	}
}

// TestCanonicalURLUnparseable: a hit that is not an absolute URL falls back
// to the exact comparison rather than collapsing to an empty key, which
// would make every such hit match every other.
func TestCanonicalURLUnparseable(t *testing.T) {
	for _, s := range []string{"", "not a url", "item?id=123", "/relative/path"} {
		if got := canonicalURL(s); got != strings.TrimRight(strings.TrimSpace(s), "/") {
			t.Errorf("canonicalURL(%q) = %q, want it left alone", s, got)
		}
	}
	// And two different unparseable strings stay different.
	if canonicalURL("not a url") == canonicalURL("also not a url") {
		t.Error("two unparseable URLs collapsed to the same key")
	}
}

// TestPastDiscussionsMatchesURLVariants is the bug in the issue: genuine
// earlier submissions were dropped whenever they differed from the current
// one in any way but a trailing slash.
func TestPastDiscussionsMatchesURLVariants(t *testing.T) {
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"hits":[
			{"objectID":"100","title":"current","url":"https://www.example.com/post","points":10,"num_comments":5,"created_at_i":1000},
			{"objectID":"200","title":"http, years ago","url":"http://example.com/post","points":300,"num_comments":450,"created_at_i":2000},
			{"objectID":"300","title":"with www","url":"https://www.example.com/post","points":50,"num_comments":300,"created_at_i":3000},
			{"objectID":"400","title":"with tracking","url":"https://example.com/post?utm_source=twitter","points":50,"num_comments":200,"created_at_i":4000},
			{"objectID":"500","title":"with fragment","url":"https://example.com/post#comments","points":50,"num_comments":100,"created_at_i":5000},
			{"objectID":"600","title":"host case","url":"https://EXAMPLE.com/post/","points":50,"num_comments":50,"created_at_i":6000},
			{"objectID":"700","title":"a different page","url":"https://example.com/other","points":900,"num_comments":900,"created_at_i":7000}
		]}`))
	})

	got, err := c.PastDiscussions(context.Background(), "https://www.example.com/post", 100)
	if err != nil {
		t.Fatalf("PastDiscussions: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d discussions, want 5: %+v", len(got), got)
	}
	// Most-commented first, and the current story and other page are gone.
	want := []int{200, 300, 400, 500, 600}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("result %d = %d, want %d (full: %+v)", i, got[i].ID, id, got)
		}
	}
}

// TestPastDiscussionsAsksForADeepPage: the exact matches are filtered out of
// a fuzzy result, so the page has to be deep enough that a heavily
// resubmitted URL does not lose its own submissions to near misses.
func TestPastDiscussionsAsksForADeepPage(t *testing.T) {
	var got string
	c := newAlgoliaClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("hitsPerPage")
		w.Write([]byte(`{"hits":[]}`))
	})
	if _, err := c.PastDiscussions(context.Background(), "https://example.com/x", 1); err != nil {
		t.Fatalf("PastDiscussions: %v", err)
	}
	n, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("hitsPerPage = %q: %v", got, err)
	}
	if n < 50 {
		t.Errorf("hitsPerPage = %d, want at least 50", n)
	}
}
