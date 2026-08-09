package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jonhadfield/orange/internal/hn"
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
