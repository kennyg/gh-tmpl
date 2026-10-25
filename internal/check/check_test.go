package check

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kennyg/gh-tmpl/internal/form"
)

func fixture(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// codes returns "severity:code" for each finding, sorted.
func codes(r Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, string(f.Severity)+":"+f.Code)
	}
	sort.Strings(out)
	return out
}

func TestMarkdown(t *testing.T) {
	tmpl := fixture(t, "pr_template.md")
	tests := []struct {
		name string
		body string
		opts Options
		ok   bool
		want []string
	}{
		{
			name: "filled in",
			body: "## Summary\n\nFix the login loop.\n\n## Testing\n\nRan go test ./...\n\n## Checklist\n\n- [x] Tests added or updated\n- [x] Docs updated\n",
			ok:   true,
		},
		{
			name: "heading level, case and order do not matter",
			body: "# testing:\n\nManual run.\n\n### **summary**\n\nFix.\n\n## Checklist\n\n- [x] tests added or updated\n",
			ok:   true,
		},
		{
			name: "deleted items count as touched",
			body: "## Summary\n\nFix.\n\n## Testing\n\nRan it.\n\n## Checklist\n\n- [x] Tests added or updated\n",
			ok:   true,
		},
		{
			name: "one unchecked item is a warning",
			body: "## Summary\n\nFix.\n\n## Testing\n\nRan it.\n\n## Checklist\n\n- [x] Tests added or updated\n- [ ] Docs updated\n",
			ok:   true,
			want: []string{"warning:unchecked-checkbox"},
		},
		{
			name: "strict turns warnings into errors",
			body: "## Summary\n\nFix.\n\n## Testing\n\nRan it.\n\n## Checklist\n\n- [x] Tests added or updated\n- [ ] Docs updated\n",
			opts: Options{Strict: true},
			want: []string{"error:unchecked-checkbox"},
		},
		{
			name: "the raw template fails",
			body: tmpl,
			want: []string{"error:empty-section", "error:empty-section", "error:placeholder-comment", "error:placeholder-comment", "error:untouched-checklist"},
		},
		{
			name: "allow unchecked",
			body: "## Summary\n\nFix.\n\n## Testing\n\nRan it.\n\n## Checklist\n\n- [ ] Tests added or updated\n- [ ] Docs updated\n",
			opts: Options{AllowUnchecked: true},
			ok:   true,
			want: []string{"warning:untouched-checklist"},
		},
		{
			name: "missing heading and empty section",
			body: "## Summary\n\n<!-- my own note -->\n\n## Checklist\n\n- [x] Tests added or updated\n",
			want: []string{"error:empty-section", "error:missing-heading"},
		},
		{
			name: "an added heading stays inside the section",
			body: "## Summary\n\n### Screenshots\n\nimg\n\n## Testing\n\nRan it.\n\n## Checklist\n\n- [x] Tests added or updated\n",
			ok:   true,
		},
		{
			name: "headings and comments in code blocks do not count",
			body: "## Summary\n\n```md\n## Testing\n<!-- How did you test it? -->\n```\n\n## Checklist\n\n- [x] Docs updated\n",
			want: []string{"error:missing-heading"},
		},
		{
			name: "empty body",
			body: " \n\n",
			want: []string{"error:empty-body"},
		},
		{
			name: "CRLF line endings",
			body: "## Summary\r\n\r\nFix.\r\n\r\n## Testing\r\n\r\nRan it.\r\n\r\n## Checklist\r\n\r\n- [X] Tests added or updated\r\n",
			ok:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Markdown(tmpl, tt.body, tt.opts)
			if r.OK != tt.ok {
				t.Errorf("OK = %v, want %v; findings: %+v", r.OK, tt.ok, r.Findings)
			}
			if got := codes(r); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("codes = %v, want %v", got, tt.want)
			}
			for _, f := range r.Findings {
				if f.Message == "" || f.Fix == "" {
					t.Errorf("finding without message or fix: %+v", f)
				}
			}
		})
	}
}

func TestUnchangedSection(t *testing.T) {
	tmpl := "## What\n\nDescribe the change here.\n\n## Why\n\nExplain.\n"
	r := Markdown(tmpl, "## What\n\nDescribe the change here.\n\n## Why\n\nBecause.\n", Options{})
	if got := codes(r); strings.Join(got, ",") != "error:unchanged-section" {
		t.Fatalf("codes = %v", got)
	}
	if r.Findings[0].Section != "## What" || r.Findings[0].Line != 1 {
		t.Errorf("finding = %+v", r.Findings[0])
	}
}

