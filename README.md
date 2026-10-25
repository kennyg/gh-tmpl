# gh-tmpl

A GitHub CLI extension that fills and checks pull request and issue bodies against a repository's templates.

`gh pr create --body` and `gh issue create --body` skip the repository's template. AI agents nearly always pass `--body`, so their pull requests and issues often miss the sections a project asks for. gh-tmpl finds the template, prints it for the writer to fill, and checks the body before `gh` creates anything.

```console
$ gh tmpl check --body-file body.md
FAIL: the body does not follow .github/pull_request_template.md (2 errors, 0 warnings).

error[missing-heading]: The heading "## Testing" is missing.
  fix: Add the heading "## Testing" and write that section. Keep the heading text as it is in the template.

error[untouched-checklist] line 7: No item in the checklist under "## Checklist" is checked or removed (3 items).
  fix: Check each item that applies with "- [x]". Delete each item that does not apply.
```

## Install

You need [GitHub CLI](https://cli.github.com) 2.0 or later.

```sh
gh extension install kennyg/gh-tmpl
```

`gh` downloads the precompiled binary for your platform from the latest release. To upgrade, run `gh extension upgrade tmpl`.

## Quick start

Print the pull request template, fill it in, and check it:

```sh
gh tmpl show pr > body.md
$EDITOR body.md
gh tmpl check --body-file body.md
```

Create the pull request only if the body passes. gh-tmpl passes every other flag to `gh pr create`:

```sh
gh tmpl pr create --title "Fix the login loop" --body-file body.md
```

For an issue with several templates, list them, then pick one:

```sh
gh tmpl show issue --list
gh tmpl show issue --form bug_report > body.md
gh tmpl issue create --tmpl-name bug_report --title "Crash on start" --body-file body.md
```

## Commands

| Command | What it does |
| --- | --- |
| [`gh tmpl show`](docs/commands/show.md) | Prints a template, or lists the templates. Prints an issue form as Markdown sections. |
| [`gh tmpl check`](docs/commands/check.md) | Checks a body against a template. Exits 1 if the body fails. Use `--json` for a report that a program can read. |
| [`gh tmpl pr create`](docs/commands/create.md) | Runs the check, then runs `gh pr create` with the other flags. |
| [`gh tmpl issue create`](docs/commands/create.md) | Runs the check, then runs `gh issue create` with the other flags. |

## Documentation

- [Command reference](docs/reference/index.md), generated from [`gh-tmpl.usage.kdl`](gh-tmpl.usage.kdl)
- [Where gh-tmpl looks for templates](docs/template-discovery.md)
- [How the check decides pass or fail](docs/check-rules.md)
- [Proofs of the core rules](docs/verification.md), with Lean
- [Issue form conversion](docs/issue-forms.md)
- [Use with AI agents](docs/ai-agents.md), with an `AGENTS.md` and `CLAUDE.md` snippet
- [GitHub Enterprise hosts](docs/enterprise.md)
- [Release process](docs/release.md)
- [Development](docs/development.md)
- [Security policy and design](SECURITY.md)

## Shell completions and man page

`gh` does not complete the commands of extensions. If you put the `gh-tmpl` binary on your `PATH`, you can complete `gh-tmpl` itself. Each release ships the scripts and a man page, and the binary prints the scripts:

```sh
gh-tmpl completion zsh > "${fpath[1]}/_gh-tmpl"
gh-tmpl completion bash > ~/.local/share/bash-completion/completions/gh-tmpl
gh-tmpl completion fish > ~/.config/fish/completions/gh-tmpl.fish
```

The binary answers each completion request from its own tables. The scripts need no other tool. The scripts are also in [`completions/`](completions), and the man page is [`man/gh-tmpl.1`](man/gh-tmpl.1).

## License

[MIT](LICENSE)
