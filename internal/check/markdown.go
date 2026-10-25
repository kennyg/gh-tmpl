package check

import (
	"regexp"
	"strings"
)

// The scanner reads only the parts of Markdown that the check needs:
// ATX headings, task list items, HTML comments and fenced code blocks.
// It works line by line and never renders or evaluates the text.

var (
	headingRE  = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	checkboxRE = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d{1,9}[.)])[ \t]+\[([ xX])\][ \t]+(.*?)[ \t]*$`)
	fenceRE    = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

type heading struct {
	level int
	text  string // as written
	key   string // normalized for matching
	line  int    // index into doc.lines
}

type checkbox struct {
	checked bool
	text    string
	key     string
	line    int
}

type comment struct {
	text string // normalized
	line int    // index of the line where the comment starts
}

type line struct {
	raw     string
	visible string // the line with HTML comments removed
	fenced  bool   // inside a fenced code block, fence lines included
}

type doc struct {
	lines      []line
	headings   []heading
	checkboxes []checkbox
	comments   []comment
}

func parse(s string) *doc {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	d := &doc{lines: make([]line, len(raw))}

	var fence string
	inComment := false
	var cur strings.Builder
	start := 0

	for i, r := range raw {
		l := line{raw: r}
		if fence == "" && !inComment {
			if m := fenceRE.FindStringSubmatch(r); m != nil {
				fence = m[1]
				l.fenced = true
				d.lines[i] = l
				continue
			}
		} else if fence != "" {
			l.fenced = true
			t := strings.TrimLeft(r, " ")
			if strings.HasPrefix(t, fence[:1]) && strings.Trim(t, fence[:1]+" \t") == "" &&
				len(strings.TrimRight(t, " \t")) >= len(fence) {
				fence = ""
			}
			d.lines[i] = l
			continue
		}

		// Split the line into visible text and comment text.
		var vis strings.Builder
		rest := r
		for rest != "" {
			if inComment {
				end := strings.Index(rest, "-->")
				if end < 0 {
					cur.WriteString(rest)
					cur.WriteString("\n")
					rest = ""
					break
				}
				cur.WriteString(rest[:end])
				d.comments = append(d.comments, comment{text: normalize(cur.String()), line: start})
				cur.Reset()
				inComment = false
				rest = rest[end+3:]
				continue
			}
			open := strings.Index(rest, "<!--")
			if open < 0 {
				vis.WriteString(rest)
				break
			}
			vis.WriteString(rest[:open])
			rest = rest[open+4:]
			inComment = true
			start = i
		}
		l.visible = vis.String()
		d.lines[i] = l

		if m := headingRE.FindStringSubmatch(l.visible); m != nil {
			d.headings = append(d.headings, heading{
				level: len(m[1]),
				text:  strings.TrimSpace(m[2]),
				key:   headingKey(m[2]),
				line:  i,
			})
			continue
		}
		if m := checkboxRE.FindStringSubmatch(l.visible); m != nil {
			d.checkboxes = append(d.checkboxes, checkbox{
				checked: m[1] != " ",
				text:    m[2],
				key:     strings.ToLower(normalize(m[2])),
				line:    i,
			})
		}
	}
	if inComment {
		// An open comment runs to the end of the document, as in HTML.
		d.comments = append(d.comments, comment{text: normalize(cur.String()), line: start})
	}
	return d
}

// normalize collapses all white space to single spaces.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// headingKey normalizes a heading for matching. Case, emphasis markers,
// extra spaces and a trailing colon do not matter.
func headingKey(s string) string {
	s = strings.NewReplacer("*", "", "_", "", "`", "").Replace(s)
	s = strings.ToLower(normalize(s))
	return strings.TrimSpace(strings.TrimSuffix(s, ":"))
}

// section returns the line range of the section that starts at the heading
// on line start. The section ends at the next line in ends, or at the end
// of the document.
func (d *doc) section(start int, ends []int) (int, int) {
	end := len(d.lines)
	for _, e := range ends {
		if e > start && e < end {
			end = e
		}
	}
	return start + 1, end
}

// content returns the visible text of lines [from, to), with blank lines
// removed and white space normalized. Fenced lines count as content.
func (d *doc) content(from, to int) []string {
	var out []string
	for _, l := range d.lines[from:to] {
		text := l.visible
		if l.fenced {
			text = l.raw
		}
		if t := normalize(text); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// hasComments reports whether any comment starts in lines [from, to).
func (d *doc) hasComments(from, to int) bool {
	for _, c := range d.comments {
		if c.line >= from && c.line < to && c.text != "" {
			return true
		}
	}
	return false
}
