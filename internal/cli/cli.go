// Package cli implements the gh tmpl commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/jdx/usage/go/argv"

	"github.com/kennyg/gh-tmpl/internal/discover"
	"github.com/kennyg/gh-tmpl/internal/spec"
)

// Exit codes.
const (
	ExitOK     = 0
	ExitFailed = 1 // the body does not follow the template
	ExitUsage  = 2 // bad arguments, or no single template to use
)

// Env holds the dependencies of the commands, so that tests can replace
// them.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Getwd returns the working folder.
	Getwd func() (string, error)
	// CurrentRepo returns the repository of the working folder, from its
	// git remotes or GH_REPO.
	CurrentRepo func() (repository.Repository, error)
	// NewClient returns a REST client for host that uses the stored gh
	// credentials.
	NewClient func(host string) (discover.Requester, error)
	// RunGh runs gh with args and returns its exit code. It passes args as
	// a list to the gh binary and never uses a shell.
	RunGh func(ctx context.Context, args []string, stdin io.Reader) (int, error)

	Version string
}

// DefaultEnv returns the Env for a real run.
func DefaultEnv(version string) *Env {
	return &Env{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getwd:       os.Getwd,
		CurrentRepo: repository.Current,
		NewClient:   newClient,
		RunGh:       runGh,
		Version:     version,
	}
}

func newClient(host string) (discover.Requester, error) {
	// go-gh reads the token that "gh auth login" stored for host. The
	// extension never reads, prints or logs the token itself. LogIgnoreEnv
	// keeps GH_DEBUG from dumping HTTP traffic.
	return api.NewRESTClient(api.ClientOptions{
		Host:         host,
		Timeout:      30 * time.Second,
		LogIgnoreEnv: true,
	})
}

func runGh(ctx context.Context, args []string, stdin io.Reader) (int, error) {
	path, err := gh.Path()
	if err != nil {
		return ExitUsage, fmt.Errorf("cannot find gh: %w", err)
	}
	// The arguments go to gh as a list. No shell parses them.
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // G204: running gh is the purpose of the create commands
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return ExitFailed, err
	}
	return ExitOK, nil
}

//go:generate usage generate go -f ../../gh-tmpl.usage.kdl -o ../spec/spec.go -p spec

// Run runs one command and returns the exit code. The parser, the help
// pages and the completion answers all come from the tables that "usage
// generate go" builds from gh-tmpl.usage.kdl.
func Run(ctx context.Context, env *Env, args []string) int {
	// A completion script calls the binary back with a hidden command.
	if out, ok := argv.Respond(args, spec.Root, spec.HelpText, spec.Meta); ok {
		fmt.Fprint(env.Stdout, out)
		return ExitOK
	}
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, argv.ShortHelp(helpMeta(env), []string{displayName}, []*argv.Command{spec.Root}, spec.HelpText))
		return ExitUsage
	}
	args = splitCreateArgs(args)
	cli, err := spec.Parse(args)
	if err != nil {
		var perr *argv.Error
		if !errors.As(err, &perr) {
			warnf(env, "%v", err)
			return ExitUsage
		}
		if perr.Code == argv.CodeVersion {
			fmt.Fprintf(env.Stdout, "%s %s\n", spec.HelpMeta.Bin, env.Version)
			return ExitOK
		}
		if text, ok := argv.RenderRequest(perr, helpMeta(env), spec.Root, args, spec.HelpText); ok {
			fmt.Fprint(env.Stdout, text)
			return ExitOK
		}
		pos := argv.Walk(spec.Root, args)
		path := []string{displayName}
		for _, c := range pos.Chain[1:] {
			path = append(path, c.Name)
		}
		fmt.Fprint(env.Stderr, argv.Render(perr, path, pos.Chain, spec.HelpText))
		return ExitUsage
	}

	switch {
	case cli.Show != nil:
		return runShow(ctx, env, cli.Show)
	case cli.Check != nil:
		return runCheck(ctx, env, cli.Check)
	case cli.Pr != nil:
		c := cli.Pr.Create
		if c == nil {
			return missingSubcommand(env, "pr")
		}
		return runCreate(ctx, env, discover.PR, c.GhArgs, createOpts{
			checkOpts: checkOpts{name: c.TmplName, strict: c.TmplStrict, allowUnchecked: c.TmplAllowUnchecked},
			offline:   c.TmplOffline,
			skip:      c.TmplSkip,
		})
	case cli.Issue != nil:
		c := cli.Issue.Create
		if c == nil {
			return missingSubcommand(env, "issue")
		}
		return runCreate(ctx, env, discover.Issue, c.GhArgs, createOpts{
			checkOpts: checkOpts{name: c.TmplName, strict: c.TmplStrict, allowUnchecked: c.TmplAllowUnchecked},
			offline:   c.TmplOffline,
			skip:      c.TmplSkip,
		})
	case cli.Completion != nil:
		shell, _ := argv.ShellNamed(cli.Completion.Shell)
		fmt.Fprint(env.Stdout, argv.Script(spec.HelpMeta.Bin, shell))
		return ExitOK
	}
	return ExitUsage
}

// splitCreateArgs prepares the arguments of "pr create" and "issue create"
// for the generated parser. The parser would read a gh-tmpl flag anywhere,
// even as the value of a gh flag, as in "--title --tmpl-skip", and it drops
// a "--". So splitCreate sorts the words first, and the gh words go after a
// "--", where the parser keeps every word as it is. The parser rejects a
// separate value that starts with "-", so each gh-tmpl value is attached to
// its flag, as in "--tmpl-name=-x".
func splitCreateArgs(args []string) []string {
	if len(args) < 2 || args[1] != "create" {
		return args
	}
	kind := discover.Kind(args[0])
	if kind != discover.PR && kind != discover.Issue {
		return args
	}
	own, gh := splitCreate(kind, args[2:])
	flags := ownFlags(kind)
	out := []string{args[0], args[1]}
	for i := 0; i < len(own); i++ {
		if needsValue, _ := ownFlag(flags, own[i]); needsValue && i+1 < len(own) {
			out = append(out, own[i]+"="+own[i+1])
			i++
			continue
		}
		out = append(out, own[i])
	}
	out = append(out, "--")
	return append(out, gh...)
}

// missingSubcommand prints the help of a command that needs a subcommand.
// The generated parser does not enforce subcommand_required, so the
// dispatcher does.
func missingSubcommand(env *Env, name string) int {
	pos := argv.Walk(spec.Root, []string{name})
	fmt.Fprintf(env.Stderr, "error: %q needs a subcommand\n\n", name)
	fmt.Fprint(env.Stderr, argv.ShortHelp(helpMeta(env), []string{displayName, name}, pos.Chain, spec.HelpText))
	return ExitUsage
}

// displayName is how people run the extension. Help pages and errors use
// it. Completion scripts use the binary name, gh-tmpl, because gh does not
// complete the commands of extensions.
const displayName = "gh tmpl"

// helpMeta returns the help page header with the version of this build.
func helpMeta(env *Env) argv.HelpSpec {
	meta := spec.HelpMeta
	meta.Bin = displayName
	meta.Version = env.Version
	return meta
}

func warnf(env *Env, format string, a ...any) {
	fmt.Fprintf(env.Stderr, "gh-tmpl: "+format+"\n", a...)
}