func TestDirectiveCommentsMayStay(t *testing.T) {
	tmpl := "<!-- markdownlint-disable MD041 -->\n## What\n\n<!-- Describe it. -->\n"
	r := Markdown(tmpl, "<!-- markdownlint-disable MD041 -->\n## What\n\nA fix.\n", Options{})
	if !r.OK || len(r.Findings) != 0 {
		t.Fatalf("findings = %+v", r.Findings)
	}
}

func TestMultiLineComment(t *testing.T) {
	tmpl := "## What\n\n<!--\n  Describe the change.\n  Link the issue.\n-->\n"
	body := "## What\n\nA fix.\n\n<!-- Describe the change.\nLink the issue. -->\n"
	r := Markdown(tmpl, body, Options{})
	if got := codes(r); strings.Join(got, ",") != "error:placeholder-comment" {
		t.Fatalf("codes = %v", got)
	}
	if r.Findings[0].Line != 5 {
		t.Errorf("line = %d, want 5", r.Findings[0].Line)
	}
}

func TestTemplateWithoutHeadings(t *testing.T) {
	tmpl := "<!-- Describe the change. -->\n\n- [ ] I ran the tests\n"
	if r := Markdown(tmpl, "A fix.\n\n- [x] I ran the tests\n", Options{}); !r.OK {
		t.Errorf("findings = %+v", r.Findings)
	}
	if r := Markdown(tmpl, "A fix.\n\n- [ ] I ran the tests\n", Options{}); r.OK {
		t.Error("untouched checklist passed")
	}
}

func TestForm(t *testing.T) {
	f, err := form.Parse([]byte(fixture(t, "bug_report.yml")))
	if err != nil {
		t.Fatal(err)
	}
	good := `### What happened?

It exits with code 2.

### Version

_No response_

### Operating system

macOS, Linux

### Logs

` + "```shell\nerror: boom\n```" + `

### Code of conduct

- [X] I agree to follow the code of conduct.
- [ ] I searched for existing issues.
`
	tests := []struct {
		name string
		body string
		ok   bool
		want []string
	}{
		{name: "filled in", body: good, ok: true},
		{name: "the rendered skeleton fails", body: f.Render(), want: []string{
			"error:placeholder-comment", "error:placeholder-comment", "error:placeholder-comment",
			"error:placeholder-comment", "error:placeholder-comment",
			"error:required-checkbox", "error:required-field-empty", "error:required-field-empty",
		}},
		{name: "unknown option", body: strings.Replace(good, "macOS, Linux", "macOS, BeOS", 1), ok: true, want: []string{"warning:unknown-option"}},
		{name: "required field says no response", body: strings.Replace(good, "It exits with code 2.", "_No response_", 1), want: []string{"error:required-field-empty"}},
		{name: "dropdown None", body: strings.Replace(good, "macOS, Linux", "None", 1), want: []string{"error:required-field-empty"}},
		{name: "empty code block does not count", body: strings.Replace(good, "It exits with code 2.", "```\n```", 1), want: []string{"error:required-field-empty"}},
		{name: "missing section", body: strings.Replace(good, "### Logs\n\n```shell\nerror: boom\n```\n\n", "", 1), want: []string{"error:missing-heading"}},
		{name: "required box unchecked", body: strings.Replace(good, "[X] I agree", "[ ] I agree", 1), want: []string{"error:required-checkbox"}},
		{name: "empty", body: "", want: []string{"error:empty-body"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Form(f, tt.body, Options{})
			if r.OK != tt.ok {
				t.Errorf("OK = %v, want %v; findings: %+v", r.OK, tt.ok, r.Findings)
			}
			if got := codes(r); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("codes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFences(t *testing.T) {
	d := parse("~~~~\n```\n# not a heading\n~~~~\n# heading\n````\n# in fence\n```\nstill fenced\n````\n")
	if len(d.headings) != 1 || d.headings[0].text != "heading" {
		t.Fatalf("headings = %+v", d.headings)
	}
}

func TestHeadingKey(t *testing.T) {
	for in, want := range map[string]string{
		"Summary":            "summary",
		"  **Summary**:  ":   "summary",
		"What `gh` does":     "what gh does",
		"Tests  and   docs":  "tests and docs",
		"Changes in C#":      "changes in c#",
		"_Breaking_ changes": "breaking changes",
	} {
		if got := headingKey(in); got != want {
			t.Errorf("headingKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzMarkdown(f *testing.F) {
	f.Add(fixture(f, "pr_template.md"), "## Summary\n\nx\n")
	f.Add("<!--\n## a\n```\n", "```\n<!-- -->\n# b")
	f.Fuzz(func(t *testing.T, tmpl, body string) {
		r := Markdown(tmpl, body, Options{})
		if r.OK != (r.Errors == 0) {
			t.Fatalf("OK = %v with %d errors", r.OK, r.Errors)
		}
	})
}
