// Package store persists orange's small local state: the set of watched
// stories and how much of each discussion has been read.
package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// WatchState records one watched story.
type WatchState struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	LastReadAt   int64  `json:"last_read_at"`
	LastComments int    `json:"last_comments"`
}

// Store is a JSON-file-backed watch list, safe for concurrent use.
type Store struct {
	path string
	// held for the whole of Save, so two writes cannot overlap and an
	// older snapshot cannot land on top of a newer one
	writeMu sync.Mutex
	// where an unparseable file was moved to at Open, empty if there was
	// nothing wrong with it
	corrupt string

	mu      sync.Mutex
	watched map[int]WatchState
	// set by a mutation, cleared by a Save that reached the disk
	dirty bool
}

// DefaultPath is where the watch list lives when Open is given no path.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "orange", "watched.json"), nil
}

// Open loads the store at path, or the default per-user location when path
// is empty. A missing file yields an empty store, and so does an unreadable
// one: see setAside. Recovered reports the latter.
func Open(path string) (*Store, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	s := &Store{path: path, watched: map[int]WatchState{}}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		// An empty file is not corrupt, just nothing written yet, and is
		// not worth setting aside.
		if len(bytes.TrimSpace(b)) == 0 {
			return s, nil
		}
		if err := json.Unmarshal(b, &s.watched); err != nil {
			if err := s.setAside(); err != nil {
				return nil, err
			}
		}
	case !os.IsNotExist(err):
		return nil, err
	}
	return s, nil
}

// setAside moves an unparseable state file out of the way, so that watching
// works on this run and every later one instead of staying dead until
// somebody deletes the file by hand. The old contents are kept alongside,
// where they can still be looked at. A previous .corrupt file is replaced,
// the newer one being the more useful of the two.
func (s *Store) setAside() error {
	// Unmarshal can populate part of the map before failing, so the list
	// starts again rather than carrying half a file forward.
	s.watched = map[int]WatchState{}
	moved := s.path + ".corrupt"
	if err := os.Rename(s.path, moved); err != nil {
		return err
	}
	s.corrupt = moved
	return nil
}

// Recovered reports whether Open found an unparseable state file, and where
// it was moved to. The watch list started empty when it did.
func (s *Store) Recovered() (movedTo string, ok bool) {
	return s.corrupt, s.corrupt != ""
}

// Save writes the watch list if anything has changed since the last one, and
// is a no-op otherwise. It is safe to call from any goroutine, and meant to
// be called off the update loop: the write is an atomic rewrite with an
// fsync in it, which is far too slow to sit in the path of a keystroke.
func (s *Store) Save() error {
	// One write at a time. Holding this for the whole of Save means the
	// snapshot below is taken after any earlier write has finished, so a
	// slower earlier write cannot land on top of a newer one.
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// The lock is held just long enough to copy the state out, never across
	// the file I/O, so a keypress arriving mid-write does not wait for it.
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	b, err := json.MarshalIndent(s.watched, "", "  ")
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return err
	}

	if err := s.write(b); err != nil {
		// Still unsaved, so a later Save — the one on the way out, if
		// nothing else — tries again.
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return err
	}
	return nil
}

// write rewrites the store atomically: a crash or a full disk mid-write
// leaves the previous watch list intact rather than a truncated file.
func (s *Store) write(b []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// The temp file must share a filesystem with the target for the rename
	// to be atomic, so it goes in the same directory.
	f, err := os.CreateTemp(dir, ".watched-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once the rename succeeds

	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// IsWatched reports whether the story is on the watch list.
func (s *Store) IsWatched(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.watched[id]
	return ok
}

// Get returns the watch state for a story.
func (s *Store) Get(id int) (WatchState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, ok := s.watched[id]
	return ws, ok
}

// Toggle adds the story to the watch list, or removes it if present, and
// reports whether it is now watched. The change is in memory only; Save
// puts it on disk.
func (s *Store) Toggle(id int, title string, comments int, now int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
	if _, ok := s.watched[id]; ok {
		delete(s.watched, id)
		return false
	}
	s.watched[id] = WatchState{ID: id, Title: title, LastReadAt: now, LastComments: comments}
	return true
}

// MarkRead records that the story's discussion has been read up to now,
// with the given comment count. It is a no-op for unwatched stories. As
// with Toggle, the change is in memory only until Save.
func (s *Store) MarkRead(id int, comments int, now int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, ok := s.watched[id]
	if !ok {
		return
	}
	ws.LastReadAt = now
	ws.LastComments = comments
	s.watched[id] = ws
	s.dirty = true
}

// All returns the watch list, most recently read first.
func (s *Store) All() []WatchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]WatchState, 0, len(s.watched))
	for _, ws := range s.watched {
		out = append(out, ws)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastReadAt > out[j].LastReadAt })
	return out
}
