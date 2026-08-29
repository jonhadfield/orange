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
		// The URL search is fuzzy and the exact matches are filtered out
		// of it below, so a heavily resubmitted page needs a page deep
		// enough that its own submissions are not pushed off the end by
		// near misses.
		"hitsPerPage": {"50"},
	}
	var resp algoliaResponse
	if err := c.getURL(ctx, c.algoliaURL+"/search?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	want := canonicalURL(storyURL)
	var out []PastDiscussion
	for _, h := range resp.Hits {
		id, err := strconv.Atoi(h.ObjectID)
		if err != nil || id == excludeID {
			continue
		}
		// The URL attribute search is fuzzy; keep the ones pointing at the
		// same page.
		if canonicalURL(h.URL) != want {
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

// trackingParams are query parameters that record where a click came from
// rather than what is being linked to. The list is deliberately short:
// dropping a parameter that does carry meaning would merge two genuinely
// different pages into one discussion, which is a worse failure than the
// missed match this is here to fix.
var trackingParams = map[string]bool{
	"fbclid":  true,
	"gclid":   true,
	"dclid":   true,
	"msclkid": true,
	"yclid":   true,
	"igshid":  true,
	"mc_cid":  true,
	"mc_eid":  true,
	"ref_src": true,
}

// canonicalURL reduces a submission URL to a key for deciding whether two
// submissions point at the same page. HN's own submissions of one page
// differ in ways that say nothing about the target — http against https,
// www against the bare host, a tracking parameter picked up on the way, a
// fragment — and this feature exists to surface submissions made years
// apart, which is exactly when those differences show up.
//
// The scheme is dropped rather than compared, which is what makes http and
// https equivalent.
func canonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Not an absolute URL, so there is nothing to take apart. Compare
		// it as it stands, as the previous exact match did.
		return strings.TrimRight(raw, "/")
	}

	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	// A default port says nothing either.
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		host += ":" + port
	}

	// The path keeps its case: only the host is case-insensitive.
	key := host + strings.TrimRight(u.Path, "/")
	if q := canonicalQuery(u.Query()); q != "" {
		key += "?" + q
	}
	// The fragment is left out entirely: it selects a place within the
	// page, not a different page.
	return key
}

// canonicalQuery drops tracking parameters and puts the rest in a fixed
// order, so two submissions differing only in parameter order still match.
func canonicalQuery(q url.Values) string {
	for k := range q {
		if trackingParams[strings.ToLower(k)] || strings.HasPrefix(strings.ToLower(k), "utm_") {
			delete(q, k)
		}
	}
	// Encode sorts by key, and the values within each key are sorted here
	// so that ?a=2&a=1 and ?a=1&a=2 agree.
	for _, vs := range q {
		sort.Strings(vs)
	}
	return q.Encode()
}

// algoliaNode is one item in Algolia's nested item tree. Absent fields come
// back as JSON null, so the nullable ones are pointers.
type algoliaNode struct {
	ID         int           `json:"id"`
	Type       string        `json:"type"`
	Author     *string       `json:"author"`
	Text       *string       `json:"text"`
	ParentID   *int          `json:"parent_id"`
	CreatedAtI int64         `json:"created_at_i"`
	Children   []algoliaNode `json:"children"`
}

// ItemTree returns every comment on a story in a single request, flattened
// depth-first with parents ahead of their children so callers can build the
// tree in one pass.
//
// This is a fast path, not a complete one: Algolia omits dead and deleted
// comments along with their entire subtrees, and its index trails the live
// site by minutes. Callers that need every comment must reconcile against the
// Firebase API afterwards.
func (c *Client) ItemTree(ctx context.Context, id int) ([]Item, error) {
	var root algoliaNode
	if err := c.getURL(ctx, fmt.Sprintf("%s/items/%d", c.algoliaURL, id), &root); err != nil {
		return nil, err
	}
	if root.ID == 0 {
		return nil, fmt.Errorf("hn: item %d not found", id)
	}
	var out []Item
	var walk func(nodes []algoliaNode)
	walk = func(nodes []algoliaNode) {
		for _, n := range nodes {
			out = append(out, n.item())
			walk(n.Children)
		}
	}
	walk(root.Children)
	return out, nil
}

// item converts an Algolia node to the Item shape used across the app.
func (n algoliaNode) item() Item {
	it := Item{
		ID:   n.ID,
		Type: n.Type,
		Time: n.CreatedAtI,
	}
	if n.Author != nil {
		it.By = *n.Author
	}
	if n.Text != nil {
		it.Text = *n.Text
	}
	if n.ParentID != nil {
		it.Parent = *n.ParentID
	}
	// Algolia normally drops removed comments entirely, but when it does
	// surface one it arrives stripped of both author and text.
	if n.Author == nil && n.Text == nil {
		it.Deleted = true
	}
	for _, ch := range n.Children {
		it.Kids = append(it.Kids, ch.ID)
	}
	return it
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
