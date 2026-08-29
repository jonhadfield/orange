package htmltext

import (
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "entities",
			in:   "Hello &amp; goodbye &gt; hi",
			want: "Hello & goodbye > hi",
		},
		{
			name: "paragraphs",
			in:   "one<p>two<p>three",
			want: "one\n\ntwo\n\nthree",
		},
		{
			name: "italics",
			in:   "<i>word</i> rest",
			want: "_word_ rest",
		},
		{
			name: "link with url as text",
			in:   `see <a href="https://x.com/Page" rel="nofollow">https:&#x2F;&#x2F;x.com&#x2F;Page</a>`,
			want: "see https://x.com/Page",
		},
		{
			name: "link with truncated url text",
			in:   `<a href="https://example.com/very/long/path" rel="nofollow">https:&#x2F;&#x2F;example.com&#x2F;very&#x2F;lo...</a>`,
			want: "https://example.com/very/long/path",
		},
		{
			name: "link with descriptive text",
			in:   `<a href="https://x.com/">this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "code block",
			in:   "a<p><pre><code>x := 1\ny := 2\n</code></pre><p>b",
			want: "a\n\n    x := 1\n    y := 2\n\nb",
		},
		{
			name: "line breaks",
			in:   "one<br>two",
			want: "one\ntwo",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "stray angle bracket",
			in:   "1 < 2 is true",
			want: "1 < 2 is true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestConvertLinked(t *testing.T) {
	in := `see <a href="https://x.com/page">docs</a>`
	want := "see \x1b[4m\x1b]8;;https://x.com/page\x1b\\docs (https://x.com/page)\x1b]8;;\x1b\\\x1b[24m"
	if got := ConvertLinked(in); got != want {
		t.Errorf("ConvertLinked(%q)\n got: %q\nwant: %q", in, got, want)
	}

	// Text without links is identical to Convert.
	plain := "one<p><i>two</i>"
	if got, want := ConvertLinked(plain), Convert(plain); got != want {
		t.Errorf("ConvertLinked(%q) = %q, want %q", plain, got, want)
	}
}

// TestConvertMalformed covers input the parser has to survive rather than
// silently swallow. HN sanitises comment HTML, so these are latent rather
// than live, but the failure mode was text loss: an unclosed <a> or <pre>
// buffered its content and then dropped it on the floor.
func TestConvertMalformed(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unclosed link still renders its text",
			in:   `<a href="https://x.com/">click`,
			want: "click (https://x.com/)",
		},
		{
			name: "unclosed pre still renders its block",
			in:   "a<p><pre><code>x := 1\ny := 2",
			want: "a\n\n    x := 1\n    y := 2",
		},
		{
			// Pre-existing and unrelated to the unclosed tag: convert ends
			// with TrimSpace, so a block at the very start of the input
			// loses its first line's indent. Pinned here so a later fix is
			// a deliberate change rather than a surprise.
			name: "a block at the very start loses its first indent",
			in:   "<pre><code>x := 1\ny := 2\n</code></pre>",
			want: "x := 1\n    y := 2",
		},
		{
			name: "text after an unclosed link is not lost",
			in:   `before <a href="https://x.com/">click`,
			want: "before click (https://x.com/)",
		},
		{
			name: "unclosed link inside an unclosed pre lands in the block",
			in:   `a<p><pre>code <a href="https://x.com/">t`,
			want: "a\n\n    code t (https://x.com/)",
		},
		{
			name: "single-quoted href",
			in:   `<a href='https://x.com/'>this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "bare href",
			in:   `<a href=https://x.com/>this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "bare href followed by another attribute",
			in:   `<a href=https://x.com/ rel=nofollow>this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "spaces around the equals sign",
			in:   `<a href = "https://x.com/">this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "unterminated quote salvages the rest",
			in:   `<a href="https://x.com/>this</a>`,
			want: "this (https://x.com/)",
		},
		{
			// The whole point of anchoring the name: data-href is a
			// different attribute and must not answer for href.
			name: "data-href does not answer for href",
			in:   `<a data-href="https://evil.example/" href="https://good.example/">this</a>`,
			want: "this (https://good.example/)",
		},
		{
			name: "data-href alone leaves the link with no target",
			in:   `<a data-href="https://evil.example/">this</a>`,
			want: "this",
		},
		{
			name: "hreflang is not href",
			in:   `<a hreflang="en">this</a>`,
			want: "this",
		},
		{
			name: "uppercase attribute name still matches",
			in:   `<a HREF="https://x.com/">this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "entities in a bare value are still unescaped",
			in:   `<a href=https://x.com/?a=1&amp;b=2>this</a>`,
			want: "this (https://x.com/?a=1&b=2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestAttrValue exercises the attribute scanner directly, where the
// whole-name rule is easiest to state.
func TestAttrValue(t *testing.T) {
	tests := []struct {
		attrs, name, want string
	}{
		{`href="a"`, "href", "a"},
		{`href='a'`, "href", "a"},
		{`href=a`, "href", "a"},
		{`rel="nofollow" href="a"`, "href", "a"},
		{`data-href="a"`, "href", ""},
		{`hreflang="en"`, "href", ""},
		{`hreflang="en" href="a"`, "href", "a"},
		{`href=""`, "href", ""},
		{``, "href", ""},
		{`href`, "href", ""},
		{`href=`, "href", ""},
	}
	for _, tt := range tests {
		if got := attrValue(tt.attrs, tt.name); got != tt.want {
			t.Errorf("attrValue(%q, %q) = %q, want %q", tt.attrs, tt.name, got, tt.want)
		}
	}
}

// TestConvertTagWhitespace: HTML separates a tag's name from its attributes
// with any whitespace, not only a space. Cutting on " " alone made the whole
// of `a\nhref="x"` the tag name, which matched no case, so the tag was
// dropped and took its attributes and its rendering with it.
func TestConvertTagWhitespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "newline before the attributes",
			in:   "<a\nhref=\"https://x.com/\">this</a>",
			want: "this (https://x.com/)",
		},
		{
			name: "newline before attributes, more attributes after",
			in:   "<a\nhref=\"https://x.com/\" rel=\"nofollow\">this</a>",
			want: "this (https://x.com/)",
		},
		{
			name: "tab before the attributes",
			in:   "<a\thref=\"https://x.com/\">this</a>",
			want: "this (https://x.com/)",
		},
		{
			name: "several spaces before the attributes",
			in:   `<a   href="https://x.com/">this</a>`,
			want: "this (https://x.com/)",
		},
		{
			name: "a paragraph split across lines still breaks",
			in:   "one<p\nclass=\"c\">two",
			want: "one\n\ntwo",
		},
		{
			name: "a pre split across lines still indents",
			in:   "a<p><pre\nclass=\"c\"><code>x := 1</code></pre>",
			want: "a\n\n    x := 1",
		},
		{
			name: "a tag name alone is unchanged",
			in:   "<i>word</i>",
			want: "_word_",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCutTagName(t *testing.T) {
	tests := []struct{ tag, name, attrs string }{
		{`a href="x"`, "a", `href="x"`},
		{"a\nhref=\"x\"", "a", `href="x"`},
		{"a\thref=\"x\"", "a", `href="x"`},
		{"a\r\nhref=\"x\"", "a", "\nhref=\"x\""},
		{"a", "a", ""},
		{"/a", "/a", ""},
		{"br/", "br/", ""},
		{"br /", "br", "/"},
	}
	for _, tt := range tests {
		name, attrs := cutTagName(tt.tag)
		if name != tt.name || attrs != tt.attrs {
			t.Errorf("cutTagName(%q) = (%q, %q), want (%q, %q)", tt.tag, name, attrs, tt.name, tt.attrs)
		}
	}
}

// TestStripControl covers the characters a terminal acts on rather than
// displays, and the two whitespace controls the renderer needs kept.
func TestStripControl(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"escape", "before\x1bafter", "beforeafter"},
		{"bel ends an OSC string", "a\x07b", "ab"},
		{"nul", "a\x00b", "ab"},
		{"del", "a\x7fb", "ab"},
		{"c1 csi", "a\u009bb", "ab"},
		{"c1 range start and end", "a\u0080b\u009fc", "abc"},
		{"a full escape sequence loses its escape", "\x1b[31mred", "[31mred"},
		{"carriage return goes", "a\rb", "ab"},
		{"newline stays", "a\nb", "a\nb"},
		{"tab stays", "a\tb", "a\tb"},
		{"ordinary text is untouched", "hello, world", "hello, world"},
		{"non-ascii is untouched", "café — naïve — 日本語 — 🎉", "café — naïve — 日本語 — 🎉"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripControl(tt.in); got != tt.want {
				t.Errorf("StripControl(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestConvertStripsEscapedControls is the case that matters: HN escapes its
// HTML, so a control character arrives as an entity and only becomes one
// after unescaping. Stripping before that would miss it entirely.
func TestConvertStripsEscapedControls(t *testing.T) {
	for _, in := range []string{
		"before&#x1b;[31mafter",
		"before&#27;[31mafter",
		"a&#0;b",
		"<p>text&#x9b;more",
		`<a href="https://example.com/&#x1b;\\">link</a>`,
	} {
		if got := Convert(in); strings.ContainsAny(got, "\x1b\x07\x00") {
			t.Errorf("Convert(%q) = %q, want no control characters", in, got)
		}
	}
}

// TestConvertLinkedCannotBeBrokenOutOf: href is interpolated straight into
// an OSC 8 sequence, so an ESC or BEL in it would end the sequence early and
// let the rest be read as terminal input. The only escapes in the output
// must be the ones this package wrote.
func TestConvertLinkedCannotBeBrokenOutOf(t *testing.T) {
	// The same link with the ESC left out. If the hostile one renders
	// identically, the ESC contributed nothing.
	hostile := ConvertLinked(`<a href="https://example.com/&#x1b;&#x5c;evil">click</a>`)
	benign := ConvertLinked(`<a href="https://example.com/&#x5c;evil">click</a>`)
	if hostile != benign {
		t.Errorf("the escape changed the output:\n got: %q\nwant: %q", hostile, benign)
	}

	// And no more escapes than an ordinary link needs, so the count is
	// taken from one rather than written down here.
	plain := ConvertLinked(`<a href="https://example.com/x">click</a>`)
	if got, want := strings.Count(hostile, "\x1b"), strings.Count(plain, "\x1b"); got != want {
		t.Errorf("output has %d escapes, want the %d an ordinary link uses:\n%q", got, want, hostile)
	}
	if strings.Contains(hostile, "\x07") {
		t.Errorf("output contains BEL, which ends an OSC string:\n%q", hostile)
	}
	// The link target survives, minus the escape.
	if !strings.Contains(hostile, "https://example.com/") {
		t.Errorf("the link target was lost entirely:\n%q", hostile)
	}
}

// FuzzConvertEmitsNoControls is the lock the issue asks for: whatever goes
// in, plain Convert never writes a character the terminal would act on.
func FuzzConvertEmitsNoControls(f *testing.F) {
	for _, seed := range []string{
		"", "plain text", "<p>one<p>two", "<i>x</i>", "a &amp; b",
		"&#x1b;[31m", "\x1bliteral", "<a href=\"&#7;\">t</a>",
		"<pre><code>x := 1", "<a href='x", "&#x9b;", "\x00\x01\x02",
		"<a\nhref=\"&#27;\">t</a>", "&#xfeff;", "\u0080\u009f",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Convert(in)
		for i, r := range out {
			if isControl(r) {
				t.Errorf("Convert(%q) emitted %#U at byte %d:\n%q", in, r, i, out)
				break
			}
		}
	})
}
