package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/kennyg/gh-tmpl/internal/discover"
)

// lookup holds the flags that choose where templates come from.
type lookup struct {
	repo    string
	offline bool
}

// errNotInRepo tells the user how to name a repository.
var errNotInRepo = errors.New("not inside a git repository: run the command in a clone, or pass --repo OWNER/REPO")

// sharedRepo is the repository that holds an owner's default community
// health files, including templates.
const sharedRepo = ".github"

// find runs discovery for kind. It reads the local working tree, or the
// --repo repository through the API. If that holds no template of the kind,
// it reads the owner's shared .github repository.
func (env *Env) find(ctx context.Context, kind discover.Kind, l lookup) (discover.Result, error) {
	var sources []func() (discover.FS, error)
	var closers []func() error
	defer func() {
		for _, c := range closers {
			_ = c()
		}
	}()

	if l.repo != "" {
		r, err := repository.Parse(l.repo)
		if err != nil {
			return discover.Result{}, err
		}
		sources = append(sources, env.remote(r.Host, r.Owner, r.Name))
		if !l.offline && !strings.EqualFold(r.Name, sharedRepo) {
			sources = append(sources, env.remote(r.Host, r.Owner, sharedRepo))
		}
	} else {
		cwd, err := env.Getwd()
		if err != nil {
			return discover.Result{}, err
		}
		root, ok := repoRoot(cwd)
		if !ok {
			return discover.Result{}, errNotInRepo
		}
		local, err := discover.OpenLocal(root)
		if err != nil {
			return discover.Result{}, err
		}
		closers = append(closers, local.Close)
		sources = append(sources, func() (discover.FS, error) { return local, nil })
		if !l.offline {
			sources = append(sources, env.orgFallback())
		}
	}
	return discover.Find(ctx, kind, sources...)
}

func (env *Env) remote(host, owner, name string) func() (discover.FS, error) {
	return func() (discover.FS, error) {
		client, err := env.NewClient(host)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s/%s on %s: %w (run \"gh auth login --hostname %s\")", owner, name, host, err, host)
		}
		return discover.NewRemote(client, host, owner, name), nil
	}
}

// orgFallback returns a source for the owner's shared .github repository.
// It is skipped when the working tree has no GitHub remote or when gh has
// no credentials for the host.
func (env *Env) orgFallback() func() (discover.FS, error) {
	return func() (discover.FS, error) {
		r, err := env.CurrentRepo()
		if err != nil || strings.EqualFold(r.Name, sharedRepo) {
			return nil, nil
		}
		client, err := env.NewClient(r.Host)
		if err != nil {
			warnf(env, "skipped the shared templates in %s/%s: %v", r.Owner, sharedRepo, err)
			return nil, nil
		}
		return discover.NewRemote(client, r.Host, r.Owner, sharedRepo), nil
	}
}

// repoRoot walks up from dir to the folder that holds .git.
func repoRoot(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// pick runs discovery and selection. It prints the problems that
// discovery found as warnings.
func (env *Env) pick(ctx context.Context, kind discover.Kind, l lookup, name string) (discover.Template, discover.Result, error) {
	res, err := env.find(ctx, kind, l)
	if err != nil {
		return discover.Template{}, res, err
	}
	warnProblems(env, res)
	t, err := discover.Select(res, name)
	return t, res, err
}
