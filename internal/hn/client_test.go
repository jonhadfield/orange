package hn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// TestCacheEvictsLeastRecentlyUsed: the cache used to keep everything it had
// ever seen, so a reader left running all day grew without limit. It is now
// bounded, and what goes first is what has been left alone longest.
func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewClient("http://unused.invalid")
	c.cacheMax = 3

	for id := 1; id <= 3; id++ {
		c.store(id, Item{ID: id, Type: "story"})
	}
	if got := c.cacheLen(); got != 3 {
		t.Fatalf("cache holds %d, want 3", got)
	}

	// Touching 1 makes 2 the oldest, so adding 4 must drop 2, not 1.
	if _, ok := c.cached(1); !ok {
		t.Fatal("item 1 is not cached")
	}
	c.store(4, Item{ID: 4, Type: "story"})

	if got := c.cacheLen(); got != 3 {
		t.Errorf("cache holds %d after exceeding the limit, want 3", got)
	}
	if _, ok := c.cached(2); ok {
		t.Error("item 2 survived, want the least recently used one evicted")
	}
	for _, id := range []int{1, 3, 4} {
		if _, ok := c.cached(id); !ok {
			t.Errorf("item %d was evicted, want it kept", id)
		}
	}
}

// TestCacheDoesNotGrowOnRepeatedStores: refreshing an item already held has
// to replace it rather than add a second copy, or a feed on a refresh timer
// would fill the cache with one story.
func TestCacheDoesNotGrowOnRepeatedStores(t *testing.T) {
	c := NewClient("http://unused.invalid")
	for range 100 {
		c.store(7, Item{ID: 7, Title: "latest"})
	}
	if got := c.cacheLen(); got != 1 {
		t.Errorf("cache holds %d entries after 100 stores of one item, want 1", got)
	}
	it, ok := c.cached(7)
	if !ok || it.Title != "latest" {
		t.Errorf("cached item = %+v, %v, want the newest version", it, ok)
	}
}

// TestCacheStaysWithinItsLimit is the property the issue is about: however
// much goes in, the cache does not keep growing.
func TestCacheStaysWithinItsLimit(t *testing.T) {
	c := NewClient("http://unused.invalid")
	c.cacheMax = 50
	for id := range 5000 {
		c.store(id, Item{ID: id, Text: "a comment body"})
		if got := c.cacheLen(); got > c.cacheMax {
			t.Fatalf("cache grew to %d, over its limit of %d", got, c.cacheMax)
		}
	}
	if got := c.cacheLen(); got != 50 {
		t.Errorf("cache holds %d at the end, want it full at 50", got)
	}
}

// TestDefaultCacheSizeCoversASession: the cap only exists to bound a very
// long run, so it has to sit well above what one session actually touches —
// six feeds of 500 stories plus several large threads.
func TestDefaultCacheSizeCoversASession(t *testing.T) {
	const feeds, storiesPerFeed, threads, commentsPerThread = 6, 500, 4, 2500
	session := feeds*storiesPerFeed + threads*commentsPerThread
	if defaultCacheSize <= session {
		t.Errorf("defaultCacheSize = %d, want more than a session's %d items",
			defaultCacheSize, session)
	}
}

// TestCacheConcurrentAccess runs under -race: items are fetched on several
// goroutines, so the LRU bookkeeping is reached concurrently.
func TestCacheConcurrentAccess(t *testing.T) {
	c := NewClient("http://unused.invalid")
	c.cacheMax = 20

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				id := g*200 + i
				c.store(id, Item{ID: id})
				c.cached(id)
				c.cached(id / 2)
			}
		}()
	}
	wg.Wait()
	if got := c.cacheLen(); got > 20 {
		t.Errorf("cache holds %d, over its limit of 20", got)
	}
}

// TestItemStringsAreStrippedOfControls: titles and authors are rendered
// straight into the terminal without going near the HTML renderer, so the
// stripping has to happen where the response is decoded, not only there.
func TestItemStringsAreStrippedOfControls(t *testing.T) {
	items := map[int]Item{
		1: {
			ID:    1,
			Type:  "story\x1b",
			By:    "user\x1b[31m",
			Title: "A title\x1b]8;;http://evil\x1b\\ with an escape",
			URL:   "https://example.com/\x07path",
			Text:  "body\x00with\x9bcontrols",
		},
	}
	srv := newTestServer(t, nil, items, nil)
	c := NewClient(srv.URL)

	it, err := c.Item(context.Background(), 1)
	if err != nil {
		t.Fatalf("Item: %v", err)
	}
	for _, f := range []struct{ name, got string }{
		{"Type", it.Type}, {"By", it.By}, {"Title", it.Title},
		{"URL", it.URL}, {"Text", it.Text},
	} {
		for _, r := range f.got {
			if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Errorf("%s = %q, still contains %#U", f.name, f.got, r)
				break
			}
		}
	}
	// The readable content survives.
	if !strings.Contains(it.Title, "A title") || !strings.Contains(it.By, "user") {
		t.Errorf("stripping removed more than the controls: title=%q by=%q", it.Title, it.By)
	}
}
