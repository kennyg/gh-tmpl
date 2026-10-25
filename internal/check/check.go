// Package check compares an issue or pull request body with its template.
//
// The check treats the template and the body as untrusted text. It reads
// them line by line and never renders, executes or evaluates them.
package check

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kennyg/gh-tmpl/internal/form"
)

// MaxBodySize is the largest body that the check reads.
const MaxBodySize = 1 << 20

// Severity tells whether a finding fails the check.
type Severity string

// The severities.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Finding codes.
const (
	CodeEmptyBody          = "empty-body"
	CodeMissingHeading     = "missing-heading"
	CodeEmptySection       = "empty-section"
	CodeUnchangedSection   = "unchanged-section"
	CodePlaceholderComment = "placeholder-comment"
	CodeUntouchedChecklist = "untouched-checklist"
	CodeUncheckedCheckbox  = "unchecked-checkbox"
	CodeRequiredField      = "required-field-empty"
	CodeRequiredCheckbox   = "required-checkbox"
	CodeUnknownOption      = "unknown-option"
)

// Finding is one problem in the body.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	// Section is the template heading that the finding is about.
	Section string `json:"section,omitempty"`
	// Line is the 1-based line in the body, or 0 if the finding has no line.
	Line int `json:"line,omitempty"`
	// Fix tells the writer what to change.
	Fix string `json:"fix"`
}

