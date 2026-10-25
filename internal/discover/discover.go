// Package discover finds the pull request and issue templates of a
// repository. It follows the rules that GitHub uses, and it reads the
// owner's shared ".github" repository when the repository has none.
//
// Discovery only reads files. It never executes or evaluates them.
package discover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kennyg/gh-tmpl/internal/form"
)

// Kind is the type of template.
type Kind string

// The template kinds.
const (
	PR    Kind = "pr"
	Issue Kind = "issue"
)

// ParseKind accepts "pr" or "issue" and common spellings of them.
func ParseKind(s string) (Kind, error) {
	switch strings.ToLower(s) {
	case "pr", "pull", "pull-request", "pull_request", "pullrequest":
		return PR, nil
	case "issue", "issues":
		return Issue, nil
	}
	return "", fmt.Errorf("unknown template kind %q: use pr or issue", s)
}

// Format is the file format of a template.
type Format string

// The template formats.
const (
	Markdown Format = "markdown"
	Form     Format = "form"
)

// Locations are the folders that GitHub searches, in order of precedence.
var Locations = []string{".github", "", "docs"}

// maxTemplates limits how many templates one folder can hold.
const maxTemplates = 100

// Template is one template file.
type Template struct {
	Kind   Kind   `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	About  string `json:"about,omitempty"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Format Format `json:"format"`
	// Default is true for the single-file template, for example
	// .github/pull_request_template.md. GitHub fills a new pull request
	// with it.
	Default bool `json:"default"`
	// Body is the Markdown body. For a Markdown template, the front matter
	// is removed. For a form, Body is the rendered skeleton.
	Body string `json:"body"`
	// Raw is the file content as read.
	Raw []byte `json:"-"`
	// Form is set when Format is Form.
	Form *form.Form `json:"form,omitempty"`
}

// Result is the outcome of discovery in one repository.
type Result struct {
	Kind      Kind
	Source    string
	Templates []Template
	// Shadowed lists template paths that a location with higher
	// precedence hides.
	Shadowed []string
	// Problems lists template files that could not be read or parsed.
	Problems []string
}

// Find searches each source in order and returns the first result that
// holds at least one template. A source is a function so that a caller can
// delay costly setup, such as an API client, until discovery needs it.
// A source function that returns a nil FS is skipped.
func Find(ctx context.Context, kind Kind, sources ...func() (FS, error)) (Result, error) {
	var last Result
	last.Kind = kind
	for _, open := range sources {
		fsys, err := open()
		if err != nil {
			return last, err
		}
		if fsys == nil {
			continue
		}
		res, err := FindIn(ctx, fsys, kind)
		if err != nil {
			return res, err
		}
		if len(res.Templates) > 0 {
			return res, nil
		}
		res.Problems = append(last.Problems, res.Problems...)
		last = res
	}
	return last, nil
}

// FindIn searches one repository.
func FindIn(ctx context.Context, fsys FS, kind Kind) (Result, error) {
	res := Result{Kind: kind, Source: fsys.Source()}
	single, folder := "pull_request_template", "pull_request_template"
	if kind == Issue {
		single, folder = "issue_template", "issue_template"
	}

	// The root listing shows which of the other locations exist, so that
	// discovery does not ask the API for folders that are not there.
	root, err := fsys.ReadDir(ctx, "")
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("reading %s in %s: %w", displayDir(""), fsys.Source(), err)
	}

	var singleFound, folderFound bool
	for _, loc := range Locations {
		entries := root
		if loc != "" {
			if !hasDir(root, loc) {
				continue
			}
			entries, err = fsys.ReadDir(ctx, loc)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return res, fmt.Errorf("reading %s in %s: %w", displayDir(loc), fsys.Source(), err)
			}
		}
		sortEntries(entries)
		for _, e := range entries {
			p := path.Join(loc, e.Name)
			switch {
			case !e.IsDir && isSingleTemplate(e.Name, single):
				if singleFound {
					res.Shadowed = append(res.Shadowed, p)
					continue
				}
				t, err := load(ctx, fsys, kind, p, true)
				if err != nil {
					res.Problems = append(res.Problems, err.Error())
					continue
				}
				singleFound = true
				res.Templates = append(res.Templates, t)
			case e.IsDir && strings.EqualFold(e.Name, folder):
				if folderFound {
					res.Shadowed = append(res.Shadowed, p+"/")
					continue
				}
				ts, err := loadFolder(ctx, fsys, kind, p, &res)
				if err != nil {
					return res, err
				}
				if len(ts) > 0 {
					folderFound = true
					res.Templates = append(res.Templates, ts...)
				}
			}
		}
	}
	return res, nil
}

func displayDir(loc string) string {
	if loc == "" {
		return "the repository root"
	}
	return loc + "/"
}

// sortEntries puts entries in a stable order, so that the result does not
// depend on the order of a folder listing.
func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
}

