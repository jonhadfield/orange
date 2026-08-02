package hn

import (
	"context"
	"encoding/json"
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

func TestItemsSkipsMissing(t *testing.T) {
	items := map[int]Item{
		1: {ID: 1, Type: "story", Title: "one"},
		3: {ID: 3, Type: "story", Title: "three"},
	}
	srv := newTestServer(t, nil, items, nil)
	c := NewClient(srv.URL)

	got, err := c.Items(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("Items = %+v, want items 1 and 3", got)
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
