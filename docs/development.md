# Development

The project uses [mise](https://mise.jdx.dev) for tools and tasks, and [hk](https://hk.jdx.dev) for git hooks. CI runs the same mise tasks, so a local pass predicts a CI pass.

## Set up

```sh
mise trust
mise install
hk install
```

`mise install` installs the versions in `mise.toml`: Go, golangci-lint, GoReleaser, govulncheck, hk, pkl, actionlint, zizmor and typos. `hk install` adds the git hooks.

## Tasks

| Task | What it runs |
| --- | --- |
| `mise run build` | Builds `./gh-tmpl` |
| `mise run test` | `go test ./...` |
| `mise run test:race` | The tests with the race detector |
| `mise run lint` | golangci-lint, with the config in `.golangci.yml` |
| `mise run vuln` | govulncheck |
| `mise run lint:actions` | actionlint and zizmor on `.github/` |
| `mise run release:check` | `goreleaser check` |
| `mise run release:snapshot` | Builds every release binary into `dist/` without publishing |
| `mise run check` | test, lint, vuln, lint:actions and release:check |
| `mise run install-local` | Builds the binary and runs `gh extension install .` |

## Git hooks

`hk.pkl` defines the hooks.

| Hook | Steps |
| --- | --- |
| `pre-commit` | gofmt, `go mod tidy`, golangci-lint, actionlint, zizmor, typos, whitespace and line-ending fixes, merge conflict markers, private keys. It fixes what it can. |
| `pre-push` | The pre-commit steps, plus `go test ./...` |

Run `hk check --all` to run the checks on every file. Run `hk fix --all` to apply the fixes.

## Try your build

```sh
mise run install-local
gh tmpl --version
```

`gh extension install .` needs the folder name to start with `gh-`, so run it from a clone in a folder named `gh-tmpl`. The version is `dev` for a local build. To remove the local install, run `gh extension remove tmpl`.

## Code layout

| Package | Job |
| --- | --- |
| `main.go` | Entry point. Sets the version. |
| `internal/cli` | Commands, flags, output, and the call to `gh` |
| `internal/discover` | Template search, the local and API file readers, front matter |
| `internal/form` | Issue form parsing and conversion to Markdown |
| `internal/check` | The Markdown scanner and the check rules |

## Tests

Tests use fixture repositories in `internal/discover/testdata/repos/`. Each folder is a small repository with templates in different places. `internal/form/testdata/` holds a form and its expected Markdown. To update the expected Markdown after a deliberate change, run:

```sh
go test ./internal/form -run TestParseAndRender -update
```

The form and check packages have fuzz tests:

```sh
go test ./internal/form -fuzz FuzzParse -fuzztime 30s
go test ./internal/check -fuzz FuzzMarkdown -fuzztime 30s
```
