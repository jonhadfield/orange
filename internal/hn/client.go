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

// DefaultUserAgent identifies orange to the API. Callers should override it
// with the running version via WithUserAgent.
const DefaultUserAgent = "orange (+https://github.com/jonhadfield/orange)"

const (
	fetchWorkers = 8
	maxAttempts  = 3
)

// PartialError reports that some — but not all — of a batch of items failed to
// load. The items that did arrive are still returned alongside it, so callers
// can show them rather than treating the whole batch as lost.
type PartialError struct {
	Fetched   int
	Requested int
	Err       error
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("hn: loaded %d of %d items: %v", e.Fetched, e.Requested, e.Err)
}

func (e *PartialError) Unwrap() error { return e.Err }

// Client fetches feeds and items from the Hacker News API, caching items
// in memory for the lifetime of the process.
type Client struct {
	baseURL    string
	algoliaURL string
	userAgent  string
	retryBase  time.Duration
	httpClient *http.Client

	mu    sync.Mutex
	cache map[int]Item
}

// Option configures a Client.
type Option func(*Client)

// WithUserAgent sets the User-Agent sent with every request.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// NewClient returns a Client for the given base URL, or the official API
// when baseURL is empty.
func NewClient(baseURL string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	// The default MaxIdleConnsPerHost of 2 throttles the fetchWorkers
	// goroutines, which all target the same host.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = fetchWorkers
	c := &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		algoliaURL: DefaultAlgoliaURL,
		userAgent:  DefaultUserAgent,
		retryBase:  100 * time.Millisecond,
		httpClient: &http.Client{Timeout: 15 * time.Second, Transport: tr},
		cache:      make(map[int]Item),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	return c.getURL(ctx, c.baseURL+path, v)
}

// getURL fetches and decodes full, retrying transient failures.
func (c *Client) getURL(ctx context.Context, full string, v any) error {
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			if err := c.wait(ctx, attempt); err != nil {
				return err
			}
		}
		retryable, err := c.doGet(ctx, full, v)
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// doGet performs one request, reporting whether a failure is worth retrying.
func (c *Client) doGet(ctx context.Context, full string, v any) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// A cancelled or expired context is deliberate, never transient.
		return ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		transient := resp.StatusCode >= http.StatusInternalServerError ||
			resp.StatusCode == http.StatusTooManyRequests
		return transient, fmt.Errorf("hn: GET %s: %s", full, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return false, err
	}
	return false, nil
}

// wait sleeps for the exponential backoff before the given attempt, or
// returns early if the context is done.
func (c *Client) wait(ctx context.Context, attempt int) error {
	if c.retryBase <= 0 {
		return nil
	}
	t := time.NewTimer(c.retryBase << (attempt - 1))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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
// When some items fail the rest are still returned, with a *PartialError; an
// error on its own means nothing could be fetched at all.
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
		// Acquire before spawning, so a large batch doesn't create one
		// goroutine per ID up front just to have them all block.
		sem <- struct{}{}
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
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
	failed := 0
	for i := range results {
		if results[i] != nil {
			items = append(items, *results[i])
			continue
		}
		failed++
		if firstErr == nil {
			firstErr = errs[i]
		}
	}
	switch {
	case failed == 0:
		return items, nil
	case len(items) == 0:
		return nil, firstErr
	default:
		return items, &PartialError{Fetched: len(items), Requested: len(ids), Err: firstErr}
	}
}
