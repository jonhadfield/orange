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
		name, attrs, _ := strings.Cut(rawTag, " ")
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
				inPre = false
				out.WriteString(indent(strings.TrimRight(pre.String(), "\n"), "    "))
				out.WriteString("\n")
			}
		case "code", "/code", "b", "/b", "strong", "/strong", "u", "/u":
			// No terminal representation needed.
		}
	}

	result := out.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// attrValue extracts a double-quoted attribute value from a tag's attribute
// list, e.g. attrValue(`href="https://x"`, "href") == "https://x".
func attrValue(attrs, name string) string {
	i := strings.Index(strings.ToLower(attrs), name+`="`)
	if i < 0 {
		return ""
	}
	start := i + len(name) + 2
	end := strings.IndexByte(attrs[start:], '"')
	if end < 0 {
		return ""
	}
	return html.UnescapeString(attrs[start : start+end])
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
