package hn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the official Hacker News Firebase API endpoint.
const DefaultBaseURL = "https://hacker-news.firebaseio.com/v0"

const fetchWorkers = 8

// Client fetches feeds and items from the Hacker News API, caching items
// in memory for the lifetime of the process.
type Client struct {
	baseURL    string
	algoliaURL string
	httpClient *http.Client

	mu    sync.Mutex
	cache map[int]Item
}

// NewClient returns a Client for the given base URL, or the official API
// when baseURL is empty.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		algoliaURL: DefaultAlgoliaURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		cache:      make(map[int]Item),
	}
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	return c.getURL(ctx, c.baseURL+path, v)
}

func (c *Client) getURL(ctx context.Context, full string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hn: GET %s: %s", full, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// FeedIDs returns the item IDs for the given feed.
func (c *Client) FeedIDs(ctx context.Context, feed Feed) ([]int, error) {
	var ids []int
	if err := c.get(ctx, "/"+string(feed)+".json", &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// Item returns a single item, served from cache when available.
func (c *Client) Item(ctx context.Context, id int) (Item, error) {
	return c.item(ctx, id, false)
}

func (c *Client) item(ctx context.Context, id int, fresh bool) (Item, error) {
	if !fresh {
		c.mu.Lock()
		if it, ok := c.cache[id]; ok {
			c.mu.Unlock()
			return it, nil
		}
		c.mu.Unlock()
	}

	var it Item
	if err := c.get(ctx, fmt.Sprintf("/item/%d.json", id), &it); err != nil {
		return Item{}, err
	}
	// The API returns the JSON literal "null" for unknown IDs, which
	// decodes to the zero Item.
	if it.ID == 0 {
		return Item{}, fmt.Errorf("hn: item %d not found", id)
	}
	c.mu.Lock()
	c.cache[id] = it
	c.mu.Unlock()
	return it, nil
}

// Items fetches the given items concurrently, preserving the order of ids.
// Individual items that fail to load are skipped; an error is returned only
// when nothing could be fetched at all.
func (c *Client) Items(ctx context.Context, ids []int) ([]Item, error) {
	return c.items(ctx, ids, false)
}

// ItemsFresh is Items but always refetches, updating the cache — used where
// live scores and comment counts matter.
func (c *Client) ItemsFresh(ctx context.Context, ids []int) ([]Item, error) {
	return c.items(ctx, ids, true)
}

func (c *Client) items(ctx context.Context, ids []int, fresh bool) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	results := make([]*Item, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, fetchWorkers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			it, err := c.item(ctx, id, fresh)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = &it
		}(i, id)
	}
	wg.Wait()

	items := make([]Item, 0, len(ids))
	var firstErr error
	for i := range results {
		if results[i] != nil {
			items = append(items, *results[i])
			continue
		}
		if firstErr == nil {
			firstErr = errs[i]
		}
	}
	if len(items) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return items, nil
}