// Report is the result of a check.
type Report struct {
	OK       bool      `json:"ok"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Findings []Finding `json:"findings"`
}

// Options change how strict the check is.
type Options struct {
	// Strict turns every warning into an error.
	Strict bool
	// AllowUnchecked turns untouched checklists into warnings.
	AllowUnchecked bool
}

type checker struct {
	opts     Options
	findings []Finding
}

func (c *checker) add(sev Severity, code, section string, line int, msg, fix string) {
	if sev == Warning && c.opts.Strict {
		sev = Error
	}
	c.findings = append(c.findings, Finding{Severity: sev, Code: code, Section: section, Line: line, Message: msg, Fix: fix})
}

func (c *checker) report() Report {
	r := Report{Findings: c.findings}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	for _, f := range r.Findings {
		if f.Severity == Error {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
	r.OK = r.Errors == 0
	return r
}

// Markdown checks a body against a Markdown template.
func Markdown(template, body string, opts Options) Report {
	c := &checker{opts: opts}
	t, b := parse(template), parse(body)
	if strings.TrimSpace(body) == "" {
		c.add(Error, CodeEmptyBody, "", 0, "The body is empty.", "Start from the template: run \"gh tmpl show\" and fill in each section.")
		return c.report()
	}

	th := usable(t.headings)
	match := matchHeadings(th, b.headings)

	tEnds := headingLines(th)
	var bEnds []int
	for _, m := range match {
		if m >= 0 {
			bEnds = append(bEnds, b.headings[m].line)
		}
	}

	for i, h := range th {
		label := headingLabel(h)
		if match[i] < 0 {
			c.add(Error, CodeMissingHeading, label, 0,
				fmt.Sprintf("The heading %q is missing.", label),
				fmt.Sprintf("Add the heading %q and write that section. Keep the heading text as it is in the template.", label))
			continue
		}
		tFrom, tTo := t.section(h.line, tEnds)
		tContent := t.content(tFrom, tTo)
		if len(tContent) == 0 && !t.hasComments(tFrom, tTo) {
			continue
		}
		bh := b.headings[match[i]]
		bFrom, bTo := b.section(bh.line, bEnds)
		bContent := b.content(bFrom, bTo)
		switch {
		case len(bContent) == 0:
			c.add(Error, CodeEmptySection, label, bh.line+1,
				fmt.Sprintf("The section %q is empty.", label),
				fmt.Sprintf("Write the %q section. If it does not apply, write \"N/A\" and the reason.", label))
		case hasProse(t, tFrom, tTo) && equal(tContent, bContent):
			c.add(Error, CodeUnchangedSection, label, bh.line+1,
				fmt.Sprintf("The section %q still holds the template's example text.", label),
				fmt.Sprintf("Replace the example text under %q with your own.", label))
		}
	}

	c.comments(t, b)
	c.checklists(t, b, th)
	return c.report()
}

// usable drops headings with no text, which cannot be matched.
func usable(hs []heading) []heading {
	var out []heading
	for _, h := range hs {
		if h.key != "" {
			out = append(out, h)
		}
	}
	return out
}

func headingLines(hs []heading) []int {
	out := make([]int, len(hs))
	for i, h := range hs {
		out[i] = h.line
	}
	return out
}

// matchHeadings maps each template heading to the index of the first
// unused body heading with the same text, or to -1. Heading levels and
// order do not matter.
func matchHeadings(th, bh []heading) []int {
	used := make([]bool, len(bh))
	out := make([]int, len(th))
	for i, h := range th {
		out[i] = -1
		for j, b := range bh {
			if !used[j] && b.key == h.key {
				used[j] = true
				out[i] = j
				break
			}
		}
	}
	return out
}

func headingLabel(h heading) string {
	return strings.Repeat("#", h.level) + " " + h.text
}

// hasProse reports whether the template section holds text that is not a
// task list item.
func hasProse(d *doc, from, to int) bool {
	for i := from; i < to; i++ {
		l := d.lines[i]
		if normalize(l.visible) == "" && !l.fenced {
			continue
		}
		if !l.fenced && checkboxRE.MatchString(l.visible) {
			continue
		}
		return true
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// directiveRE matches comments that configure tools, such as
// "<!-- markdownlint-disable -->". Such comments may stay in the body.
var directiveRE = regexp.MustCompile(`(?i)^(markdownlint|prettier-ignore|cspell|vale|textlint|toc|all-contributors|lint|eslint|remark)`)

func (c *checker) comments(t, b *doc) {
	placeholders := map[string]bool{}
	for _, cm := range t.comments {
		if cm.text != "" && !directiveRE.MatchString(cm.text) {
			placeholders[cm.text] = true
		}
	}
	for _, cm := range b.comments {
		if !placeholders[cm.text] {
			continue
		}
		short := truncate(cm.text)
		c.add(Error, CodePlaceholderComment, sectionAt(b, cm.line), cm.line+1,
			fmt.Sprintf("The template comment \"<!-- %s -->\" is still in the body.", short),
			fmt.Sprintf("Follow the comment on line %d, then delete it.", cm.line+1))
	}
}

// sectionAt returns the heading above line i, for context in a finding.
func sectionAt(d *doc, i int) string {
	label := ""
	for _, h := range d.headings {
		if h.line > i {
			break
		}
		label = headingLabel(h)
	}
	return label
}

// maxQuote is the longest piece of template or body text that a finding
// quotes.
const maxQuote = 80

func truncate(s string) string {
	r := []rune(s)
	if len(r) <= maxQuote {
		return s
	}
	return string(r[:maxQuote]) + "..."
}

func (c *checker) checklists(t, b *doc, th []heading) {
	if len(t.checkboxes) == 0 {
		return
	}
	// Group the template checkboxes by the section that holds them.
	type group struct {
		section string
		items   []checkbox
	}
	var groups []*group
	byLabel := map[string]*group{}
	for _, cb := range t.checkboxes {
		label := ""
		for _, h := range th {
			if h.line > cb.line {
				break
			}
			label = headingLabel(h)
		}
		g, ok := byLabel[label]
		if !ok {
			g = &group{section: label}
			byLabel[label] = g
			groups = append(groups, g)
		}
		g.items = append(g.items, cb)
	}

	used := make([]bool, len(b.checkboxes))
	find := func(key string) int {
		for j, bc := range b.checkboxes {
			if !used[j] && bc.key == key {
				used[j] = true
				return j
			}
		}
		return -1
	}

	for _, g := range groups {
		var untouched []checkbox
		var lines []int
		touched := false
		for _, item := range g.items {
			j := find(item.key)
			if j < 0 || b.checkboxes[j].checked {
				// Deleted or checked: the writer looked at it.
				touched = true
				continue
			}
			untouched = append(untouched, item)
			lines = append(lines, b.checkboxes[j].line+1)
		}
		if len(untouched) == 0 {
			continue
		}
		where := "the checklist"
		if g.section != "" {
			where = fmt.Sprintf("the checklist under %q", g.section)
		}
		if !touched {
			sev := Error
			if c.opts.AllowUnchecked {
				sev = Warning
			}
			c.add(sev, CodeUntouchedChecklist, g.section, lines[0],
				fmt.Sprintf("No item in %s is checked or removed (%d items).", where, len(untouched)),
				"Check each item that applies with \"- [x]\". Delete each item that does not apply.")
			continue
		}
		for i, item := range untouched {
			c.add(Warning, CodeUncheckedCheckbox, g.section, lines[i],
				fmt.Sprintf("The item \"- [ ] %s\" is not checked.", truncate(normalize(item.text))),
				"Check it with \"- [x]\" if it applies. Delete the line if it does not.")
		}
	}
}

// Form checks a body against an issue form. The body must have the layout
// that GitHub writes for a submitted form: one "### <label>" section per
// field.
func Form(f *form.Form, body string, opts Options) Report {
	c := &checker{opts: opts}
	if strings.TrimSpace(body) == "" {
		c.add(Error, CodeEmptyBody, "", 0, "The body is empty.", "Start from the form: run \"gh tmpl show issue --form <name>\" and fill in each section.")
		return c.report()
	}
	b := parse(body)
	fields := f.Fields()

	th := make([]heading, len(fields))
	for i, fl := range fields {
		th[i] = heading{level: 3, text: fl.Label, key: headingKey(fl.Label)}
	}
	match := matchHeadings(th, b.headings)
	var bEnds []int
	for _, m := range match {
		if m >= 0 {
			bEnds = append(bEnds, b.headings[m].line)
		}
	}

	for i, fl := range fields {
		label := headingLabel(th[i])
		if match[i] < 0 {
			c.add(Error, CodeMissingHeading, label, 0,
				fmt.Sprintf("The heading %q is missing.", label),
				fmt.Sprintf("Add the heading %q. Every form field needs its own section.", label))
			continue
		}
		bh := b.headings[match[i]]
		from, to := b.section(bh.line, bEnds)
		switch fl.Type {
		case form.TypeCheckboxes:
			c.formCheckboxes(b, fl, label, from, to)
		default:
			values := answer(b, from, to)
			if fl.Type == form.TypeDropdown && len(values) == 1 && values[0] == "None" {
				// GitHub writes "None" for a dropdown with no choice.
				values = nil
			}
			if len(values) == 0 {
				if fl.Required {
					c.add(Error, CodeRequiredField, label, bh.line+1,
						fmt.Sprintf("The required field %q is empty.", label),
						fmt.Sprintf("Fill in the %q section. The form requires it.", label))
				}
				continue
			}
			if fl.Type == form.TypeDropdown {
				c.dropdown(fl, label, bh.line+1, values)
			}
		}
	}

	c.comments(parse(f.Render()), b)
	return c.report()
}

// answer returns the content of a form section. Bare code fence lines and
// GitHub's "_No response_" marker do not count.
func answer(d *doc, from, to int) []string {
	var out []string
	for i := from; i < to; i++ {
		l := d.lines[i]
		text := l.visible
		if l.fenced {
			if fenceRE.MatchString(l.raw) {
				continue
			}
			text = l.raw
		}
		t := normalize(text)
		if t == "" || t == form.NoResponse {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (c *checker) dropdown(fl form.Field, label string, line int, values []string) {
	valid := map[string]bool{}
	names := make([]string, len(fl.Options))
	for i, o := range fl.Options {
		valid[strings.ToLower(normalize(o.Label))] = true
		names[i] = o.Label
	}
	var picked []string
	if fl.Multiple {
		for _, v := range strings.Split(strings.Join(values, ","), ",") {
			if v = strings.TrimSpace(v); v != "" {
				picked = append(picked, v)
			}
		}
	} else {
		picked = []string{strings.Join(values, " ")}
	}
	for _, v := range picked {
		if !valid[strings.ToLower(v)] {
			c.add(Warning, CodeUnknownOption, label, line,
				fmt.Sprintf("%q is not one of the options for %q.", truncate(v), label),
				"Use one of: "+strings.Join(names, ", ")+".")
		}
	}
}

func (c *checker) formCheckboxes(b *doc, fl form.Field, label string, from, to int) {
	for _, o := range fl.Options {
		if !o.Required {
			continue
		}
		key := strings.ToLower(normalize(o.Label))
		checked := false
		for _, cb := range b.checkboxes {
			if cb.line >= from && cb.line < to && cb.key == key && cb.checked {
				checked = true
				break
			}
		}
		if !checked {
			c.add(Error, CodeRequiredCheckbox, label, from,
				fmt.Sprintf("The required item %q under %q is not checked.", truncate(normalize(o.Label)), label),
				fmt.Sprintf("Check \"- [x] %s\". The form requires it.", truncate(normalize(o.Label))))
		}
	}
}
