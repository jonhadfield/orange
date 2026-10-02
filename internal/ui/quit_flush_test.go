package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonhadfield/orange/internal/store"
)

func TestFlushStoreOnQuitReportsFailure(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	st, err := store.Open(filepath.Join(sub, "watched.json"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.Toggle(1, "t", 0, 1)
	if err := os.WriteFile(sub, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	flushStoreOnQuit(st, &buf)
	if !strings.Contains(buf.String(), "watch list not saved") {
		t.Errorf("stderr = %q, want a save-failure message", buf.String())
	}
}

func TestFlushStoreOnQuitNilIsNoop(t *testing.T) {
	var buf bytes.Buffer
	flushStoreOnQuit(nil, &buf)
	if buf.Len() != 0 {
		t.Errorf("nil store wrote %q", buf.String())
	}
}
