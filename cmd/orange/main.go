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
	st, err := store.Open("")
	if err != nil {
		// Watching is disabled but the reader still works.
		fmt.Fprintln(os.Stderr, "orange: watch state unavailable:", err)
		st = nil
	}
	// The alternate screen is declared by the model's View in Bubble Tea v2,
	// so it is no longer a program option here.
	p := tea.NewProgram(ui.New(client, st))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "orange:", err)
		os.Exit(1)
	}
}
