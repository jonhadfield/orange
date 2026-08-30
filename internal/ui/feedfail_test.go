package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

// failedFeeds returns a feed list whose current feed failed to load, which
// is what a reader sees when the network is down.
func failedFeeds(t *testing.T, err error) feedsModel {
	t.Helper()
	m := newFeedsModel(nil, newKeyMap())
	m.setSize(100, 24)
	// Start a real load first, so the failure interrupts something. The
	// command is discarded rather than run, so nothing reaches the network.
	if cmd := m.loadFeed(m.feed()); cmd == nil {
		t.Fatal("the feed did not start loading")
	}
	if !m.state().loading {
		t.Fatal("loadFeed did not mark the feed as loading")
	}
	m, _ = m.Update(feedIDsMsg{feed: m.feed(), err: err})
	if m.state().err == nil {
		t.Fatal("the feed did not record the failure")
	}
	return m
}

// TestFeedFailureIsShown: the error screen is the least exercised in the
// program and the one a reader meets when their connection drops. It has to
// say what happened, why, and what to do about it.
func TestFeedFailureIsShown(t *testing.T) {
	m := failedFeeds(t, errors.New("dial tcp: no such host"))
	view := stripStyles(m.View())

	if !strings.Contains(view, "could not load") {
		t.Errorf("the view does not say the load failed:\n%s", view)
	}
	// The underlying error, or there is nothing to diagnose.
	if !strings.Contains(view, "dial tcp: no such host") {
		t.Errorf("the view does not show the reason:\n%s", view)
	}
	// And a way out. r reloads this feed; tab leaves it.
	if !strings.Contains(view, "press r") {
		t.Errorf("the view does not name the key that retries:\n%s", view)
	}
	// The spinner must not still be running underneath the error.
	if m.state().loading {
		t.Error("the feed is still marked as loading after it failed")
	}
}

// TestFeedFailureAdviceMatchesTheKeyThatWorks guards the wording against the
// key it names: whatever the message tells the reader to press has to be a
// key this view actually handles.
func TestFeedFailureAdviceMatchesTheKeyThatWorks(t *testing.T) {
	m := failedFeeds(t, errors.New("boom"))
	view := stripStyles(m.View())

	if strings.Contains(view, "press tab to retry") {
		t.Error("the message still sends the reader to tab, which leaves this feed")
	}

	// r must actually clear the failure and start a fresh load.
	next, cmd := m.handleKey(keyPress("r"))
	if cmd == nil {
		t.Fatal("r produced no command, so nothing was retried")
	}
	if next.state().err != nil {
		t.Error("r left the error in place")
	}
	if !next.state().loading {
		t.Error("r did not start a new load")
	}
}

// TestFeedRecoversAfterAFailedLoad: the failure has to be recoverable, not
// just displayed. A retry that succeeds returns an ordinary list.
func TestFeedRecoversAfterAFailedLoad(t *testing.T) {
	m := failedFeeds(t, errors.New("temporary failure"))

	m, _ = m.handleKey(keyPress("r"))
	feed := m.feed()
	m, _ = m.Update(feedIDsMsg{feed: feed, ids: []int{1, 2}})
	m, _ = m.Update(feedItemsMsg{feed: feed, offset: 0, items: []hn.Item{
		{ID: 1, Type: "story", Title: "First story", By: "someone", Score: 10},
		{ID: 2, Type: "story", Title: "Second story", By: "someone", Score: 20},
	}})

	if err := m.state().err; err != nil {
		t.Errorf("the error survived a successful reload: %v", err)
	}
	view := stripStyles(m.View())
	if strings.Contains(view, "could not load") {
		t.Errorf("the failure message is still on screen after recovery:\n%s", view)
	}
	if !strings.Contains(view, "First story") {
		t.Errorf("the reloaded stories are not shown:\n%s", view)
	}
}

// TestSwitchingAwayAndBackAlsoRetries pins the other route, which the old
// message pointed at: leaving a failed feed and returning clears the error.
// It is a longer way round, but it must keep working.
func TestSwitchingAwayAndBackAlsoRetries(t *testing.T) {
	m := failedFeeds(t, errors.New("boom"))
	failed := m.active

	m, _ = m.handleKey(keyPress("tab"))
	if m.active == failed {
		t.Fatal("tab did not move to another feed")
	}
	m, _ = m.handleKey(keyPress("shift+tab"))
	if m.active != failed {
		t.Fatalf("shift+tab returned to feed %d, want %d", m.active, failed)
	}
	if m.state().err != nil {
		t.Error("returning to the failed feed left the error in place")
	}
}

// TestOneFeedFailingLeavesTheOthersAlone: the feeds hold their state
// separately, so a failure on one must not show up on another.
func TestOneFeedFailingLeavesTheOthersAlone(t *testing.T) {
	m := failedFeeds(t, errors.New("boom"))
	broken := m.feed()

	m, _ = m.handleKey(keyPress("tab"))
	if m.feed() == broken {
		t.Fatal("tab did not move to another feed")
	}
	if m.state().err != nil {
		t.Errorf("a different feed inherited the failure: %v", m.state().err)
	}
	if view := stripStyles(m.View()); strings.Contains(view, "could not load") {
		t.Errorf("the failure message followed the reader to another feed:\n%s", view)
	}
}

// TestItemLoadFailureIsShownToo: the ids can arrive and the items still
// fail, which is a second, separate path to the same screen.
func TestItemLoadFailureIsShownToo(t *testing.T) {
	m := newFeedsModel(nil, newKeyMap())
	m.setSize(100, 24)
	feed := m.feed()

	m, _ = m.Update(feedIDsMsg{feed: feed, ids: []int{1, 2, 3}})
	m, _ = m.Update(feedItemsMsg{feed: feed, offset: 0, err: errors.New("fetch failed")})

	if m.state().err == nil {
		t.Fatal("a failed item fetch was not recorded")
	}
	if view := stripStyles(m.View()); !strings.Contains(view, "fetch failed") {
		t.Errorf("the view does not show the item fetch error:\n%s", view)
	}
}

// TestRefreshReloadsAFeedThatAlreadyHasStories: r has to work on a healthy
// feed too, and that depends on the refresh clearing the state. loadFeed
// returns early when ids are already present, so without the reset r would
// silently do nothing on the one feed a reader is most likely to press it
// on.
func TestRefreshReloadsAFeedThatAlreadyHasStories(t *testing.T) {
	m := newFeedsModel(nil, newKeyMap())
	m.setSize(100, 24)
	feed := m.feed()
	m, _ = m.Update(feedIDsMsg{feed: feed, ids: []int{1, 2}})
	m, _ = m.Update(feedItemsMsg{feed: feed, offset: 0, items: []hn.Item{
		{ID: 1, Type: "story", Title: "Already here", By: "someone"},
	}})
	if len(m.state().items) == 0 {
		t.Fatal("the feed has no stories to refresh")
	}

	next, cmd := m.handleKey(keyPress("r"))
	if cmd == nil {
		t.Fatal("r produced no command on a loaded feed, so nothing was refreshed")
	}
	if !next.state().loading {
		t.Error("r did not mark the feed as loading")
	}
	if n := len(next.state().ids); n != 0 {
		t.Errorf("r left %d stale ids in place, so the reload will be skipped", n)
	}
}
