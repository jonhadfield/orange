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
