package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
)

// TestPulseRefreshLeavesOneTimer: every reading schedules the next refresh,
// so a reading that r asked for must replace the timer already running
// rather than start a second one beside it. Otherwise each r adds another
// refresh loop and Pulse polls more often than the interval it shows.
func TestPulseRefreshLeavesOneTimer(t *testing.T) {
	m := newPulseModel(nil, newKeyMap())
	m = pulseWith(m, []hn.Item{story(1, 10, 0)})
	stale := pulseTickMsg{gen: m.tickGen}

	m, cmd := m.Update(keyPress("r"))
	if cmd == nil || !m.loading {
		t.Fatal("r did not start a refresh")
	}
	m = pulseWith(m, []hn.Item{story(1, 12, 0)})

	m, cmd = m.Update(stale)
	if cmd != nil || m.loading {
		t.Error("the timer from before r still refreshed, so two refresh loops are running")
	}

	m, cmd = m.Update(pulseTickMsg{gen: m.tickGen})
	if cmd == nil || !m.loading {
		t.Error("the current timer did not refresh")
	}
}

// TestPulseReentryLeavesOneTimer: leaving Pulse and pressing p again within
// the interval starts a fresh reading, and the timer that was still pending
// from the earlier visit must not keep a second loop going.
func TestPulseReentryLeavesOneTimer(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "p")
	m = update(t, m, pulseDataMsg{items: []hn.Item{story(1, 10, 0)}})
	stale := pulseTickMsg{gen: m.pulse.tickGen}

	m = press(t, m, "esc")
	m = press(t, m, "p")
	m = update(t, m, pulseDataMsg{items: []hn.Item{story(1, 12, 0)}})

	m = update(t, m, stale)
	if m.pulse.loading {
		t.Error("the timer from the earlier visit still refreshed, so two refresh loops are running")
	}
}

// TestPulseResumesAfterReadingAStory: the timer that comes due while a story
// opened from Pulse is on screen is dropped, so nothing refreshes in the
// background. Coming back to Pulse has to start refreshing again, or the
// header keeps promising a refresh every 45s that never happens.
func TestPulseResumesAfterReadingAStory(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "p")
	m = update(t, m, pulseDataMsg{items: []hn.Item{story(1, 10, 0)}})

	m = press(t, m, "enter")
	if m.view != viewStory {
		t.Fatalf("enter opened view %v, want the story", m.view)
	}
	m = update(t, m, pulseTickMsg{gen: m.pulse.tickGen})
	if m.pulse.loading {
		t.Fatal("Pulse refreshed while a story was on screen")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.view != viewPulse {
		t.Fatalf("esc went to view %v, want Pulse", m.view)
	}
	if cmd == nil || !m.pulse.loading {
		t.Error("Pulse did not resume refreshing after the story was closed")
	}
}

// TestPulseKeepsItsTimerAcrossAQuickStoryVisit: back from a story before the
// timer came due, the timer is still pending, so returning must not start
// an extra refresh on top of it.
func TestPulseKeepsItsTimerAcrossAQuickStoryVisit(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "p")
	m = update(t, m, pulseDataMsg{items: []hn.Item{story(1, 10, 0)}})

	m = press(t, m, "enter")
	m = press(t, m, "esc")
	if m.pulse.loading {
		t.Error("returning to Pulse refreshed although its timer was still pending")
	}
}

// press sends one key to the root model; esc and enter by name, anything
// else as typed.
func press(t *testing.T, m Model, k string) Model {
	t.Helper()
	msg := keyPress(k)
	switch k {
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return update(t, m, msg)
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}
