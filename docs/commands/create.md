# gh tmpl pr create and gh tmpl issue create

`gh tmpl pr create` and `gh tmpl issue create` check the body, then run `gh pr create` or `gh issue create`. If the check fails, they print the report and exit 1. Nothing is created.

```text
gh tmpl pr create [gh pr create flags] [gh-tmpl flags]
gh tmpl issue create [gh issue create flags] [gh-tmpl flags]
```

## How it works

1. gh-tmpl reads the body from `--body`, `-b`, `--body-file` or `-F`. It accepts the same forms as `gh`, such as `--body-file=b.md`, `-Fb.md` and `-dF b.md`.
2. It finds the template, the same way as [gh tmpl check](check.md). With `-R` or `--repo`, it reads that repository's templates.
3. It checks the body. On failure it prints the report to standard error and exits 1.
4. On success it runs `gh` with every argument except the gh-tmpl flags. It prints warnings to standard error first.

gh-tmpl gives the arguments to the `gh` binary as a list. No shell reads them. For `--body-file -`, gh-tmpl reads standard input for the check, then passes the same bytes to `gh`.

gh-tmpl uses the `gh` binary that runs the extension. It reads the `GH_PATH` variable that `gh` sets for extensions.

## gh-tmpl flags

The gh-tmpl flags start with `--tmpl-`, so they cannot clash with a `gh` flag. The generated reference lists them for [`gh tmpl pr create`](../reference/pr/create.md) and [`gh tmpl issue create`](../reference/issue/create.md).

| Flag | Meaning |
| --- | --- |
| `--tmpl-name <template>` | Check against this template. |
| `--tmpl-strict` | Fail on warnings too. |
| `--tmpl-allow-unchecked` | Report an untouched checklist as a warning, not an error. |
| `--tmpl-offline` | Do not read the owner's shared `.github` repository. |
| `--tmpl-skip` | Run `gh` without the check. |

`-h` and `--help` show the gh-tmpl help for the command. Run `gh pr create --help` for the `gh` flags.

## When gh-tmpl does not check

| Case | What happens |
| --- | --- |
| No `--body` and no `--body-file`, for example with `--fill`, `--web` or the editor | gh-tmpl prints a note and runs `gh`. There is no body to check. |
| The repository has no template of the kind | gh-tmpl prints a note and runs `gh`. |
| Several templates and no `--tmpl-name` | gh-tmpl lists them and exits 2. It does not run `gh`. |

## Output and exit codes

`gh` prints the URL of the new pull request or issue on standard output. gh-tmpl writes its own messages to standard error, so a script can still read the URL.

| Code | Meaning |
| --- | --- |
| 1 | The body failed the check. gh-tmpl did not run `gh`. |
| 2 | Bad gh-tmpl flags, no single template to check against, or a `--repo` repository that does not exist or that the token cannot read. |
| Other | The exit code of `gh`. |

## Examples

```sh
gh tmpl pr create --title "Fix the login loop" --body-file body.md --draft
gh tmpl pr create -t "Fix" -F - < body.md
gh tmpl issue create --tmpl-name bug_report --title "Crash on start" --body-file body.md --label bug
gh tmpl pr create --tmpl-strict --title "Docs" --body-file body.md
```

## Limits

The parser removes a bare `--` before it passes the arguments on. `gh pr create` and `gh issue create` take no positional arguments, so `--` has no effect on them.

A value that is spelled like a gh-tmpl flag, as in `--title --tmpl-skip`, is read as the gh-tmpl flag. Write it as `--title=--tmpl-skip`.

gh-tmpl knows which `gh pr create` and `gh issue create` flags take a value. It needs that list to find the body in the arguments. If a new `gh` release adds a flag that takes a value, write it as `--flag=value`. gh-tmpl then passes it through unchanged. A test compares the list with the `--help` output of the installed `gh`, so CI notices a new flag.
