package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kennyg/gh-tmpl/internal/discover"
	"github.com/kennyg/gh-tmpl/internal/spec"
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

// ownFlags maps each long flag of "pr create" or "issue create" in the
// generated parser to whether it takes a value. It includes --help.
func ownFlags(kind discover.Kind) map[string]bool {
	out := map[string]bool{"help": false}
	for _, c := range spec.Root.Subcommands {
		if c.Name != string(kind) {
			continue
		}
		for _, s := range c.Subcommands {
			if s.Name != "create" {
				continue
			}
			for _, f := range s.Flags {
				for _, l := range f.Longs {
					out[l] = f.TakesValue
				}
			}
		}
	}
	return out
}

// splitCreate sorts the words after "pr create" or "issue create" into the
// words for gh-tmpl, which are its own flags and their values, and the words
// for gh. Both keep the order of args, and no word is dropped, split or
// joined. A flag that takes a value, of gh-tmpl or of gh, takes the next
// word, so a value is never read as a flag. "--" and every word after it go
// to gh.
//
// verify/GhTmpl/Argv.lean proves these rules on a model of this function,
// and TestSplitCreateMatchesLean checks the function against the model.
func splitCreate(kind discover.Kind, args []string) (own, gh []string) {
	flags := ownFlags(kind)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return own, append(gh, args[i:]...)
		}
		takesValue, isOwn := ownFlag(flags, a)
		if !isOwn {
			takesValue = ghNeedsValue(kind, a)
		}
		n := 1
		if takesValue && i+1 < len(args) {
			n = 2
		}
		if isOwn {
			own = append(own, args[i:i+n]...)
		} else {
			gh = append(gh, args[i:i+n]...)
		}
		i += n - 1
	}
	return own, gh
}

// ownFlag reports whether a is a gh-tmpl flag, and whether it needs the next
// word as its value.
func ownFlag(flags map[string]bool, a string) (needsValue, ok bool) {
	if a == "-h" {
		return false, true
	}
	if !strings.HasPrefix(a, "--") {
		return false, false
	}
	name, _, hasValue := strings.Cut(a[2:], "=")
	takesValue, ok := flags[name]
	return ok && takesValue && !hasValue, ok
}

// ghNeedsValue reports whether a is a gh flag that takes the next word as
// its value: a long flag without "=", or a cluster of short flags that ends
// with one that takes a value.
func ghNeedsValue(kind discover.Kind, a string) bool {
	valued := ghFlags[kind]
	if strings.HasPrefix(a, "--") {
		_, ok := valued[a[2:]]
		return ok && !strings.Contains(a, "=")
	}
	if !strings.HasPrefix(a, "-") {
		return false
	}
	for j := 1; j < len(a); j++ {
		if isShortValued(valued, a[j]) {
			return j == len(a)-1
		}
	}
	return false
}

func isShortValued(valued map[string]string, b byte) bool {
	for _, short := range valued {
		if short != "" && short[0] == b {
			return true
		}
	}
	return false
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
// arguments for gh. It does not change the arguments. splitCreate has
// already removed the --tmpl-* flags.
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
		case a == "--":
			// gh reads the words after "--" as arguments, not flags.
			return c, nil
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
