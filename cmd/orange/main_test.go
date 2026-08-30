package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
	"github.com/jonhadfield/orange/internal/ui"
)

// TestParseArgs covers the command line, which is the whole of orange's
// interface before the reader takes over: anything that is not a recognised
// argument has to say so and stop, rather than dropping the caller into the
// alternate screen with no idea why.
func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		code  int
		start bool
		out   string // substring expected on stdout
		errs  string // substring expected on stderr
	}{
		{name: "no arguments start the reader", args: nil, code: 0, start: true},

		{name: "--version", args: []string{"--version"}, out: "orange "},
		{name: "-v", args: []string{"-v"}, out: "orange "},
		{name: "version", args: []string{"version"}, out: "orange "},

		{name: "--help", args: []string{"--help"}, out: "Usage:"},
		{name: "-h", args: []string{"-h"}, out: "Usage:"},
		{name: "help", args: []string{"help"}, out: "Usage:"},

		{
			name: "an unknown flag is a usage error",
			args: []string{"--nonsense"}, code: 2, errs: `unknown argument "--nonsense"`,
		},
		{
			name: "a bare word is a usage error too",
			args: []string{"nonsense"}, code: 2, errs: `unknown argument "nonsense"`,
		},
		{
			name: "trailing arguments are rejected rather than ignored",
			args: []string{"--version", "extra"}, code: 2, errs: `unexpected argument "extra"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code, start := parseArgs(tc.args, &stdout, &stderr)

			if code != tc.code {
				t.Errorf("exit code = %d, want %d", code, tc.code)
			}
			if start != tc.start {
				t.Errorf("start = %v, want %v", start, tc.start)
			}
			if tc.out != "" && !strings.Contains(stdout.String(), tc.out) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.out)
			}
			if tc.errs != "" && !strings.Contains(stderr.String(), tc.errs) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.errs)
			}
			// A usage error is no use if it does not say what the usage is.
			if tc.code != 0 && !strings.Contains(stderr.String(), "Usage:") {
				t.Errorf("stderr does not include the usage block:\n%s", stderr.String())
			}
			// Nothing is written to the stream the caller is not reading.
			if tc.code == 0 && stderr.Len() > 0 {
				t.Errorf("wrote to stderr on success: %q", stderr.String())
			}
			if tc.code != 0 && stdout.Len() > 0 {
				t.Errorf("wrote to stdout on a usage error: %q", stdout.String())
			}
		})
	}
}

// TestUsageMentionsTheWayOut: the reason --help matters here is that a
// reader who has not found ? or q is stuck in the alternate screen, so the
// help has to name both.
func TestUsageMentionsTheWayOut(t *testing.T) {
	for _, want := range []string{"?", "q", "--help", "--version"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not mention %q:\n%s", want, usage)
		}
	}
}

// TestOpenStoreFailureDoesNotStopTheReader: a store that cannot be opened
// disables watching, but orange still has to start. This is the degradation
// path — the reader working without a watch list is the whole point of
// returning nil rather than exiting.
func TestOpenStoreFailureDoesNotStopTheReader(t *testing.T) {
	dir := t.TempDir()
	// A file where the directory has to be, so the path cannot be read.
	blocked := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocked, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr strings.Builder
	st := openStore(filepath.Join(blocked, "watched.json"), &stderr)

	if st != nil {
		t.Error("openStore returned a store for an unreadable path")
	}
	said := stderr.String()
	if !strings.Contains(said, "watch state unavailable") {
		t.Errorf("stderr = %q, want it to say watching is unavailable", said)
	}
	// The message has to carry the underlying reason, or there is nothing
	// to act on.
	if strings.TrimSpace(said) == "orange: watch state unavailable:" {
		t.Errorf("stderr = %q, want the underlying error included", said)
	}
	// And the reader still runs on the nil store rather than refusing: the
	// model is built, sized as a terminal would, and draws a frame.
	m, _ := ui.New(hn.NewClient("http://unused.invalid"), st).
		Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// Styling is stripped first: the tab labels are coloured per character,
	// so the words are not contiguous in the raw output.
	if content := stripANSI(m.View().Content); !strings.Contains(content, "Top") {
		t.Errorf("the reader did not draw its feed list without a store:\n%s", content)
	}
}

// TestOpenStoreSuccessIsSilent: an ordinary start says nothing on stderr,
// which matters because the warning is printed just before the alternate
// screen takes over.
func TestOpenStoreSuccessIsSilent(t *testing.T) {
	var stderr strings.Builder
	st := openStore(filepath.Join(t.TempDir(), "watched.json"), &stderr)

	if st == nil {
		t.Fatal("openStore returned nil for a usable path")
	}
	if stderr.Len() != 0 {
		t.Errorf("wrote to stderr on a clean start: %q", stderr.String())
	}
	if st.IsWatched(1) {
		t.Error("a fresh store reports a story as watched")
	}
}

// TestOpenStoreRecoversRatherThanFailing: an unreadable watch file is moved
// aside by the store rather than reported here, so this path returns a
// working store and stays quiet. Without that, a corrupt file would look
// like the failure case above and disable watching for good.
func TestOpenStoreRecoversRatherThanFailing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watched.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr strings.Builder
	st := openStore(path, &stderr)

	if st == nil {
		t.Fatal("a corrupt watch file disabled watching, want it recovered")
	}
	if stderr.Len() != 0 {
		t.Errorf("recovery wrote to stderr: %q", stderr.String())
	}
	if moved, ok := st.Recovered(); !ok {
		t.Error("the store does not report the file it set aside")
	} else if !strings.HasSuffix(moved, ".corrupt") {
		t.Errorf("set aside at %q, want a .corrupt file", moved)
	}
}

// stripANSI removes escape sequences so a test can match on what a reader
// would see rather than on how it was coloured.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		for i < len(s) && s[i] != 'm' && s[i] != '\\' && s[i] != 0x07 {
			i++
		}
	}
	return b.String()
}
