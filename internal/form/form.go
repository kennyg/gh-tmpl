// Package form parses GitHub issue forms and renders them as Markdown.
//
// An issue form is a YAML file in .github/ISSUE_TEMPLATE. When someone
// submits a form, GitHub writes the issue body as one "### <label>" section
// per field. Render produces that same layout with empty sections, so an
// agent can fill it and pass it to "gh issue create --body-file".
package form

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxSize is the largest form file that Parse accepts.
const MaxSize = 256 << 10

// maxElements limits the number of body elements in one form.
const maxElements = 200

// maxOptions limits the number of options in one dropdown or checkbox list.
const maxOptions = 200

// NoResponse is the text GitHub writes for an optional field left empty.
const NoResponse = "_No response_"

// Element types that GitHub supports in issue forms.
const (
	TypeMarkdown   = "markdown"
	TypeInput      = "input"
	TypeTextarea   = "textarea"
	TypeDropdown   = "dropdown"
	TypeCheckboxes = "checkboxes"
)

// Form is a parsed issue form.
type Form struct {
	Name        string     `yaml:"name" json:"name"`
	Description string     `yaml:"description" json:"description,omitempty"`
	Title       string     `yaml:"title" json:"title,omitempty"`
	Labels      StringList `yaml:"labels" json:"labels,omitempty"`
	Assignees   StringList `yaml:"assignees" json:"assignees,omitempty"`
	Projects    StringList `yaml:"projects" json:"projects,omitempty"`
	Type        string     `yaml:"type" json:"type,omitempty"`
	Body        []Element  `yaml:"body" json:"-"`
}

// Element is one item in the form body.
type Element struct {
	Type        string      `yaml:"type"`
	ID          string      `yaml:"id"`
	Attributes  Attributes  `yaml:"attributes"`
	Validations Validations `yaml:"validations"`
}

// Attributes holds the attributes of an element. Each element type uses a
// subset of them.
type Attributes struct {
	Label       string   `yaml:"label"`
	Description string   `yaml:"description"`
	Placeholder string   `yaml:"placeholder"`
	Value       string   `yaml:"value"`
	Render      string   `yaml:"render"`
	Multiple    bool     `yaml:"multiple"`
	Default     *int     `yaml:"default"`
	Options     []Option `yaml:"options"`
}

// Validations holds the validation rules of an element.
type Validations struct {
	Required bool `yaml:"required"`
}

// Option is a dropdown choice or a checkbox. Dropdown options are plain
// strings in YAML. Checkbox options are mappings with a label.
type Option struct {
	Label    string `yaml:"label"`
	Required bool   `yaml:"required"`
}

// UnmarshalYAML accepts a string or a mapping.
func (o *Option) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		o.Label = n.Value
		return nil
	}
	type plain Option
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*o = Option(p)
	return nil
}

// StringList is a YAML list of strings that also accepts one
// comma-separated string, as GitHub does for labels and assignees.
type StringList []string

// UnmarshalYAML accepts a string or a sequence of strings.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*s = nil
		for _, part := range strings.Split(n.Value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				*s = append(*s, part)
			}
		}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

