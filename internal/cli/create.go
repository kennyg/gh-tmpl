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

// createOpts holds the --tmpl-* flags, which the generated parser reads.
type createOpts struct {
	checkOpts
	offline bool
	skip    bool
}

// createArgs is what scanCreate finds in the gh arguments.
type createArgs struct {
	body     *string
	bodyFile string
	repo     string
}

// scanCreate reads the body, body file and repository flags from the
// arguments for gh. It does not change the arguments. The generated parser
// has already removed the --tmpl-* flags.
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
		switch {
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
				}
				record(long, value)
				break
			}
		}
	}
	return c, nil
}

func runCreate(ctx context.Context, env *Env, kind discover.Kind, args []string, o createOpts) int {
	sub := string(kind)
	c, err := scanCreate(kind, args)
	if err != nil {
		warnf(env, "%v", err)
		return ExitUsage
	}
	ghArgs := append([]string{sub, "create"}, args...)

	stdin := env.Stdin
	var body string
	switch {
	case o.skip:
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

	t, res, err := env.pick(ctx, kind, lookup{repo: c.repo, offline: o.offline}, o.name)
	switch {
	case errors.Is(err, discover.ErrNone):
		warnf(env, "no %s template found; running gh %s create without a check", kindLabel(kind), sub)
		return env.gh(ctx, ghArgs, stdin)
	case err != nil:
		warnf(env, "%s", selectErrorOrPlain(env, res, err, "--tmpl-name", o.name))
		return ExitUsage
	}

	rep := runChecks(t, body, o.options())
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
