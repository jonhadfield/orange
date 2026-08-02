package hn

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// DefaultAlgoliaURL is the HN Algolia search API endpoint.
const DefaultAlgoliaURL = "https://hn.algolia.com/api/v1"

// PastDiscussion is an earlier HN submission of the same URL.
type PastDiscussion struct {
	ID       int
	Title    string
	Points   int
	Comments int
	Time     int64
}

type algoliaHit struct {
	ObjectID    string `json:"objectID"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Points      int    `json:"points"`
	NumComments int    `json:"num_comments"`
	CreatedAtI  int64  `json:"created_at_i"`
}

type algoliaResponse struct {
	Hits []algoliaHit `json:"hits"`
}

// PastDiscussions returns earlier submissions of storyURL, excluding the
// item excludeID, most-commented first (at most five).
func (c *Client) PastDiscussions(ctx context.Context, storyURL string, excludeID int) ([]PastDiscussion, error) {
	q := url.Values{
		"query":                        {storyURL},
		"restrictSearchableAttributes": {"url"},
		"tags":                         {"story"},
		"hitsPerPage":                  {"10"},
	}
	var resp algoliaResponse
	if err := c.getURL(ctx, c.algoliaURL+"/search?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	norm := func(s string) string { return strings.TrimRight(s, "/") }
	var out []PastDiscussion
	for _, h := range resp.Hits {
		id, err := strconv.Atoi(h.ObjectID)
		if err != nil || id == excludeID {
			continue
		}
		// The URL attribute search is fuzzy; keep exact matches only.
		if norm(h.URL) != norm(storyURL) {
			continue
		}
		out = append(out, PastDiscussion{
			ID:       id,
			Title:    h.Title,
			Points:   h.Points,
			Comments: h.NumComments,
			Time:     h.CreatedAtI,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Comments > out[j].Comments })
	if len(out) > 5 {
		out = out[:5]
	}
	return out, nil
}

// LatestHiringThread returns the item ID of the most recent monthly
// "Ask HN: Who is hiring?" thread.
func (c *Client) LatestHiringThread(ctx context.Context) (int, error) {
	q := url.Values{
		"tags":        {"story,author_whoishiring"},
		"hitsPerPage": {"10"},
	}
	var resp algoliaResponse
	if err := c.getURL(ctx, c.algoliaURL+"/search_by_date?"+q.Encode(), &resp); err != nil {
		return 0, err
	}
	for _, h := range resp.Hits {
		if strings.Contains(h.Title, "Who is hiring?") {
			return strconv.Atoi(h.ObjectID)
		}
	}
	return 0, fmt.Errorf("hn: no hiring thread found")
}
