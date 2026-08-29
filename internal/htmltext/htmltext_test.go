package htmltext

import "testing"

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
