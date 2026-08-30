package ui

import (
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

// pulseWith feeds one reading into a pulse model and returns the result, the
// way the refresh timer does.
func pulseWith(m pulseModel, items []hn.Item) pulseModel {
	next, _ := m.Update(pulseDataMsg{items: items})
	return next
}

func story(id, score, comments int) hn.Item {
	return hn.Item{ID: id, Type: "story", Title: "story", Score: score, Descendants: comments}
}

// rowFor finds a story's row by ID, since the row order is the rank order
// and moves between readings.
func rowFor(t *testing.T, m pulseModel, id int) pulseRow {
	t.Helper()
	for _, r := range m.rows {
		if r.item.ID == id {
			return r
		}
	}
	t.Fatalf("story %d is not in the pulse rows", id)
	return pulseRow{}
}

// TestPulseFirstReadingShowsNoMovement: there is nothing to compare a first
// reading against, so every row has to be still. This is what a reader sees
// on opening Pulse, and it is the state the layout tests never assert.
func TestPulseFirstReadingShowsNoMovement(t *testing.T) {
	m := pulseWith(newPulseModel(nil, newKeyMap()), []hn.Item{
		story(1, 100, 10), story(2, 90, 5), story(3, 80, 1),
	})

	if len(m.rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(m.rows))
	}
	for _, r := range m.rows {
		if r.dRank != 0 || r.dScore != 0 || r.dComments != 0 {
			t.Errorf("story %d moved on the first reading: %+v", r.item.ID, r)
		}
		if r.isNew {
			t.Errorf("story %d marked new on the first reading", r.item.ID)
		}
	}
}

// TestPulseMovementBetweenReadings pins the arithmetic and, more
// importantly, the signs: a story climbing the list has a smaller index, so
// dRank is positive and renders as ↑.
func TestPulseMovementBetweenReadings(t *testing.T) {
	m := newPulseModel(nil, newKeyMap())
	m = pulseWith(m, []hn.Item{
		story(1, 100, 10), // rank 0
		story(2, 90, 5),   // rank 1
		story(3, 80, 1),   // rank 2
	})
	// Story 3 climbs to the top, 1 falls to the middle, 2 falls to the
	// bottom, and 4 arrives.
	m = pulseWith(m, []hn.Item{
		story(3, 300, 40), // rank 0, was 2
		story(1, 120, 12), // rank 1, was 0
		story(4, 50, 2),   // rank 2, new
		story(2, 90, 5),   // rank 3, was 1
	})

	risen := rowFor(t, m, 3)
	if risen.dRank != 2 {
		t.Errorf("story 3 climbed from rank 2 to 0, dRank = %d, want +2 (an up arrow)", risen.dRank)
	}
	if risen.dScore != 220 || risen.dComments != 39 {
		t.Errorf("story 3 velocity = +%d points, +%d comments, want +220 and +39",
			risen.dScore, risen.dComments)
	}

	if fell := rowFor(t, m, 1); fell.dRank != -1 {
		t.Errorf("story 1 fell from rank 0 to 1, dRank = %d, want -1 (a down arrow)", fell.dRank)
	}
	if fell := rowFor(t, m, 2); fell.dRank != -2 {
		t.Errorf("story 2 fell from rank 1 to 3, dRank = %d, want -2", fell.dRank)
	}

	// An unchanged score must not show velocity.
	if r := rowFor(t, m, 2); r.dScore != 0 || r.dComments != 0 {
		t.Errorf("story 2 did not change but shows %+d points, %+d comments", r.dScore, r.dComments)
	}

	// The arrival is marked new rather than as having moved.
	arrival := rowFor(t, m, 4)
	if !arrival.isNew {
		t.Error("story 4 arrived between readings but is not marked new")
	}
	if arrival.dRank != 0 || arrival.dScore != 0 {
		t.Errorf("story 4 is new, so it cannot have moved: %+v", arrival)
	}
}

// TestPulseDroppedStoryLeavesTheList: a story falling off the front page
// goes, and must not leave a stale row behind.
func TestPulseDroppedStoryLeavesTheList(t *testing.T) {
	m := newPulseModel(nil, newKeyMap())
	m = pulseWith(m, []hn.Item{story(1, 100, 10), story(2, 90, 5)})
	m = pulseWith(m, []hn.Item{story(1, 110, 12)})

	if len(m.rows) != 1 {
		t.Fatalf("got %d rows after one story dropped, want 1: %+v", len(m.rows), m.rows)
	}
	if m.rows[0].item.ID != 1 {
		t.Errorf("the remaining row is story %d, want 1", m.rows[0].item.ID)
	}
}

// TestPulseCursorSurvivesAShrinkingList: the cursor indexes the rows, so a
// shorter reading must not leave it pointing past the end.
func TestPulseCursorSurvivesAShrinkingList(t *testing.T) {
	m := newPulseModel(nil, newKeyMap())
	m = pulseWith(m, []hn.Item{story(1, 100, 1), story(2, 90, 1), story(3, 80, 1)})
	m.cursor = 2

	m = pulseWith(m, []hn.Item{story(1, 100, 1)})
	if m.cursor >= len(m.rows) {
		t.Errorf("cursor = %d with %d rows, want it inside the list", m.cursor, len(m.rows))
	}
	if _, ok := m.selected(); !ok {
		t.Error("selected() found nothing after the list shrank")
	}
}
