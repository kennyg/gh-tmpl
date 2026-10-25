package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/kennyg/gh-tmpl/internal/discover"
	"github.com/kennyg/gh-tmpl/internal/spec"
)

func runShow(ctx context.Context, env *Env, c *spec.ShowCmd) int {
	l := lookup{repo: c.Repo, offline: c.Offline}
	name, formName, raw, asJSON, list := c.Name, c.Form, c.Raw, c.Json, c.List
	kind := kindOf(c.Kind)
	if formName != "" {
		if c.Kind == string(discover.PR) {
			warnf(env, "--form works with issue templates only")
			return ExitUsage
		}
		kind, name = discover.Issue, formName
	}

	res, err := env.find(ctx, kind, l)
	if err != nil {
		warnf(env, "%s", safeText(err.Error()))
		return ExitUsage
	}
	warnProblems(env, res)

	if list {
		if asJSON {
			_ = writeJSON(env.Stdout, map[string]any{"kind": kind, "source": res.Source, "templates": infos(res), "ignored": nonNil(res.Shadowed), "lookup_warnings": lookupWarnings(res)})
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
			_ = writeJSON(env.Stdout, map[string]any{"error": msg, "kind": kind, "templates": infos(res), "lookup_warnings": lookupWarnings(res)})
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
			Body           string          `json:"body"`
			Title          string          `json:"title,omitempty"`
			Form           any             `json:"form,omitempty"`
			LookupWarnings []lookupWarning `json:"lookup_warnings"`
		}{templateInfo: info(t), Body: body, LookupWarnings: lookupWarnings(res)}
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
