package hn

import (
	"context"
	"net/http"
	"net/http/httptest"
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
