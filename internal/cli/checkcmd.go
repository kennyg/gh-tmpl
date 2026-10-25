package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kennyg/gh-tmpl/internal/check"
	"github.com/kennyg/gh-tmpl/internal/discover"
)

const checkHelp = `Check an issue or pull request body against its template.

Usage:
  gh tmpl check [pr|issue] (--body-file <file> | --body <text>) [flags]

The kind defaults to pr. Use "--body-file -" to read the body from standard
input. The check reports missing headings, empty sections, template
comments left in the body, and checklists that nobody touched.

Exit codes:
  0  the body passes, or the repository has no template of this kind
  1  the body fails the check
  2  bad arguments, or several templates and no --name

Examples:
  gh tmpl check --body-file body.md
  gh tmpl check issue --name bug_report --body-file body.md --json
  git log -1 --format=%b | gh tmpl check --body-file -
`

// checkOpts holds the flags that change how strict the check is.
type checkOpts struct {
	name           string
	templateFile   string
	strict         bool
	allowUnchecked bool
}

func (o checkOpts) options() check.Options {
	return check.Options{Strict: o.strict, AllowUnchecked: o.allowUnchecked}
}

func runCheck(ctx context.Context, env *Env, args []string) int {
	fs := newFlagSet(env, "check", checkHelp)
	var l lookup
	var o checkOpts
	var bodyFile, bodyText string
	var asJSON bool
	fs.StringVar(&bodyFile, "body-file", "", "read the body from `file` (use - for standard input)")
	fs.StringVar(&bodyFile, "F", "", "short for --body-file")
	fs.StringVar(&bodyText, "body", "", "the body `text`")
	fs.StringVar(&bodyText, "b", "", "short for --body")
	fs.StringVar(&o.name, "name", "", "check against the template with this `ID`, file name or name")
	fs.StringVar(&o.templateFile, "template-file", "", "check against this template `file` instead of discovering one")
	fs.BoolVar(&o.strict, "strict", false, "fail on warnings too")
	fs.BoolVar(&o.allowUnchecked, "allow-unchecked", false, "report an untouched checklist as a warning, not an error")
	fs.BoolVar(&asJSON, "json", false, "print the report as JSON")
	fs.StringVar(&l.repo, "repo", "", "read the templates of `[HOST/]OWNER/REPO` through the API")
	fs.StringVar(&l.repo, "R", "", "short for --repo")
	fs.BoolVar(&l.offline, "offline", false, "do not read the owner's shared .github repository")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return flagExit(err)
	}
	kind, err := kindArg(pos)
	if err != nil {
		return usageFail(env, asJSON, err.Error())
	}

	var body string
	switch {
	case bodyFile != "" && bodyText != "":
		return usageFail(env, asJSON, "use --body or --body-file, not both")
	case bodyFile != "":
		data, err := readBody(env, bodyFile)
		if err != nil {
			return usageFail(env, asJSON, err.Error())
		}
		body = string(data)
	case isSet(fs, "body", "b"):
		body = bodyText
	default:
		return usageFail(env, asJSON, "pass the body with --body-file <file> or --body <text>")
	}

	var t discover.Template
	if o.templateFile != "" {
		t, err = loadTemplateFile(kind, o.templateFile)
		if err != nil {
			return usageFail(env, asJSON, err.Error())
		}
	} else {
		var res discover.Result
		t, res, err = env.pick(ctx, kind, l, o.name)
		if errors.Is(err, discover.ErrNone) {
			note := fmt.Sprintf("no %s template found in %s; nothing to check", kindLabel(kind), sourceLabel(res))
			if asJSON {
				_ = writeJSON(env.Stdout, map[string]any{"ok": true, "errors": 0, "warnings": 0, "template": nil, "findings": []check.Finding{}, "note": note})
			} else {
				fmt.Fprintf(env.Stdout, "PASS: %s.\n", note)
			}
			return ExitOK
		}
		if err != nil {
			return usageFail(env, asJSON, selectErrorOrPlain(env, res, err, "--name", o.name))
		}
	}

	rep := runChecks(t, body, o.options())
	if asJSON {
		_ = writeJSON(env.Stdout, jsonReport(t, rep))
	} else {
		printReport(env.Stdout, t, rep)
	}
	if !rep.OK {
		return ExitFailed
	}
	return ExitOK
}

func isSet(fs *flag.FlagSet, names ...string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		for _, n := range names {
			if f.Name == n {
				set = true
			}
		}
	})
	return set
}

// selectErrorOrPlain formats a discovery or selection error.
func selectErrorOrPlain(env *Env, res discover.Result, err error, nameFlag, name string) string {
	if errors.Is(err, discover.ErrAmbiguous) || errors.Is(err, discover.ErrNoMatch) || errors.Is(err, discover.ErrNone) {
		return selectError(env, res, err, nameFlag, name)
	}
	return safeText(err.Error())
}

func usageFail(env *Env, asJSON bool, msg string) int {
	if asJSON {
		_ = writeJSON(env.Stdout, map[string]any{"ok": false, "error": msg})
	}
	warnf(env, "%s", msg)
	return ExitUsage
}

// readBody reads a body file, or standard input for "-", up to
// check.MaxBodySize bytes.
func readBody(env *Env, name string) ([]byte, error) {
	var r io.Reader
	if name == "-" {
		r = env.Stdin
	} else {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: not a regular file", name)
		}
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, check.MaxBodySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > check.MaxBodySize {
		return nil, fmt.Errorf("the body is larger than %d bytes", check.MaxBodySize)
	}
	return data, nil
}

// loadTemplateFile reads a template from a path on disk.
func loadTemplateFile(kind discover.Kind, name string) (discover.Template, error) {
	dir, base := filepath.Split(name)
	if dir == "" {
		dir = "."
	}
	local, err := discover.OpenLocal(dir)
	if err != nil {
		return discover.Template{}, err
	}
	defer local.Close()
	return discover.Load(context.Background(), local, kind, base)
}

func runChecks(t discover.Template, body string, opts check.Options) check.Report {
	if t.Format == discover.Form && t.Form != nil {
		return check.Form(t.Form, body, opts)
	}
	return check.Markdown(t.Body, body, opts)
}

type reportJSON struct {
	check.Report
	Template templateInfo `json:"template"`
}

func jsonReport(t discover.Template, rep check.Report) reportJSON {
	return reportJSON{Report: rep, Template: info(t)}
}

func printReport(w io.Writer, t discover.Template, rep check.Report) {
	where := safeText(t.Path)
	if t.Source != "" && t.Source != "local" {
		where += " in " + safeText(t.Source)
	}
	counts := plural(rep.Errors, "error") + ", " + plural(rep.Warnings, "warning")
	if rep.OK {
		fmt.Fprintf(w, "PASS: the body follows %s (%s).\n", where, counts)
	} else {
		fmt.Fprintf(w, "FAIL: the body does not follow %s (%s).\n", where, counts)
	}
	for _, f := range rep.Findings {
		loc := ""
		if f.Line > 0 {
			loc = fmt.Sprintf(" line %d", f.Line)
		}
		fmt.Fprintf(w, "\n%s[%s]%s: %s\n  fix: %s\n", f.Severity, f.Code, loc, safeText(f.Message), safeText(f.Fix))
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// formHint is used by the create commands to point at the form skeleton.
func formHint(t discover.Template) string {
	if t.Format != discover.Form {
		return ""
	}
	return fmt.Sprintf("Run \"gh tmpl show issue --form %s\" for the sections the form needs.", t.ID)
}
