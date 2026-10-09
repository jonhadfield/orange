package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

func TestStickyNoticeSurvivesNavigationKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "t", 0, 1)

	m := New(hn.NewClient("http://unused.invalid"), st)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	next, _ = m.Update(storeErrMsg{err: errors.New("disk full")})
	m = next.(Model)
	if !m.noticeSticky || !strings.Contains(m.notice, "not saved") {
		t.Fatalf("sticky save notice missing: sticky=%v notice=%q", m.noticeSticky, m.notice)
	}

	next, _ = m.Update(keyPress("j"))
	m = next.(Model)
	if !strings.Contains(m.notice, "not saved") {
		t.Errorf("sticky notice cleared by j: %q", m.notice)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.notice != "" || m.noticeSticky {
		t.Errorf("esc left notice behind: sticky=%v notice=%q", m.noticeSticky, m.notice)
	}
}

func TestEphemeralNoticeClearsOnNextKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	m := New(hn.NewClient("http://unused.invalid"), st)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m.feeds.state().items = []hn.Item{{ID: 1, Type: "story", Title: "Watch me"}}

	next, _ = m.Update(keyPress("w"))
	m = next.(Model)
	if m.noticeSticky || !strings.Contains(m.notice, "watching") {
		t.Fatalf("ephemeral watch notice missing: sticky=%v notice=%q", m.noticeSticky, m.notice)
	}

	next, _ = m.Update(keyPress("j"))
	m = next.(Model)
	if m.notice != "" {
		t.Errorf("ephemeral notice survived j: %q", m.notice)
	}
}

func TestRecoveredStoreNoticeIsSticky(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	m := New(hn.NewClient("http://unused.invalid"), st)
	if !m.noticeSticky {
		t.Fatal("recovered-store notice should be sticky")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	next, _ = m.Update(keyPress("j"))
	m = next.(Model)
	if !strings.Contains(m.notice, "unreadable") {
		t.Errorf("recovered notice cleared by j: %q", m.notice)
	}
}
