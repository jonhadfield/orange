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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("orange " + version)
			return
		}
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
