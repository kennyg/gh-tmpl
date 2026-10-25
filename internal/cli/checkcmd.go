package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kennyg/gh-tmpl/internal/check"
	"github.com/kennyg/gh-tmpl/internal/discover"
	"github.com/kennyg/gh-tmpl/internal/spec"
)

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

func runCheck(ctx context.Context, env *Env, c *spec.CheckCmd) int {
	l := lookup{repo: c.Repo, offline: c.Offline}
	o := checkOpts{name: c.Name, templateFile: c.TemplateFile, strict: c.Strict, allowUnchecked: c.AllowUnchecked}
	asJSON := c.Json
	kind := kindOf(c.Kind)

	// The parser rejects --body together with --body-file.
	var body string
	switch {
	case c.BodyFile != "":
		data, err := readBody(env, c.BodyFile)
		if err != nil {
			return usageFail(env, asJSON, err.Error())
		}
		body = string(data)
	case c.Body != "":
		body = c.Body
	default:
		return usageFail(env, asJSON, "pass a non-empty body with --body-file <file> or --body <text>")
	}

	var t discover.Template
	var res discover.Result
	var err error
	if o.templateFile != "" {
		t, err = loadTemplateFile(kind, o.templateFile)
		if err != nil {
			return usageFail(env, asJSON, err.Error())
		}
	} else {
		t, res, err = env.pick(ctx, kind, l, o.name)
		if errors.Is(err, discover.ErrNone) {
			note := fmt.Sprintf("no %s template found in %s; nothing to check", kindLabel(kind), sourceLabel(res))
			if asJSON {
				_ = writeJSON(env.Stdout, map[string]any{"ok": true, "errors": 0, "warnings": 0, "template": nil, "findings": []check.Finding{}, "note": note, "lookup_warnings": lookupWarnings(res)})
			} else {
				fmt.Fprintf(env.Stdout, "PASS: %s.\n", note)
			}
			return ExitOK
		}
		if err != nil {
			msg := selectErrorOrPlain(env, res, err, "--name", o.name)
			if asJSON && (errors.Is(err, discover.ErrAmbiguous) || errors.Is(err, discover.ErrNoMatch)) {
				// List the choices, so that a program that reads only the
				// JSON knows which names it can pass.
				_ = writeJSON(env.Stdout, map[string]any{"ok": false, "error": msg, "kind": kind, "templates": infos(res), "lookup_warnings": lookupWarnings(res)})
				warnf(env, "%s", msg)
				return ExitUsage
			}
			return usageFail(env, asJSON, msg)
		}
	}

	rep := runChecks(t, body, o.options())
	if asJSON {
		_ = writeJSON(env.Stdout, jsonReport(t, rep, res))
	} else {
		printReport(env.Stdout, t, rep)
	}
	if !rep.OK {
		return ExitFailed
	}
	return ExitOK
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
	Template       templateInfo    `json:"template"`
	LookupWarnings []lookupWarning `json:"lookup_warnings"`
}

func jsonReport(t discover.Template, rep check.Report, res discover.Result) reportJSON {
	return reportJSON{Report: rep, Template: info(t), LookupWarnings: lookupWarnings(res)}
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
