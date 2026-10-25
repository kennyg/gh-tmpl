package cli

import (
	"maps"
	"slices"
	"testing"

	"github.com/kennyg/gh-tmpl/internal/discover"
	"github.com/kennyg/gh-tmpl/internal/leantest"
	"github.com/kennyg/gh-tmpl/internal/spec"
)

type leanScan struct {
	Body     *string `json:"body"`
	BodyFile *string `json:"bodyFile"`
	Repo     *string `json:"repo"`
}

type leanArgvCase struct {
	Kind discover.Kind `json:"kind"`
	// A table line has the flag tables of the model.
	OwnFlags map[string]bool   `json:"ownFlags"`
	GhFlags  map[string]string `json:"ghFlags"`
	// A case line has the arguments and the model's results.
	Args []string  `json:"args"`
	Own  []string  `json:"own"`
	Gh   []string  `json:"gh"`
	Scan *leanScan `json:"scan"`
	// Parses tells whether the generated parser accepts the gh-tmpl words,
	// and TmplName is the --tmpl-name value it reads.
	Parses   bool    `json:"parses"`
	TmplName *string `json:"tmplName"`
}

// TestSplitCreateMatchesLean checks splitCreate, scanCreate and Run with the
// generated parser against the model in verify/GhTmpl/Argv.lean.
func TestSplitCreateMatchesLean(t *testing.T) {
	h := newHarness(t, "empty")
	tables, ran := 0, 0
	leantest.Each(t, "argv.jsonl", func(t *testing.T, line int, c leanArgvCase) {
		if c.OwnFlags != nil {
			tables++
			if !maps.Equal(c.OwnFlags, ownFlags(c.Kind)) {
				t.Errorf("%s: the model's gh-tmpl flags %v differ from the parser's %v", c.Kind, c.OwnFlags, ownFlags(c.Kind))
			}
			if !maps.Equal(c.GhFlags, ghFlags[c.Kind]) {
				t.Errorf("%s: the model's gh flags %v differ from ghFlags %v", c.Kind, c.GhFlags, ghFlags[c.Kind])
			}
			return
		}
		own, gh := splitCreate(c.Kind, c.Args)
		if !slices.Equal(own, c.Own) || !slices.Equal(gh, c.Gh) {
			t.Errorf("case %d: splitCreate(%s, %q) = %q, %q, want %q, %q", line, c.Kind, c.Args, own, gh, c.Own, c.Gh)
		}

		sc, err := scanCreate(c.Kind, c.Gh)
		switch {
		case c.Scan == nil:
			if err == nil {
				t.Errorf("case %d: scanCreate(%s, %q) returned no error", line, c.Kind, c.Gh)
			}
		case err != nil:
			t.Errorf("case %d: scanCreate(%s, %q): %v", line, c.Kind, c.Gh, err)
		case !eqPtr(sc.body, c.Scan.Body) || !eqPtr(nonEmpty(sc.bodyFile), nonEmpty(deref(c.Scan.BodyFile))) ||
			!eqPtr(nonEmpty(sc.repo), nonEmpty(deref(c.Scan.Repo))):
			t.Errorf("case %d: scanCreate(%s, %q) = body %q, file %q, repo %q, want %q, %q, %q", line, c.Kind, c.Gh,
				deref(sc.body), sc.bodyFile, sc.repo, deref(c.Scan.Body), deref(c.Scan.BodyFile), deref(c.Scan.Repo))
		}

		// The generated parser must read the gh-tmpl words as the model
		// does, in every case, also when Run stops before it calls gh.
		parsed, err := spec.Parse(splitCreateArgs(append([]string{string(c.Kind), "create"}, c.Args...)))
		switch {
		case !c.Parses:
			if err == nil {
				t.Errorf("case %d: the parser accepts %q, want an error", line, c.Args)
			}
		case err != nil:
			t.Errorf("case %d: the parser rejects %q: %v", line, c.Args, err)
		default:
			gotName := ""
			if parsed.Pr != nil {
				gotName = parsed.Pr.Create.TmplName
			} else {
				gotName = parsed.Issue.Create.TmplName
			}
			if gotName != deref(c.TmplName) {
				t.Errorf("case %d: the parser reads --tmpl-name %q from %q, want %q", line, gotName, c.Args, deref(c.TmplName))
			}
		}

		// Run, with the generated parser, must pass the gh words on
		// unchanged. The fixture has no templates, so Run calls gh unless
		// the parser rejects the arguments or they name a file or a remote
		// repository that the harness cannot read. The parser check above
		// covers the first reason.
		h.calls = nil
		h.run(append([]string{string(c.Kind), "create"}, c.Args...)...)
		if len(h.calls) == 0 {
			return
		}
		ran++
		want := append([]string{string(c.Kind), "create"}, c.Gh...)
		if got := h.calls[0].args; !slices.Equal(got, want) {
			t.Errorf("case %d: Run passes %q to gh, want %q", line, got, want)
		}
	})
	if tables != 2 {
		t.Errorf("found %d flag tables, want 2", tables)
	}
	if ran < 1000 {
		t.Errorf("Run called gh in only %d cases", ran)
	}
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// nonEmpty maps the empty string to nil. scanCreate uses "" for a body file
// or repository that is not given, so it cannot tell an empty --repo value from no
// --repo. Both lead to the same lookup.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
