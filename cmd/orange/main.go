// Command orange is a terminal reader for Hacker News.
package main

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/pkg/browser"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/store"
	"github.com/jonhadfield/orange/internal/ui"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `orange — a terminal reader for Hacker News.

Usage:
  orange                start reading
  orange -h, --help     print this help
  orange -v, --version  print the version

orange takes no configuration. Once it is running, press ? for the keys
that work in whatever view you are on, and q to quit.
`

// parseArgs handles the command line before the reader starts. It returns
// the exit code and whether to go on and start the reader, rather than
// exiting itself, so that it can be tested.
func parseArgs(args []string, stdout, stderr io.Writer) (code int, start bool) {
	if len(args) == 0 {
		return 0, true
	}
	if len(args) > 1 {
		fmt.Fprintf(stderr, "orange: unexpected argument %q\n\n%s", args[1], usage)
		return 2, false
	}
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintln(stdout, "orange "+version)
		return 0, false
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usage)
		return 0, false
	default:
		// Exit 2 for a usage error, as the flag package does, so a script
		// can tell a bad invocation from a reader that failed to run.
		fmt.Fprintf(stderr, "orange: unknown argument %q\n\n%s", args[0], usage)
		return 2, false
	}
}

// openStore loads the watch list, or explains why it could not and returns
// nil, which disables watching while leaving the rest of the reader working.
// Nothing here can stop orange starting: the store failing is a reason to
// read without watching, not a reason not to read.
//
// The warning goes out before the alternate screen takes over, so in
// practice it scrolls past unseen; ui.New says it again on the first frame
// where it will actually be read. This line is for the scrollback and for
// anyone running orange from a script.
func openStore(path string, stderr io.Writer) *store.Store {
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintln(stderr, "orange: watch state unavailable:", err)
		return nil
	}
	return st
}

func main() {
	if code, start := parseArgs(os.Args[1:], os.Stdout, os.Stderr); !start {
		os.Exit(code)
	}

	// pkg/browser writes launcher output to stdout/stderr, which would
	// corrupt the alternate screen.
	browser.Stdout = io.Discard
	browser.Stderr = io.Discard

	client := hn.NewClient("", hn.WithUserAgent(
		"orange/"+version+" (+https://github.com/jonhadfield/orange)"))
	// The alternate screen is declared by the model's View in Bubble Tea v2,
	// so it is no longer a program option here.
	p := tea.NewProgram(ui.New(client, openStore("", os.Stderr)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "orange:", err)
		os.Exit(1)
	}
}
