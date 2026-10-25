# gh tmpl show

`gh tmpl show` prints a pull request or issue template. With several templates and no `--name`, it lists them and exits 2.

```text
gh tmpl show [pr|issue] [flags]
```

The kind is `pr` or `issue`. It defaults to `pr`.

## Flags

| Flag | Meaning |
| --- | --- |
| `--name <template>` | Pick a template by ID, file name, path, or the `name` in its front matter or form. Case does not matter. |
| `--form <name>` | Pick an issue form and print it as Markdown. Implies `issue`. Fails for a Markdown template. |
| `--list` | List the templates and exit 0. |
| `--raw` | Print the file as it is, with front matter or YAML. |
| `--json` | Print JSON instead of text. |
| `-R`, `--repo <[HOST/]OWNER/REPO>` | Read the templates of another repository through the API. |
| `--offline` | Do not read the owner's shared `.github` repository. |

The ID of a template is its file name without the extension. For `.github/ISSUE_TEMPLATE/bug_report.yml`, the ID is `bug_report`.

## Pick a template

gh-tmpl picks a template in this order:

1. With `--name` or `--form`, it uses the template that matches.
2. With one template, it uses that template.
3. For a pull request, it uses the single-file template, such as `.github/pull_request_template.md`. GitHub fills new pull requests with that file.
4. Otherwise, it lists the templates on standard error and exits 2.

See [Where gh-tmpl looks for templates](../template-discovery.md) for the search order.

## Output

The text output is the template body. gh-tmpl removes the front matter of a Markdown template. It converts an issue form to Markdown, as [Issue form conversion](../issue-forms.md) describes. Use `--raw` to keep the file as it is.

`--list` prints a table:

```console
$ gh tmpl show issue --list
ID               NAME             PATH
bug_report       Bug report       .github/ISSUE_TEMPLATE/bug_report.yml (form)
feature_request  Feature request  .github/ISSUE_TEMPLATE/feature_request.yml (form)
```

`--json` prints one object for a template:

```json
{
  "kind": "issue",
  "id": "bug_report",
  "name": "Bug report",
  "about": "Report something in gh-tmpl that does not work as the docs say.",
  "path": ".github/ISSUE_TEMPLATE/bug_report.yml",
  "source": "local",
  "format": "form",
  "default": false,
  "body": "### What happened?\n\n<!-- Required. Tell us what you ran, what you expected, and what you saw. -->\n\n...",
  "form": {
    "name": "Bug report",
    "description": "Report something in gh-tmpl that does not work as the docs say.",
    "labels": ["bug"]
  }
}
```

`source` is `local` for the clone in the working folder. For a template from the API it is `HOST/OWNER/REPO`, for example `github.com/octo/.github`. With `--list --json`, the output holds a `templates` array of the same objects, without `body` and `form`.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | gh-tmpl printed the template or the list. |
| 2 | Bad flags, no template, several templates and no `--name`, no match for `--name`, or a `--repo` repository that does not exist or that the token cannot read. |

## Examples

```sh
gh tmpl show > body.md
gh tmpl show pr --name feature
gh tmpl show issue --form bug_report > body.md
gh tmpl show issue -R cli/cli --list
gh tmpl show issue --name bug_report --json | jq -r .form.title
```
