package ui

import (
	"errors"
	"fmt"

	"github.com/jonhadfield/orange/internal/hn"
)

// applyFetchErr classifies a batch fetch error the way the story view does:
// a *hn.PartialError (or any failure that still left something to show)
// becomes a warning so the list stays on screen; only a total failure with
// nothing to display is a hard error.
func applyFetchErr(err error, gotItems, hadItems bool, unit string) (warn string, hard error) {
	if err == nil {
		return "", nil
	}
	var partial *hn.PartialError
	if errors.As(err, &partial) {
		return fmt.Sprintf("%d of %d %s failed to load",
			partial.Requested-partial.Fetched, partial.Requested, unit), nil
	}
	if gotItems || hadItems {
		return err.Error(), nil
	}
	return "", err
}
