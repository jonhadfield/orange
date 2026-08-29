// Package htmltext renders the limited HTML found in Hacker News comments
// and story text (<p>, <i>/<em>, <a>, <pre><code>, <br>) as readable plain
// text for terminal display.
package htmltext

import (
	"html"
	"strings"
)

// Convert turns HN item HTML into plain text: entities are unescaped,
// <p> becomes a paragraph break, <i>/<em> become _underscore_ emphasis,
// links render as "text (url)" (or just the URL when the text is the URL),
// and <pre><code> blocks are indented.
func Convert(src string) string {
	return convert(src, false)
}

// ConvertLinked is Convert with each link additionally wrapped in an OSC 8
// terminal hyperlink, clickable in terminals that support them.
func ConvertLinked(src string) string {
	return convert(src, true)
}

func convert(src string, linkify bool) string {
	var (
		out    strings.Builder
		link   strings.Builder
		pre    strings.Builder
		href   string
		inLink bool
		inPre  bool
	)

	dst := func() *strings.Builder {
		switch {
		case inLink:
			return &link
		case inPre:
			return &pre
		default:
			return &out
		}
	}

	flushLink := func() {
		inLink = false
		text := strings.TrimSpace(link.String())
		var display string
		switch {
		case href == "":
			// No target to show, so the anchor text is all there is. Worth
			// its own case now that data-href no longer answers a lookup
			// for href, which would otherwise render as "text ()".
			display = text
		case text == "" || text == href || strings.HasSuffix(text, "..."):
			// HN link text is usually the (possibly truncated) URL itself.
			display = href
		default:
			display = text + " (" + href + ")"
		}
		if linkify && href != "" {
			// OSC 8 hyperlink, underlined so links are recognizable.
			// The underline stays outside the link markers: some
			// terminals (JetBrains) cannot parse links whose anchor
			// text contains SGR sequences.
			display = "\x1b[4m\x1b]8;;" + href + "\x1b\\" + display + "\x1b]8;;\x1b\\\x1b[24m"
		}
		dst().WriteString(display)
	}

	flushPre := func() {
		inPre = false
		out.WriteString(indent(strings.TrimRight(pre.String(), "\n"), "    "))
		out.WriteString("\n")
	}

	s := src
	for s != "" {
		lt := strings.IndexByte(s, '<')
		if lt < 0 {
			dst().WriteString(html.UnescapeString(s))
			break
		}
		if lt > 0 {
			dst().WriteString(html.UnescapeString(s[:lt]))
		}
		s = s[lt+1:]
		gt := strings.IndexByte(s, '>')
		if gt < 0 {
			dst().WriteString("<" + html.UnescapeString(s))
			break
		}
		rawTag := strings.TrimSpace(s[:gt])
		s = s[gt+1:]
		name, attrs := cutTagName(rawTag)
		switch strings.ToLower(name) {
		case "p", "/p":
			if !inPre && !inLink {
				out.WriteString("\n\n")
			}
		case "br", "br/":
			dst().WriteString("\n")
		case "i", "/i", "em", "/em":
			dst().WriteString("_")
		case "a":
			href = attrValue(attrs, "href")
			link.Reset()
			inLink = true
		case "/a":
			if inLink {
				flushLink()
			}
		case "pre":
			if !inLink {
				pre.Reset()
				inPre = true
			}
		case "/pre":
			if inPre {
				flushPre()
			}
		case "code", "/code", "b", "/b", "strong", "/strong", "u", "/u":
			// No terminal representation needed.
		}
	}

	// A missing </a> or </pre> must not swallow everything written since the
	// opening tag: what was collected is rendered as if the tag had closed.
	// Link first, so its text lands inside an unclosed <pre> rather than
	// after it.
	if inLink {
		flushLink()
	}
	if inPre {
		flushPre()
	}

	result := out.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// cutTagName splits a tag into its name and the attributes after it. HTML
// separates the two with any whitespace, not only a space: <a\nhref="x"> is
// an ordinary link, and cutting on " " alone made the name the whole of
// `a\nhref="x"`, which matched no case, so the tag and its attributes were
// dropped rather than rendered.
func cutTagName(tag string) (name, attrs string) {
	if i := strings.IndexAny(tag, " \t\r\n\f"); i >= 0 {
		return tag[:i], tag[i+1:]
	}
	return tag, ""
}

// attrValue extracts an attribute value from a tag's attribute list, e.g.
// attrValue(`href="https://x"`, "href") == "https://x". The value may be
// double-quoted, single-quoted or bare, and the name has to match a whole
// attribute, so that a lookup for href does not find data-href.
func attrValue(attrs, name string) string {
	// Matched in place rather than against a lowercased copy: lowercasing
	// can change a string's length, which would put the indices out of step
	// with the original.
	for i := 0; i+len(name) <= len(attrs); i++ {
		if !strings.EqualFold(attrs[i:i+len(name)], name) {
			continue
		}
		// Reject the tail of a longer name, so href does not match
		// data-href.
		if i > 0 && isAttrNameByte(attrs[i-1]) {
			continue
		}
		// And reject a name that only starts the same way: what follows
		// has to be the "=" of a value, not more of the name, which is
		// what separates href from hreflang.
		rest := strings.TrimLeft(attrs[i+len(name):], " \t\r\n")
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		return attrText(strings.TrimLeft(rest[1:], " \t\r\n"))
	}
	return ""
}

// attrText reads one attribute value from the start of s. An unterminated
// quote takes the rest of the tag rather than yielding nothing, on the same
// grounds as flushing an unclosed tag: salvage what is there.
func attrText(s string) string {
	if s == "" {
		return ""
	}
	if q := s[0]; q == '"' || q == '\'' {
		if end := strings.IndexByte(s[1:], q); end >= 0 {
			return html.UnescapeString(s[1 : 1+end])
		}
		return html.UnescapeString(s[1:])
	}
	end := strings.IndexAny(s, " \t\r\n")
	if end < 0 {
		end = len(s)
	}
	return html.UnescapeString(s[:end])
}

// isAttrNameByte reports whether c can appear in an HTML attribute name,
// which is how a whole-name match is told from the tail of a longer one.
func isAttrNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == ':' || c == '.'
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
