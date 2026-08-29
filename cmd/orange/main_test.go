package main

import (
	"strings"
	"testing"
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
