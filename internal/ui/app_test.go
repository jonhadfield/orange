package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
)

// newTestModel builds a root model sized for rendering, without running the
// program, so nothing touches the network.
func newTestModel(t *testing.T) Model {
	t.Helper()
	m := New(hn.NewClient("http://unused.invalid"), nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

func TestViewRendersAndOwnsAltScreen(t *testing.T) {
	m := newTestModel(t)

	v := m.View()
	// v2 moved the alternate screen from a program option onto the view, so
	// losing this would silently drop the app into the normal buffer.
	if !v.AltScreen {
		t.Error("View().AltScreen = false, want true")
	}
	if strings.TrimSpace(v.Content) == "" {
		t.Fatal("View().Content is empty")
	}
	if !strings.Contains(v.Content, "HN") {
		t.Errorf("View().Content missing the HN logo:\n%s", v.Content)
	}
}

func TestViewRequestsMouseReporting(t *testing.T) {
	m := newTestModel(t)
	// Without this the terminal keeps wheel and trackpad events to itself
	// and the app never sees them.
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("View().MouseMode = %v, want MouseModeCellMotion", got)
	}
}

func TestWheelReachesTheActiveView(t *testing.T) {
	m := newTestModel(t)
	// Seed the feed so the cursor has somewhere to go.
	st := m.feeds.state()
	for i := 1; i <= 10; i++ {
		st.ids = append(st.ids, i)
		st.items = append(st.items, hn.Item{ID: i, Type: "story", Title: "story", By: "someone"})
	}

	// The root model has no case for mouse messages, so this also checks
	// they fall through to the active view rather than being dropped.
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 5, Y: 5})
	m = next.(Model)
	if got := m.feeds.state().cursor; got != 1 {
		t.Fatalf("feed cursor = %d after one wheel notch, want 1", got)
	}

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 5, Y: 5})
	m = next.(Model)
	if got := m.feeds.state().cursor; got != 0 {
		t.Errorf("feed cursor = %d after wheeling back up, want 0", got)
	}
}

func TestViewBeforeSizeIsEmptyButStillAltScreen(t *testing.T) {
	m := New(hn.NewClient("http://unused.invalid"), nil)
	v := m.View()
	if !v.AltScreen {
		t.Error("AltScreen = false before the first WindowSizeMsg, want true")
	}
	if v.Content != "" {
		t.Errorf("Content = %q before sizing, want empty", v.Content)
	}
}

func TestBackgroundColorMsgSwitchesPalette(t *testing.T) {
	t.Cleanup(func() { setTheme(true) }) // styles are package-level

	m := newTestModel(t)

	dark, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})
	darkView := dark.(Model).View().Content

	light, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	lightView := light.(Model).View().Content

	if darkView == lightView {
		t.Error("light and dark backgrounds rendered identically; the palette is not being rebuilt")
	}
	// The spinners copy their style at construction, so they must be handed
	// the rebuilt one rather than keeping the boot-time colours.
	if got := light.(Model).story.spinner.Style; got.GetForeground() != stylePoints.GetForeground() {
		t.Error("story spinner kept its old style after the theme changed")
	}
}

func TestKeyPressesDoNotPanic(t *testing.T) {
	m := newTestModel(t)

	// A walk through the global bindings, including the view switches, to
	// catch anything the v2 key rework left mis-wired.
	for _, k := range []string{"j", "k", "g", "G", "p", "H", "W", "?", "esc", "o", "c"} {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
		if strings.TrimSpace(m.View().Content) == "" {
			t.Fatalf("view went blank after key %q", k)
		}
	}
}

// TestRecoveredStoreIsAnnounced: the warning main prints before the
// alternate screen takes over scrolls past unseen, so a state file that had
// to be set aside has to be said inside the reader, on the first frame,
// naming where the old file went.
func TestRecoveredStoreIsAnnounced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	m := New(hn.NewClient("http://unused.invalid"), st)
	if m.notice == "" {
		t.Fatal("no notice after recovering a corrupt state file")
	}
	if !strings.Contains(m.notice, path+".corrupt") {
		t.Errorf("notice does not say where the old file went:\n%s", m.notice)
	}

	// And it is on screen, not merely on the model.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if content := stripStyles(next.(Model).View().Content); !strings.Contains(content, "unreadable") {
		t.Errorf("the notice is not rendered:\n%s", content)
	}
}

// TestNoNoticeForAHealthyStore is the other half: an ordinary start says
// nothing.
func TestNoNoticeForAHealthyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if m := New(hn.NewClient("http://unused.invalid"), st); m.notice != "" {
		t.Errorf("notice on a clean start: %q", m.notice)
	}
	if m := New(hn.NewClient("http://unused.invalid"), nil); m.notice != "" {
		t.Errorf("notice with no store at all: %q", m.notice)
	}
}

// TestStoreUnavailableNamesTheFile: without the path there is nothing the
// reader can go and fix, and it is documented nowhere else.
func TestStoreUnavailableNamesTheFile(t *testing.T) {
	want, err := store.DefaultPath()
	if err != nil {
		t.Skipf("no user config dir on this machine: %v", err)
	}
	got := storeUnavailable("watching")
	if !strings.Contains(got, want) {
		t.Errorf("storeUnavailable() = %q, want it to name %q", got, want)
	}
	if !strings.HasPrefix(got, "watching") {
		t.Errorf("storeUnavailable(%q) = %q, want it to start with the subject", "watching", got)
	}
}
