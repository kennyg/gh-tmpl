package form

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseAndRender(t *testing.T) {
	f, err := Parse(readFixture(t, "bug_report.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Bug report" || f.Title != "[Bug]: " || strings.Join(f.Labels, ",") != "bug,triage" {
		t.Errorf("metadata = %+v", f)
	}
	fields := f.Fields()
	if len(fields) != 5 {
		t.Fatalf("fields = %+v", fields)
	}
	if !fields[0].Required || fields[1].Required || !fields[2].Multiple {
		t.Errorf("fields = %+v", fields)
	}

	got := f.Render()
	golden := filepath.Join("testdata", "bug_report.golden.md")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := string(readFixture(t, "bug_report.golden.md"))
	if got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderDefaults(t *testing.T) {
	f, err := Parse([]byte(`
name: x
body:
  - type: dropdown
    attributes:
      label: Pick
      options: [a, b]
      default: 1
  - type: textarea
    attributes:
      label: Notes
      value: |
        Prefilled
  - type: input
    attributes:
      label: "Tricky --> label"
      description: "ends <!-- early -->"
`))
	if err != nil {
		t.Fatal(err)
	}
	got := f.Render()
	for _, want := range []string{"### Pick\n\n<!-- Optional. Choose one: a, b. -->\nb\n", "Prefilled\n", "<!-- Optional. ends < !-- early -- > -->"} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() is missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "<!--") != 3 || strings.Count(got, "-->") != 4 {
		t.Errorf("form text closed a comment early:\n%s", got)
	}
}

func TestStringList(t *testing.T) {
	f, err := Parse([]byte("name: x\nlabels: \"bug, needs triage\"\nassignees: [a, b]\nbody:\n  - type: input\n    attributes: {label: L}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.Labels, "|") != "bug|needs triage" || strings.Join(f.Assignees, "|") != "a|b" {
		t.Errorf("labels = %q, assignees = %q", f.Labels, f.Assignees)
	}
}

func TestParseRejects(t *testing.T) {
	bomb := `a: &a ["x","x","x","x","x","x","x","x","x"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f]
h: &h [*g,*g,*g,*g,*g,*g,*g,*g,*g]
i: &i [*h,*h,*h,*h,*h,*h,*h,*h,*h]
body: *i
`
	tests := map[string]string{
		"not YAML":       "body: [",
		"no body":        "name: x\n",
		"unknown type":   "name: x\nbody:\n  - type: script\n    attributes: {label: L}\n",
		"missing label":  "name: x\nbody:\n  - type: input\n",
		"empty dropdown": "name: x\nbody:\n  - type: dropdown\n    attributes: {label: L}\n",
		"alias bomb":     bomb,
		"too large":      "name: x\n#" + strings.Repeat("a", MaxSize) + "\n",
		"wrong shape":    "- just\n- a list\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(in)); err == nil {
				t.Fatal("Parse returned no error")
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add(readFixtureF(f, "bug_report.yml"))
	f.Add([]byte("name: x\nbody:\n  - type: input\n    attributes: {label: L}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		form, err := Parse(data)
		if err == nil {
			_ = form.Render()
		}
	})
}

func readFixtureF(f *testing.F, name string) []byte {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		f.Fatal(err)
	}
	return data
}
