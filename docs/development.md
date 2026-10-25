# Development

The project uses [mise](https://mise.jdx.dev) for tools and tasks, and [hk](https://hk.jdx.dev) for git hooks. CI runs the same mise tasks, so a local pass predicts a CI pass.

## Set up

```sh
mise trust
mise install
hk install
```

`mise install` installs the versions in `mise.toml`: Go, golangci-lint, GoReleaser, govulncheck, hk, pkl, actionlint, zizmor, typos and usage. `hk install` adds the git hooks.

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
| `mise run generate` | Regenerates the parser, the command reference, the man page and the completions from `gh-tmpl.usage.kdl` |
| `mise run generate:check` | Fails if `mise run generate` would change a file |
| `mise run check` | test, lint, vuln, lint:actions, release:check and generate:check |
| `mise run verify` | Checks the Lean proofs in `verify/` and writes the Lean test cases again. Needs elan. |
| `mise run verify:check` | Fails if a proof fails or a Lean test case file is out of date, then runs the differential tests |
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

## The command line spec

[`gh-tmpl.usage.kdl`](../gh-tmpl.usage.kdl) defines every command, flag and argument once, in the [usage](https://usage.jdx.dev) spec format. Four outputs come from it:

| Output | Made by | Path |
| --- | --- | --- |
| The Go parser, help text and completion tables | `usage generate go` | `internal/spec/spec.go` |
| The command reference | `usage generate markdown` | `docs/reference/` |
| The man page | `usage generate manpage` | `man/gh-tmpl.1` |
| Shell completion scripts | `gh-tmpl completion <shell>`, which reads the generated tables | `completions/` |

To change the command line, edit the spec, run `mise run generate`, and commit the spec with the generated files. Do not edit generated files by hand. CI runs `mise run generate:check`, which fails if the generated files do not match the spec.

The usage Go framework is a development preview, and its APIs can change. `mise.toml` pins the usage CLI to 7.1.2. `go.mod` pins `github.com/jdx/usage/go` to the pseudo-version of the same release commit, because the Go module has no version tags of its own. Update both together, then run `mise run generate`.

The generated parser does not enforce `subcommand_required`. The dispatcher in `internal/cli/cli.go` handles `gh tmpl pr` and `gh tmpl issue` without `create`.

## Code layout

| Package | Job |
| --- | --- |
| `main.go` | Entry point. Sets the version. |
| `internal/spec` | Generated parser tables. Do not edit. |
| `internal/cli` | Dispatch on the parsed command line, output, and the call to `gh` |
| `internal/discover` | Template search, the local and API file readers, front matter |
| `internal/form` | Issue form parsing and conversion to Markdown |
| `internal/check` | The Markdown scanner and the check rules |
| `internal/leantest` | Reads the test cases that the Lean models write |
| `verify/` | Lean models of the argument split, the check rules and the lookup order, with proofs. The Go build does not use it. |

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

The cli, check and discover packages have differential tests against the Lean models in `verify/`. The cases are in `testdata/lean/` in each package, and `go test` runs them without Lean. See [Proofs of the core rules](verification.md).
