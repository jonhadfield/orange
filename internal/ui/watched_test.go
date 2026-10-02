package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

func TestWatchedKeepsItemsOnPartialFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "one", 2, 100)
	st.Toggle(2, "two", 3, 100)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m, _ = m.start()
	m, _ = m.Update(watchedDataMsg{
		items: []hn.Item{
			{ID: 1, Type: "story", Title: "one", Descendants: 5, Score: 10},
		},
		err: &hn.PartialError{Fetched: 1, Requested: 2, Err: errors.New("timeout")},
	})

	if m.err != nil {
		t.Fatalf("partial failure became hard error: %v", m.err)
	}
	if m.warn == "" {
		t.Fatal("partial failure produced no warning")
	}
	if len(m.rows) != 1 || m.rows[0].item.ID != 1 {
		t.Fatalf("rows = %+v, want the story that arrived", m.rows)
	}
	if view := stripStyles(m.View()); !strings.Contains(view, "one") {
		t.Errorf("partial load hid the arrived story:\n%s", view)
	}
}

func TestWatchedTotalFailureShowsRetryAdvice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "one", 0, 1)

	m := newWatchedModel(nil, st, newKeyMap())
	m.setSize(100, 24)
	m, _ = m.start()
	m, _ = m.Update(watchedDataMsg{err: errors.New("dial failed")})

	if m.err == nil {
		t.Fatal("total failure was not recorded")
	}
	view := stripStyles(m.View())
	if !strings.Contains(view, "press r") {
		t.Errorf("failure view does not name the retry key:\n%s", view)
	}
}
