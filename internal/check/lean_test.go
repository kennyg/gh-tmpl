package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/kennyg/gh-tmpl/internal/leantest"
)

type leanFinding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Section  string   `json:"section"`
	Line     int      `json:"line"`
}

type leanCheckCase struct {
	Template       []string      `json:"template"`
	Body           []string      `json:"body"`
	Strict         bool          `json:"strict"`
	AllowUnchecked bool          `json:"allowUnchecked"`
	Findings       []leanFinding `json:"findings"`
}

// TestMarkdownMatchesLean checks Markdown against the model in
// verify/GhTmpl/Check.lean. The model gives the findings, in order, for
// each generated template and body.
func TestMarkdownMatchesLean(t *testing.T) {
	leantest.Each(t, "check.jsonl", func(t *testing.T, line int, c leanCheckCase) {
		tmpl, body := strings.Join(c.Template, "\n"), strings.Join(c.Body, "\n")
		r := Markdown(tmpl, body, Options{Strict: c.Strict, AllowUnchecked: c.AllowUnchecked})
		got := make([]leanFinding, len(r.Findings))
		for i, f := range r.Findings {
			got[i] = leanFinding{Severity: f.Severity, Code: f.Code, Section: f.Section, Line: f.Line}
		}
		if c.Findings == nil {
			c.Findings = []leanFinding{}
		}
		if !slices.Equal(got, c.Findings) {
			t.Errorf("case %d: strict %v, allow-unchecked %v\ntemplate:\n%s\nbody:\n%s\ngot  %v\nwant %v",
				line, c.Strict, c.AllowUnchecked, tmpl, body, got, c.Findings)
		}
	})
}
