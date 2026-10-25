package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/kennyg/gh-tmpl/internal/discover"
)

const showHelp = `Print a pull request or issue template.

Usage:
  gh tmpl show [pr|issue] [flags]

The kind defaults to pr. With several templates and no --name, the command
lists them and exits 2. A pull request uses the single-file template, such
as .github/pull_request_template.md, when there is one.

An issue form (.yml) prints as Markdown sections that match the body GitHub
writes for a submitted form. Use --raw to print the YAML.

Examples:
  gh tmpl show pr > body.md
  gh tmpl show issue --list
  gh tmpl show issue --form bug_report > body.md
  gh tmpl show issue --name feature_request --json
`

func runShow(ctx context.Context, env *Env, args []string) int {
	fs := newFlagSet(env, "show", showHelp)
	var l lookup
	var name, formName string
	var raw, asJSON, list bool
	fs.StringVar(&name, "name", "", "pick the template with this `ID`, file name or name")
	fs.StringVar(&formName, "form", "", "pick the issue form with this `ID` and print it as Markdown")
	fs.BoolVar(&raw, "raw", false, "print the file as it is, front matter or YAML included")
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	fs.BoolVar(&list, "list", false, "list the templates")
	fs.StringVar(&l.repo, "repo", "", "read the templates of `[HOST/]OWNER/REPO` through the API")
	fs.StringVar(&l.repo, "R", "", "short for --repo")
	fs.BoolVar(&l.offline, "offline", false, "do not read the owner's shared .github repository")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return flagExit(err)
	}
	kind, err := kindArg(pos)
	if err != nil {
		warnf(env, "%v", err)
		return ExitUsage
	}
	if formName != "" {
		if len(pos) == 1 && kind != discover.Issue {
			warnf(env, "--form works with issue templates only")
			return ExitUsage
		}
		if name != "" {
			warnf(env, "use --name or --form, not both")
			return ExitUsage
		}
		kind, name = discover.Issue, formName
	}

	res, err := env.find(ctx, kind, l)
	if err != nil {
		warnf(env, "%s", safeText(err.Error()))
		return ExitUsage
	}
	for _, p := range res.Problems {
		warnf(env, "skipped a template: %s", safeText(p))
	}

	if list {
		if asJSON {
			_ = writeJSON(env.Stdout, map[string]any{"kind": kind, "source": res.Source, "templates": infos(res), "ignored": nonNil(res.Shadowed)})
			return ExitOK
		}
		if len(res.Templates) == 0 {
			warnf(env, "no %s template found in %s", kindLabel(kind), sourceLabel(res))
			return ExitUsage
		}
		printList(env.Stdout, res)
		return ExitOK
	}

	t, err := discover.Select(res, name)
	if err != nil {
		msg := selectError(env, res, err, "--name", name)
		if asJSON {
			_ = writeJSON(env.Stdout, map[string]any{"error": msg, "kind": kind, "templates": infos(res)})
		}
		warnf(env, "%s", msg)
		return ExitUsage
	}
	if formName != "" && t.Format != discover.Form {
		warnf(env, "%s is a Markdown template, not an issue form: use --name", safeText(t.Path))
		return ExitUsage
	}

	body := t.Body
	if raw {
		body = string(t.Raw)
	}
	if asJSON {
		out := struct {
			templateInfo
			Body  string `json:"body"`
			Title string `json:"title,omitempty"`
			Form  any    `json:"form,omitempty"`
		}{templateInfo: info(t), Body: body}
		if t.Form != nil {
			out.Title = t.Form.Title
			out.Form = t.Form
		}
		if err := writeJSON(env.Stdout, out); err != nil {
			warnf(env, "%v", err)
			return ExitFailed
		}
		return ExitOK
	}
	body = safeText(body)
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	fmt.Fprint(env.Stdout, body)
	return ExitOK
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
