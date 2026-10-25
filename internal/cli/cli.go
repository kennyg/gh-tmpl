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

	"github.com/kennyg/gh-tmpl/internal/discover"
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

const usage = `Write and check issue and pull request bodies against a repository's templates.

Usage:
  gh tmpl show [pr|issue] [--name <template>] [--form <name>] [--list] [--json]
  gh tmpl check [pr|issue] (--body-file <file> | --body <text>) [--name <template>] [--json]
  gh tmpl pr create [gh pr create flags] [--tmpl-name <template>]
  gh tmpl issue create [gh issue create flags] [--tmpl-name <template>]

Commands:
  show     Print a template, or list the templates when there are several.
  check    Check a body against a template. Exits 1 if the body fails.
  pr       Check the body, then run "gh pr create" with the other flags.
  issue    Check the body, then run "gh issue create" with the other flags.

Run "gh tmpl <command> --help" for the flags of a command.
Docs: https://github.com/kennyg/gh-tmpl#readme
`

// Run runs one command and returns the exit code.
func Run(ctx context.Context, env *Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	case "-v", "--version", "version":
		fmt.Fprintf(env.Stdout, "gh-tmpl %s\n", env.Version)
		return ExitOK
	case "show":
		return runShow(ctx, env, args[1:])
	case "check":
		return runCheck(ctx, env, args[1:])
	case "pr", "issue":
		if len(args) < 2 || args[1] != "create" {
			fmt.Fprintf(env.Stderr, "gh-tmpl: only \"gh tmpl %s create\" is supported\n", args[0])
			return ExitUsage
		}
		kind := discover.PR
		if args[0] == "issue" {
			kind = discover.Issue
		}
		return runCreate(ctx, env, kind, args[2:])
	}
	fmt.Fprintf(env.Stderr, "gh-tmpl: unknown command %q\n\n%s", args[0], usage)
	return ExitUsage
}

func warnf(env *Env, format string, a ...any) {
	fmt.Fprintf(env.Stderr, "gh-tmpl: "+format+"\n", a...)
}
