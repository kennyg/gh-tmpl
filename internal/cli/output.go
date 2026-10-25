package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/kennyg/gh-tmpl/internal/discover"
)

// safeText removes control characters, except tab and newline, from
// untrusted text before it goes to a terminal. A template or body cannot
// then send escape sequences to the terminal.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return -1
		}
		// Bidirectional overrides can make text read differently from
		// what it is.
		if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return -1
		}
		return r
	}, s)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// templateInfo is a template without its content, for lists and reports.
type templateInfo struct {
	Kind    discover.Kind   `json:"kind"`
	ID      string          `json:"id"`
	Name    string          `json:"name,omitempty"`
	About   string          `json:"about,omitempty"`
	Path    string          `json:"path"`
	Source  string          `json:"source"`
	Format  discover.Format `json:"format"`
	Default bool            `json:"default"`
}

func info(t discover.Template) templateInfo {
	return templateInfo{Kind: t.Kind, ID: t.ID, Name: t.Name, About: t.About, Path: t.Path, Source: t.Source, Format: t.Format, Default: t.Default}
}

func infos(res discover.Result) []templateInfo {
	out := make([]templateInfo, 0, len(res.Templates))
	for _, t := range res.Templates {
		out = append(out, info(t))
	}
	return out
}

// printList writes a table of the templates in res.
func printList(w io.Writer, res discover.Result) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tPATH")
	for _, t := range res.Templates {
		p := t.Path
		var notes []string
		if t.Format == discover.Form {
			notes = append(notes, "form")
		}
		if t.Default {
			notes = append(notes, "default")
		}
		if len(notes) > 0 {
			p += " (" + strings.Join(notes, ", ") + ")"
		}
		name := t.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", safeText(t.ID), safeText(name), safeText(p))
	}
	_ = tw.Flush()
	if res.Source != "local" {
		fmt.Fprintf(w, "\nSource: %s\n", safeText(res.Source))
	}
	if len(res.Shadowed) > 0 {
		fmt.Fprintf(w, "\nIgnored because a location with higher precedence has templates: %s\n", safeText(strings.Join(res.Shadowed, ", ")))
	}
}

func kindLabel(k discover.Kind) string {
	if k == discover.PR {
		return "pull request"
	}
	return "issue"
}

// selectError explains why discover.Select found no single template.
// The name flag is the flag that picks a template in this command.
func selectError(env *Env, res discover.Result, err error, nameFlag, name string) string {
	switch {
	case errors.Is(err, discover.ErrNone):
		return fmt.Sprintf("no %s template found in %s", kindLabel(res.Kind), sourceLabel(res))
	case errors.Is(err, discover.ErrAmbiguous):
		fmt.Fprintf(env.Stderr, "Several %s templates found in %s. Pass %s with one of these IDs:\n\n", kindLabel(res.Kind), sourceName(res.Source), nameFlag)
		printList(env.Stderr, res)
		return fmt.Sprintf("more than one %s template: pass %s", kindLabel(res.Kind), nameFlag)
	case errors.Is(err, discover.ErrNoMatch):
		fmt.Fprintf(env.Stderr, "No %s template matches %q. The templates in %s are:\n\n", kindLabel(res.Kind), safeText(name), sourceName(res.Source))
		printList(env.Stderr, res)
		return fmt.Sprintf("no %s template matches %q", kindLabel(res.Kind), safeText(name))
	}
	return safeText(err.Error())
}

// sourceName describes where a template came from.
func sourceName(source string) string {
	if source == "local" {
		return "the local clone"
	}
	return safeText(source)
}

func sourceLabel(res discover.Result) string {
	if res.Source == "" || res.Source == "local" {
		return "the repository or its owner's .github repository"
	}
	return safeText(res.Source)
}

// kindOf converts the kind argument, which the parser has already checked
// against the choices pr and issue.
func kindOf(s string) discover.Kind {
	if s == string(discover.Issue) {
		return discover.Issue
	}
	return discover.PR
}
