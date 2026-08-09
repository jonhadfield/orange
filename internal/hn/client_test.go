package hn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestServer(t *testing.T, feeds map[Feed][]int, items map[int]Item, itemHits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json")
		if ids, ok := feeds[Feed(name)]; ok {
			if err := json.NewEncoder(w).Encode(ids); err != nil {
				t.Errorf("encode feed: %v", err)
			}
			return
		}
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/item/%d.json", &id); err == nil {
			if itemHits != nil {
				itemHits.Add(1)
			}
			if it, ok := items[id]; ok {
				if err := json.NewEncoder(w).Encode(it); err != nil {
					t.Errorf("encode item: %v", err)
				}
			} else {
				fmt.Fprint(w, "null")
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFeedIDs(t *testing.T) {
	srv := newTestServer(t, map[Feed][]int{FeedTop: {3, 1, 2}}, nil, nil)
	c := NewClient(srv.URL)

	ids, err := c.FeedIDs(context.Background(), FeedTop)
	if err != nil {
		t.Fatalf("FeedIDs: %v", err)
	}
	if want := []int{3, 1, 2}; !equal(ids, want) {
		t.Errorf("FeedIDs = %v, want %v", ids, want)
	}
}

func TestItemsPreservesOrder(t *testing.T) {
	items := map[int]Item{
		1: {ID: 1, Type: "story", Title: "one"},
		2: {ID: 2, Type: "story", Title: "two"},
		3: {ID: 3, Type: "story", Title: "three"},
	}
	srv := newTestServer(t, nil, items, nil)
	c := NewClient(srv.URL)

	got, err := c.Items(context.Background(), []int{3, 1, 2})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	var titles []string
	for _, it := range got {
		titles = append(titles, it.Title)
	}
	if want := "three,one,two"; strings.Join(titles, ",") != want {
		t.Errorf("Items order = %v, want %s", titles, want)
	}
}

func TestItemsReportsPartialFailure(t *testing.T) {
	items := map[int]Item{
		1: {ID: 1, Type: "story", Title: "one"},
		3: {ID: 3, Type: "story", Title: "three"},
	}
	srv := newTestServer(t, nil, items, nil)
	c := NewClient(srv.URL)

	got, err := c.Items(context.Background(), []int{1, 2, 3})
	// The items that did load are still returned, but the caller must be
	// able to tell that one went missing rather than silently losing it.
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("Items = %+v, want items 1 and 3", got)
	}
	var partial *PartialError
	if !errors.As(err, &partial) {
		t.Fatalf("Items error = %v, want *PartialError", err)
	}
	if partial.Fetched != 2 || partial.Requested != 3 {
		t.Errorf("PartialError = %d of %d, want 2 of 3", partial.Fetched, partial.Requested)
	}
}

func TestItemsAllPresentReportsNoError(t *testing.T) {
	items := map[int]Item{1: {ID: 1, Type: "story"}, 2: {ID: 2, Type: "story"}}
	srv := newTestServer(t, nil, items, nil)
	c := NewClient(srv.URL)

	if _, err := c.Items(context.Background(), []int{1, 2}); err != nil {
		t.Errorf("Items with everything present: got %v, want nil", err)
	}
}

func TestSendsUserAgent(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- r.Header.Get("User-Agent"):
		default:
		}
		fmt.Fprint(w, "[]")
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, WithUserAgent("orange/1.2.3"))

	if _, err := c.FeedIDs(context.Background(), FeedTop); err != nil {
		t.Fatalf("FeedIDs: %v", err)
	}
	if ua := <-got; ua != "orange/1.2.3" {
		t.Errorf("User-Agent = %q, want %q", ua, "orange/1.2.3")
	}
}

func TestRetriesServerErrors(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "[1,2]")
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	c.retryBase = 0

	ids, err := c.FeedIDs(context.Background(), FeedTop)
	if err != nil {
		t.Fatalf("FeedIDs after transient 500s: %v", err)
	}
	if want := []int{1, 2}; !equal(ids, want) {
		t.Errorf("FeedIDs = %v, want %v", ids, want)
	}
	if hits.Load() != 3 {
		t.Errorf("server hits = %d, want 3 (two failures then success)", hits.Load())
	}
}

func TestDoesNotRetryNotFound(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	c.retryBase = 0

	if _, err := c.FeedIDs(context.Background(), FeedTop); err == nil {
		t.Error("FeedIDs against 404: got nil error, want error")
	}
	if hits.Load() != 1 {
		t.Errorf("server hits = %d, want 1 (404 is not transient)", hits.Load())
	}
}

func TestItemsAllFailedReturnsError(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil)
	c := NewClient(srv.URL)

	if _, err := c.Items(context.Background(), []int{7, 8}); err == nil {
		t.Error("Items with no fetchable items: got nil error, want error")
	}
}

func TestItemCaching(t *testing.T) {
	var hits atomic.Int64
	items := map[int]Item{1: {ID: 1, Type: "story", Title: "one"}}
	srv := newTestServer(t, nil, items, &hits)
	c := NewClient(srv.URL)

	for range 3 {
		if _, err := c.Item(context.Background(), 1); err != nil {
			t.Fatalf("Item: %v", err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("server item hits = %d, want 1 (cache miss only once)", hits.Load())
	}
}

func TestServerErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	c.retryBase = 0 // exhaust the retries without the backoff wait

	if _, err := c.FeedIDs(context.Background(), FeedTop); err == nil {
		t.Error("FeedIDs against 500 server: got nil error, want error")
	}
	if _, err := c.Item(context.Background(), 1); err == nil {
		t.Error("Item against 500 server: got nil error, want error")
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
