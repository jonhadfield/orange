package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	watching, err := s.Toggle(42, "A story", 10, 1000)
	if err != nil || !watching {
		t.Fatalf("Toggle on = (%v, %v), want (true, nil)", watching, err)
	}
	if !s.IsWatched(42) {
		t.Error("IsWatched(42) = false after watching")
	}

	// Reopen from disk: state must survive.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	ws, ok := s2.Get(42)
	if !ok || ws.Title != "A story" || ws.LastComments != 10 || ws.LastReadAt != 1000 {
		t.Errorf("persisted state = %+v, %v", ws, ok)
	}

	watching, err = s2.Toggle(42, "A story", 10, 2000)
	if err != nil || watching {
		t.Fatalf("Toggle off = (%v, %v), want (false, nil)", watching, err)
	}
	if s2.IsWatched(42) {
		t.Error("IsWatched(42) = true after unwatching")
	}
}

func TestMarkRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// No-op for unwatched stories.
	if err := s.MarkRead(1, 5, 100); err != nil {
		t.Fatalf("MarkRead unwatched: %v", err)
	}
	if _, ok := s.Get(1); ok {
		t.Error("MarkRead created state for an unwatched story")
	}

	if _, err := s.Toggle(1, "t", 5, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(1, 25, 200); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	ws, _ := s.Get(1)
	if ws.LastComments != 25 || ws.LastReadAt != 200 {
		t.Errorf("after MarkRead = %+v, want comments 25, readAt 200", ws)
	}
}

func TestAllSortedByLastRead(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "w.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Toggle(1, "old", 0, 100)
	s.Toggle(2, "new", 0, 300)
	s.Toggle(3, "mid", 0, 200)

	all := s.All()
	if len(all) != 3 || all[0].ID != 2 || all[1].ID != 3 || all[2].ID != 1 {
		t.Errorf("All() order = %+v, want IDs [2 3 1]", all)
	}
}

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := range 5 {
		if _, err := s.Toggle(i, "story", 0, int64(i)); err != nil {
			t.Fatalf("Toggle: %v", err)
		}
	}

	// The rename-based write must not leave scratch files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "watched.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory contains %v, want only watched.json", names)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("mode = %o, want 644", perm)
	}

	// Whatever landed on disk must be complete, not a partial write.
	if _, err := Open(path); err != nil {
		t.Errorf("reopen after repeated saves: %v", err)
	}
}

func TestSaveReplacesExistingFileInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte(`{"7":{"id":7,"title":"stale"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.Toggle(7, "", 0, 0); err != nil { // removes it
		t.Fatalf("Toggle: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.IsWatched(7) {
		t.Error("story 7 still watched after removal was saved")
	}
}
