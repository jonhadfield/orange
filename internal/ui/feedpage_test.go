package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

// TestNextPageFollowsAPartialPage: when some stories on a page fail to load,
// the next page has to start after the last ID that was asked for, not after
// the number of stories that arrived. Counting arrivals re-requests the tail
// of the previous page, so those stories are listed twice, and every rank
// after the gap is shifted.
func TestNextPageFollowsAPartialPage(t *testing.T) {
	missing := map[int]bool{3: true, 7: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/item/%d.json", &id); err != nil {
			http.NotFound(w, r)
			return
		}
		if missing[id] {
			// What the API returns for an item it cannot serve.
			fmt.Fprint(w, "null")
			return
		}
		fmt.Fprintf(w, `{"id":%d,"type":"story","title":"story %d","by":"someone"}`, id, id)
	}))
	defer srv.Close()

	m := newFeedsModel(hn.NewClient(srv.URL), newKeyMap())
	m.setSize(100, 24)
	feed := m.feed()

	ids := make([]int, 2*storyPageSize)
	for i := range ids {
		ids[i] = i + 1
	}
	m, cmd := m.Update(feedIDsMsg{feed: feed, ids: ids})
	if cmd == nil {
		t.Fatal("the first page was not requested")
	}
	m, _ = m.Update(cmd())
	if got, want := len(m.state().items), storyPageSize-len(missing); got != want {
		t.Fatalf("first page has %d stories, want %d", got, want)
	}

	cmd = m.loadMore(feed)
	if cmd == nil {
		t.Fatal("the second page was not requested")
	}
	m, _ = m.Update(cmd())

	st := m.state()
	seen := map[int]int{}
	for _, it := range st.items {
		seen[it.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("story %d is listed %d times", id, n)
		}
	}
	if got, want := len(st.items), len(ids)-len(missing); got != want {
		t.Errorf("%d stories loaded across two pages, want %d", got, want)
	}

	// Ranks are positions in the feed, so the gaps left by the missing
	// stories stay gaps instead of renumbering everything below them.
	view := stripStyles(m.View())
	if !strings.Contains(view, "  8. ▲ 0    story 8") {
		t.Errorf("story 8 is not ranked 8:\n%s", view)
	}
}
