package discover

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/kennyg/gh-tmpl/internal/leantest"
)

type leanSource struct {
	Type  string   `json:"type"`
	Files []string `json:"files"`
}

type leanTemplate struct {
	Path    string `json:"path"`
	Default bool   `json:"default"`
}

type leanResult struct {
	Error     string         `json:"error"`
	Source    *int           `json:"source"`
	Templates []leanTemplate `json:"templates"`
	Shadowed  []string       `json:"shadowed"`
	Problems  int            `json:"problems"`
}

type leanFindCase struct {
	Kind    Kind         `json:"kind"`
	Sources []leanSource `json:"sources"`
	Result  leanResult   `json:"result"`
}

var (
	errLeanOpen = errors.New("cannot open the source")
	errLeanRead = errors.New("cannot list the root")
)

// memFS is a repository in memory. Markdown files hold a line of text, and
// every issue form is invalid YAML, as in the model.
type memFS struct {
	source string
	files  []string
	// rootErr is returned when the root is listed.
	rootErr error
}

func (m *memFS) Source() string { return m.source }

func (m *memFS) ReadDir(_ context.Context, dir string) ([]Entry, error) {
	if dir == "" && m.rootErr != nil {
		return nil, m.rootErr
	}
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	seen := map[string]bool{}
	var out []Entry
	found := dir == ""
	for _, f := range m.files {
		rest, ok := strings.CutPrefix(f, prefix)
		if !ok {
			continue
		}
		found = true
		name, _, isDir := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, Entry{Name: name, IsDir: isDir})
	}
	if !found {
		return nil, fs.ErrNotExist
	}
	return out, nil
}

func (m *memFS) ReadFile(_ context.Context, name string) ([]byte, error) {
	if !slices.Contains(m.files, name) {
		return nil, fs.ErrNotExist
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".yml", ".yaml":
		return []byte("name: [unclosed\n"), nil
	}
	return []byte("## Summary\n"), nil
}

// TestFindMatchesLean checks Find against the model in
// verify/GhTmpl/Lookup.lean.
func TestFindMatchesLean(t *testing.T) {
	leantest.Each(t, "find.jsonl", func(t *testing.T, line int, c leanFindCase) {
		var sources []func() (FS, error)
		for i, s := range c.Sources {
			name := fmt.Sprintf("src%d", i)
			switch s.Type {
			case "openErr":
				sources = append(sources, func() (FS, error) { return nil, errLeanOpen })
			case "skip":
				sources = append(sources, func() (FS, error) { return nil, nil })
			case "notFound":
				sources = append(sources, func() (FS, error) { return &memFS{source: name, rootErr: fs.ErrNotExist}, nil })
			case "readErr":
				sources = append(sources, func() (FS, error) { return &memFS{source: name, rootErr: errLeanRead}, nil })
			case "repo":
				sources = append(sources, func() (FS, error) { return &memFS{source: name, files: s.Files}, nil })
			default:
				t.Fatalf("case %d: unknown source type %q", line, s.Type)
			}
		}
		res, err := Find(context.Background(), c.Kind, sources...)
		want := c.Result
		if want.Error != "" {
			wantErr := map[string]error{"openErr": errLeanOpen, "notFound": ErrRepoNotFound, "readErr": errLeanRead}[want.Error]
			if !errors.Is(err, wantErr) {
				t.Errorf("case %d: %v: error %v, want %v", line, c.Sources, err, wantErr)
			}
			return
		}
		if err != nil {
			t.Errorf("case %d: %v: %v", line, c.Sources, err)
			return
		}
		got := leanResult{Shadowed: res.Shadowed, Problems: len(res.Problems)}
		if res.Source != "" {
			var i int
			if _, err := fmt.Sscanf(res.Source, "src%d", &i); err != nil {
				t.Fatalf("case %d: source %q", line, res.Source)
			}
			got.Source = &i
		}
		for _, tp := range res.Templates {
			got.Templates = append(got.Templates, leanTemplate{Path: tp.Path, Default: tp.Default})
		}
		if !equalResult(got, want) {
			t.Errorf("case %d: %s %+v\ngot  %s\nwant %s", line, c.Kind, c.Sources, showResult(got), showResult(want))
		}
	})
}

func equalResult(a, b leanResult) bool {
	return (a.Source == nil) == (b.Source == nil) && (a.Source == nil || *a.Source == *b.Source) &&
		slices.Equal(a.Templates, b.Templates) && slices.Equal(a.Shadowed, b.Shadowed) && a.Problems == b.Problems
}

func showResult(r leanResult) string {
	src := "none"
	if r.Source != nil {
		src = fmt.Sprint(*r.Source)
	}
	return fmt.Sprintf("source %s, templates %v, shadowed %q, problems %d", src, r.Templates, r.Shadowed, r.Problems)
}
