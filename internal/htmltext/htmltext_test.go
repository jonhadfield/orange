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