func isSingleTemplate(name, base string) bool {
	stem, ext := splitExt(name)
	if !strings.EqualFold(stem, base) {
		return false
	}
	switch strings.ToLower(ext) {
	case "", ".md", ".markdown", ".txt":
		return true
	}
	return false
}

func splitExt(name string) (string, string) {
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext), ext
}

func loadFolder(ctx context.Context, fsys FS, kind Kind, dir string, res *Result) ([]Template, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s/ in %s: %w", dir, fsys.Source(), err)
	}
	sortEntries(entries)
	var out []Template
	for _, e := range entries {
		if e.IsDir || len(out) >= maxTemplates {
			continue
		}
		stem, ext := splitExt(e.Name)
		switch strings.ToLower(ext) {
		case ".md", ".markdown", ".txt":
		case ".yml", ".yaml":
			// GitHub supports forms for issues only. config.yml configures
			// the template chooser and is not a template.
			if kind != Issue || strings.EqualFold(stem, "config") {
				continue
			}
		default:
			continue
		}
		t, err := load(ctx, fsys, kind, path.Join(dir, e.Name), false)
		if err != nil {
			res.Problems = append(res.Problems, err.Error())
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// Load reads one template file at path p. The file extension decides the
// format: .yml and .yaml are issue forms, anything else is Markdown.
func Load(ctx context.Context, fsys FS, kind Kind, p string) (Template, error) {
	return load(ctx, fsys, kind, p, false)
}

func load(ctx context.Context, fsys FS, kind Kind, p string, isDefault bool) (Template, error) {
	data, err := fsys.ReadFile(ctx, p)
	if err != nil {
		return Template{}, err
	}
	stem, ext := splitExt(path.Base(p))
	t := Template{
		Kind:    kind,
		ID:      stem,
		Path:    p,
		Source:  fsys.Source(),
		Default: isDefault,
		Raw:     data,
		Format:  Markdown,
	}
	switch strings.ToLower(ext) {
	case ".yml", ".yaml":
		f, err := form.Parse(data)
		if err != nil {
			return Template{}, fmt.Errorf("%s: %w", p, err)
		}
		t.Format = Form
		t.Form = f
		t.Name = f.Name
		t.About = f.Description
		t.Body = f.Render()
	default:
		fm, body := splitFrontMatter(normalizeNewlines(string(data)))
		t.Name = fm.Name
		t.About = fm.About
		t.Body = body
	}
	return t, nil
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

type frontMatter struct {
	Name  string `yaml:"name"`
	About string `yaml:"about"`
}

// splitFrontMatter removes a YAML front matter block from a Markdown
// template. GitHub uses it for the template chooser. A block that does not
// parse is kept as part of the body.
func splitFrontMatter(s string) (frontMatter, string) {
	var fm frontMatter
	if !strings.HasPrefix(s, "---\n") {
		return fm, s
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, s
	}
	after := rest[end+len("\n---"):]
	if after != "" && after[0] != '\n' {
		return fm, s
	}
	if err := decodeYAML([]byte(rest[:end]), &fm); err != nil {
		return frontMatter{}, s
	}
	return fm, strings.TrimLeft(after, "\n")
}

// decodeYAML decodes YAML into a typed value and turns a parser panic into
// an error.
func decodeYAML(data []byte, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid YAML: %v", r)
		}
	}()
	err = yaml.NewDecoder(bytes.NewReader(data)).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// Errors returned by Select.
var (
	ErrNone      = errors.New("no template found")
	ErrAmbiguous = errors.New("more than one template found")
	ErrNoMatch   = errors.New("no template matches the name")
)

// Select picks one template from a result.
//
// With a name, it matches the template ID (the file name without its
// extension), the full file name, or the name in the front matter or form,
// without regard to case. Without a name, it picks the only template. For a
// pull request with several templates, it picks the single-file default
// template, as GitHub does.
func Select(res Result, name string) (Template, error) {
	ts := res.Templates
	if name != "" {
		for _, t := range ts {
			if t.matches(name) {
				return t, nil
			}
		}
		return Template{}, ErrNoMatch
	}
	switch len(ts) {
	case 0:
		return Template{}, ErrNone
	case 1:
		return ts[0], nil
	}
	if res.Kind == PR {
		for _, t := range ts {
			if t.Default {
				return t, nil
			}
		}
	}
	return Template{}, ErrAmbiguous
}

func (t Template) matches(name string) bool {
	name = strings.TrimSpace(name)
	return strings.EqualFold(name, t.ID) ||
		strings.EqualFold(name, path.Base(t.Path)) ||
		strings.EqualFold(name, t.Path) ||
		(t.Name != "" && strings.EqualFold(name, strings.TrimSpace(t.Name)))
}

func hasDir(entries []Entry, name string) bool {
	for _, e := range entries {
		if e.IsDir && e.Name == name {
			return true
		}
	}
	return false
}
