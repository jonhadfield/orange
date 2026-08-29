package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestToggleAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if watching := s.Toggle(42, "A story", 10, 1000); !watching {
		t.Fatal("Toggle on = false, want true")
	}
	if !s.IsWatched(42) {
		t.Error("IsWatched(42) = false after watching")
	}
	// The change is in memory until Save; persisting is now a separate step.
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
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

	if watching := s2.Toggle(42, "A story", 10, 2000); watching {
		t.Fatal("Toggle off = true, want false")
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
	s.MarkRead(1, 5, 100)
	if _, ok := s.Get(1); ok {
		t.Error("MarkRead created state for an unwatched story")
	}

	s.Toggle(1, "t", 5, 100)
	s.MarkRead(1, 25, 200)
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
		s.Toggle(i, "story", 0, int64(i))
		if err := s.Save(); err != nil {
			t.Fatalf("Save: %v", err)
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
	s.Toggle(7, "", 0, 0) // removes it
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.IsWatched(7) {
		t.Error("story 7 still watched after removal was saved")
	}
}

// TestOpenSetsAsideCorruptFile: an unparseable state file used to fail Open,
// which disabled watching for that run and every later one, since nothing in
// orange could repair it. It is now moved aside so the list starts empty and
// keeps working, with the old contents still there to look at.
func TestOpenSetsAsideCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.json")
	const bad = "{not json at all"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a corrupt file = %v, want it to recover", err)
	}
	if s == nil {
		t.Fatal("Open returned no store")
	}
	if got := len(s.All()); got != 0 {
		t.Errorf("recovered store has %d entries, want an empty list", got)
	}

	moved, ok := s.Recovered()
	if !ok {
		t.Fatal("Recovered() = false, want the corrupt file to be reported")
	}
	if want := path + ".corrupt"; moved != want {
		t.Errorf("moved to %q, want %q", moved, want)
	}

	// The old contents survive, which is the point of moving rather than
	// deleting.
	kept, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("reading the set-aside file: %v", err)
	}
	if string(kept) != bad {
		t.Errorf("set-aside file = %q, want %q", kept, bad)
	}
	// And the original path is free for a fresh file.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) = %v, want the file to have been moved", path, err)
	}
}

// TestWatchingWorksAfterRecovery is the reason for setting the file aside:
// the watch list has to be usable again, on this run and the next.
func TestWatchingWorksAfterRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte("]["), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Toggle(7, "A story", 3, 100)
	if err := s.Save(); err != nil {
		t.Fatalf("Save after recovery: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !s2.IsWatched(7) {
		t.Error("the story written after recovery did not persist")
	}
	if _, ok := s2.Recovered(); ok {
		t.Error("the rewritten file was reported as corrupt")
	}
}

// TestOpenEmptyFile: a zero-length file is nothing written yet, not
// something to move aside and warn about.
func TestOpenEmptyFile(t *testing.T) {
	for _, content := range []string{"", "\n", "  \n\t"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "watched.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Open(path)
		if err != nil {
			t.Fatalf("Open(%q) = %v", content, err)
		}
		if moved, ok := s.Recovered(); ok {
			t.Errorf("Open(%q) set the file aside at %q, want it left alone", content, moved)
		}
		if _, err := os.Stat(path + ".corrupt"); !os.IsNotExist(err) {
			t.Errorf("Open(%q) created a .corrupt file", content)
		}
	}
}

// TestOpenValidFileIsNotDisturbed guards the other side: a good file must
// never be moved aside.
func TestOpenValidFileIsNotDisturbed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte(`{"9":{"id":9,"title":"t"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := s.Recovered(); ok {
		t.Error("a valid file was reported as corrupt")
	}
	if !s.IsWatched(9) {
		t.Error("the valid file was not loaded")
	}
}

// TestOpenPartialFileDoesNotLeakEntries: Unmarshal can fill part of the map
// before it fails, so recovery has to start from an empty one.
func TestOpenPartialFileDoesNotLeakEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	// Valid up to the truncation, so the decoder gets one entry in first.
	if err := os.WriteFile(path, []byte(`{"9":{"id":9,"title":"t"},"10":{`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := s.All(); len(got) != 0 {
		t.Errorf("recovered store carried %d entries forward: %+v", len(got), got)
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Skipf("no user config dir on this machine: %v", err)
	}
	if want := filepath.Join("orange", "watched.json"); !strings.HasSuffix(p, want) {
		t.Errorf("DefaultPath() = %q, want it to end in %q", p, want)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("DefaultPath() = %q, want an absolute path", p)
	}
}

// TestMutationsDoNotTouchTheDisk is the point of splitting Save out: a
// keypress changes the list in memory, and the write happens elsewhere.
func TestMutationsDoNotTouchTheDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	s.Toggle(1, "a story", 3, 100)
	s.MarkRead(1, 9, 200)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) = %v, want no file until Save", path, err)
	}
	if !s.IsWatched(1) {
		t.Error("the change is not visible in memory either")
	}

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("after Save: %v, want the file written", err)
	}
}

// TestSaveIsANoOpWhenNothingChanged: Save runs after every mutation and on
// the way out, so a clean one must not rewrite the file.
func TestSaveIsANoOpWhenNothingChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save on a clean store: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a clean Save created %q", path)
	}

	s.Toggle(1, "t", 0, 1)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// A second Save with nothing new must leave the file alone.
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(again.ModTime()) || info.Size() != again.Size() {
		t.Error("a clean Save rewrote the file")
	}
}

// TestFailedSaveStaysDirty: a write that does not land must be tried again,
// or a change would be silently dropped by the flush on the way out.
func TestFailedSaveStaysDirty(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	path := filepath.Join(sub, "watched.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Toggle(1, "t", 0, 1)

	// A file where the directory needs to be, so MkdirAll cannot succeed.
	if err := os.WriteFile(sub, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err == nil {
		t.Fatal("Save into a blocked path returned no error")
	}

	// Still dirty, so the retry writes rather than no-opping.
	if err := os.Remove(sub); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("retry after a failed Save: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !s2.IsWatched(1) {
		t.Error("the change was lost after a failed Save and a retry")
	}
}

// TestConcurrentSaves runs under -race in CI. Saves now happen off the
// update loop, so several can be in flight at once, and the file has to end
// up holding the final state rather than a half-written or stale one.
func TestConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Toggle(i, "story", 0, int64(i))
			if err := s.Save(); err != nil {
				t.Errorf("Save: %v", err)
			}
		}()
	}
	wg.Wait()

	// One last Save settles whatever the racing ones left unwritten.
	if err := s.Save(); err != nil {
		t.Fatalf("final Save: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(s2.All()); got != 20 {
		t.Errorf("reopened store has %d entries, want 20", got)
	}
}