// Parse decodes and validates an issue form. It never executes or evaluates
// the content. It rejects input larger than MaxSize and converts a parser
// panic into an error.
func Parse(data []byte) (f *Form, err error) {
	if len(data) > MaxSize {
		return nil, fmt.Errorf("form is larger than %d bytes", MaxSize)
	}
	defer func() {
		if r := recover(); r != nil {
			f, err = nil, fmt.Errorf("invalid YAML: %v", r)
		}
	}()
	var out Form
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := out.validate(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (f *Form) validate() error {
	if len(f.Body) == 0 {
		return errors.New("form has no body elements")
	}
	if len(f.Body) > maxElements {
		return fmt.Errorf("form has more than %d body elements", maxElements)
	}
	for i, e := range f.Body {
		switch e.Type {
		case TypeMarkdown:
			continue
		case TypeInput, TypeTextarea, TypeDropdown, TypeCheckboxes:
		default:
			return fmt.Errorf("body element %d: unknown type %q", i+1, e.Type)
		}
		if strings.TrimSpace(e.Attributes.Label) == "" {
			return fmt.Errorf("body element %d (%s): missing label", i+1, e.Type)
		}
		if len(e.Attributes.Options) > maxOptions {
			return fmt.Errorf("body element %d (%s): more than %d options", i+1, e.Type, maxOptions)
		}
		if (e.Type == TypeDropdown || e.Type == TypeCheckboxes) && len(e.Attributes.Options) == 0 {
			return fmt.Errorf("body element %d (%s): no options", i+1, e.Type)
		}
	}
	return nil
}

// Field is a form element that produces a section in the issue body.
type Field struct {
	Type     string
	ID       string
	Label    string
	Required bool
	Multiple bool
	Options  []Option
}

// Fields returns the elements that GitHub writes into the issue body, in
// order. Markdown elements are only shown on the form, so they are left out.
func (f *Form) Fields() []Field {
	var out []Field
	for _, e := range f.Body {
		if e.Type == TypeMarkdown {
			continue
		}
		out = append(out, Field{
			Type:     e.Type,
			ID:       e.ID,
			Label:    strings.TrimSpace(e.Attributes.Label),
			Required: e.Validations.Required,
			Multiple: e.Attributes.Multiple,
			Options:  e.Attributes.Options,
		})
	}
	return out
}

// Render returns the issue body skeleton for the form as Markdown.
// Each field becomes a "### <label>" heading followed by an HTML comment
// that tells the writer what to put there.
func (f *Form) Render() string {
	var b strings.Builder
	first := true
	for _, e := range f.Body {
		if e.Type == TypeMarkdown {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		a := e.Attributes
		fmt.Fprintf(&b, "### %s\n\n", oneLine(a.Label))
		b.WriteString(comment(hint(e)))
		b.WriteString("\n")
		switch e.Type {
		case TypeCheckboxes:
			for _, o := range a.Options {
				fmt.Fprintf(&b, "- [ ] %s\n", oneLine(o.Label))
			}
		case TypeDropdown:
			if a.Default != nil && *a.Default >= 0 && *a.Default < len(a.Options) {
				fmt.Fprintf(&b, "%s\n", oneLine(a.Options[*a.Default].Label))
			}
		default:
			if v := strings.TrimRight(a.Value, "\n"); v != "" {
				fmt.Fprintf(&b, "%s\n", v)
			}
		}
	}
	return b.String()
}

func hint(e Element) string {
	a := e.Attributes
	var parts []string
	required := e.Validations.Required
	for _, o := range a.Options {
		required = required || (e.Type == TypeCheckboxes && o.Required)
	}
	if required {
		parts = append(parts, "Required.")
	} else {
		parts = append(parts, "Optional.")
	}
	if d := strings.TrimSpace(a.Description); d != "" {
		parts = append(parts, d)
	}
	switch e.Type {
	case TypeDropdown:
		labels := make([]string, len(a.Options))
		for i, o := range a.Options {
			labels[i] = o.Label
		}
		if a.Multiple {
			parts = append(parts, sentence("Choose one or more, separated by commas: "+strings.Join(labels, ", ")))
		} else {
			parts = append(parts, sentence("Choose one: "+strings.Join(labels, ", ")))
		}
	case TypeCheckboxes:
		var req []string
		for _, o := range a.Options {
			if o.Required {
				req = append(req, o.Label)
			}
		}
		if len(req) > 0 {
			parts = append(parts, sentence("You must check: "+strings.Join(req, "; ")))
		}
	case TypeInput, TypeTextarea:
		if p := strings.TrimSpace(a.Placeholder); p != "" {
			parts = append(parts, "Example: "+p)
		}
		if a.Render != "" {
			parts = append(parts, "Put the content in a "+a.Render+" code block.")
		}
	}
	return strings.Join(parts, " ")
}

// sentence ends s with exactly one period.
func sentence(s string) string {
	return strings.TrimRight(s, ". ") + "."
}

// comment wraps text in an HTML comment. It breaks up "-->" in the text so
// that form content cannot end the comment early.
func comment(text string) string {
	text = strings.ReplaceAll(text, "-->", "-- >")
	text = strings.ReplaceAll(text, "<!--", "< !--")
	return "<!-- " + strings.TrimSpace(text) + " -->"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
