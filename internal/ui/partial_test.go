package ui

import (
	"errors"
	"testing"

	"github.com/jonhadfield/orange/internal/hn"
)

func TestApplyFetchErrPartialIsWarning(t *testing.T) {
	partial := &hn.PartialError{Fetched: 2, Requested: 3, Err: errors.New("timeout")}
	warn, hard := applyFetchErr(partial, true, false, "stories")
	if hard != nil {
		t.Fatalf("PartialError became a hard error: %v", hard)
	}
	if want := "1 of 3 stories failed to load"; warn != want {
		t.Errorf("warn = %q, want %q", warn, want)
	}
}

func TestApplyFetchErrTotalWithNothingIsHard(t *testing.T) {
	err := errors.New("dial failed")
	warn, hard := applyFetchErr(err, false, false, "stories")
	if warn != "" {
		t.Errorf("warn = %q, want empty", warn)
	}
	if hard != err {
		t.Errorf("hard = %v, want %v", hard, err)
	}
}

func TestApplyFetchErrTotalWithExistingItemsIsWarning(t *testing.T) {
	err := errors.New("dial failed")
	warn, hard := applyFetchErr(err, false, true, "posts")
	if hard != nil {
		t.Fatalf("kept items but still got hard error: %v", hard)
	}
	if warn != err.Error() {
		t.Errorf("warn = %q, want %q", warn, err.Error())
	}
}
