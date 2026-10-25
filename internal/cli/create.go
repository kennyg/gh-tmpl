package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kennyg/gh-tmpl/internal/discover"
)

const createHelp = `Check the body against the template, then run "gh %[1]s create".

Usage:
  gh tmpl %[1]s create [gh %[1]s create flags] [gh-tmpl flags]

gh-tmpl reads the body from --body or --body-file, checks it, and stops with
exit code 1 if the check fails. If the body passes, it runs
"gh %[1]s create" with every flag except the gh-tmpl flags below. Without
--body or --body-file there is no body to check, so it runs gh directly.

gh-tmpl flags:
  --tmpl-name <template>   check against the template with this ID or name
  --tmpl-strict            fail on warnings too
  --tmpl-allow-unchecked   report an untouched checklist as a warning
  --tmpl-offline           do not read the owner's shared .github repository
  --tmpl-skip              run gh without the check

Examples:
  gh tmpl pr create --title "Fix login" --body-file body.md
  gh tmpl issue create --tmpl-name bug_report --title "Crash" --body-file body.md
`

// ghFlags lists the flags of "gh pr create" and "gh issue create" that take
// a value, so the scanner does not read a value as a flag. The map value is
// the short form, or "" if there is none.
var ghFlags = map[discover.Kind]map[string]string{
	discover.PR: {
		"assignee": "a", "attach": "", "base": "B", "body": "b", "body-file": "F", "head": "H",
		"label": "l", "milestone": "m", "project": "p", "recover": "", "reviewer": "r",
		"template": "T", "title": "t", "repo": "R",
	},
	discover.Issue: {
		"assignee": "a", "attach": "", "blocked-by": "", "blocking": "", "body": "b", "body-file": "F",
		"label": "l", "milestone": "m", "parent": "", "project": "p", "recover": "", "template": "T",
		"title": "t", "repo": "R", "type": "",
	},
}

// createArgs is the result of scanning the create arguments.
type createArgs struct {
	pass     []string // arguments for gh
	body     *string
	bodyFile string
	repo     string
	opts     checkOpts
	offline  bool
	skip     bool
	help     bool
}

// scanCreate splits the arguments into gh-tmpl flags and gh flags. It reads
// the body, body file and repository flags but leaves them in place for gh.
func scanCreate(kind discover.Kind, args []string) (createArgs, error) {
	var c createArgs
	valued := ghFlags[kind]
	shortValued := map[byte]string{}
	for long, short := range valued {
		if short != "" {
			shortValued[short[0]] = long
		}
	}
	record := func(long, value string) {
		switch long {
		case "body":
			v := value
			c.body = &v
		case "body-file":
			c.bodyFile = value
		case "repo":
			c.repo = value
		}
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			c.pass = append(c.pass, args[i:]...)
			break
		}
		if strings.HasPrefix(a, "--tmpl-") {
			name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--tmpl-"), "=")
			switch name {
			case "name":
				if !hasValue {
					if i+1 >= len(args) {
						return c, errors.New("--tmpl-name needs a value")
					}
					i++
					value = args[i]
				}
				c.opts.name = value
			case "strict":
				c.opts.strict = true
			case "allow-unchecked":
				c.opts.allowUnchecked = true
			case "offline":
				c.offline = true
			case "skip":
				c.skip = true
			default:
				return c, fmt.Errorf("unknown flag %s", a)
			}
			continue
		}
		c.pass = append(c.pass, a)
		switch {
		case a == "-h" || a == "--help":
			c.help = true
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a[2:], "=")
			if _, ok := valued[name]; !ok {
				continue
			}
			if !hasValue {
				if i+1 >= len(args) {
					return c, fmt.Errorf("--%s needs a value", name)
				}
				i++
				value = args[i]
				c.pass = append(c.pass, value)
			}
			record(name, value)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// A cluster of short flags, such as -dw or -Fbody.md. The first
			// flag that takes a value takes the rest of the cluster, or the
			// next argument.
			for j := 1; j < len(a); j++ {
				long, ok := shortValued[a[j]]
				if !ok {
					continue
				}
				value := strings.TrimPrefix(a[j+1:], "=")
				if j+1 >= len(a) {
					if i+1 >= len(args) {
						return c, fmt.Errorf("-%c needs a value", a[j])
					}
					i++
					value = args[i]
					c.pass = append(c.pass, value)
				}
				record(long, value)
				break
			}
		}
	}
	return c, nil
}

func runCreate(ctx context.Context, env *Env, kind discover.Kind, args []string) int {
	sub := "pr"
	if kind == discover.Issue {
		sub = "issue"
	}
	c, err := scanCreate(kind, args)
	if err != nil {
		warnf(env, "%v", err)
		return ExitUsage
	}
	ghArgs := append([]string{sub, "create"}, c.pass...)
	if c.help {
		fmt.Fprintf(env.Stdout, createHelp+"\nThe gh %s create help follows.\n\n", sub, sub)
		return env.gh(ctx, ghArgs, env.Stdin)
	}

	stdin := env.Stdin
	var body string
	switch {
	case c.skip:
		return env.gh(ctx, ghArgs, stdin)
	case c.bodyFile != "":
		data, err := readBody(env, c.bodyFile)
		if err != nil {
			warnf(env, "%v", err)
			return ExitUsage
		}
		body = string(data)
		if c.bodyFile == "-" {
			// gh reads the body from standard input too, which is used up.
			stdin = bytes.NewReader(data)
		}
	case c.body != nil:
		body = *c.body
	default:
		warnf(env, "no --body or --body-file, so there is no body to check; running gh %s create", sub)
		return env.gh(ctx, ghArgs, stdin)
	}

	t, res, err := env.pick(ctx, kind, lookup{repo: c.repo, offline: c.offline}, c.opts.name)
	switch {
	case errors.Is(err, discover.ErrNone):
		warnf(env, "no %s template found; running gh %s create without a check", kindLabel(kind), sub)
		return env.gh(ctx, ghArgs, stdin)
	case err != nil:
		warnf(env, "%s", selectErrorOrPlain(env, res, err, "--tmpl-name", c.opts.name))
		return ExitUsage
	}

	rep := runChecks(t, body, c.opts.options())
	if !rep.OK {
		// gh prints the new URL on standard output, so the report goes to
		// standard error.
		printReport(env.Stderr, t, rep)
		if hint := formHint(t); hint != "" {
			fmt.Fprintf(env.Stderr, "\n%s\n", hint)
		}
		fmt.Fprintf(env.Stderr, "\ngh-tmpl: fix the body and run the command again. Nothing was created.\n")
		return ExitFailed
	}
	if rep.Warnings > 0 {
		printReport(env.Stderr, t, rep)
	}
	return env.gh(ctx, ghArgs, stdin)
}

func (env *Env) gh(ctx context.Context, args []string, stdin io.Reader) int {
	code, err := env.RunGh(ctx, args, stdin)
	if err != nil {
		warnf(env, "%v", err)
	}
	return code
}
