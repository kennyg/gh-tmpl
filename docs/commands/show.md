# gh tmpl show

`gh tmpl show` prints a pull request or issue template. With several templates and no `--name`, it lists them and exits 2.

```text
gh tmpl show [pr|issue] [flags]
```

The kind is `pr` or `issue`. It defaults to `pr`.

## Flags

[The generated reference for `gh tmpl show`](../reference/show.md) lists every flag. These need more words:

- `--name` picks a template by ID, file name, path, or the `name` in its front matter or form. Case does not matter.
- `--form` picks an issue form and prints it as Markdown. It implies `issue`, and it fails for a Markdown template or with `pr`.
- `--repo` reads the templates of another repository through the API. See [Enterprise hosts](../enterprise.md) for the host rules.
- `--offline` skips the owner's shared `.github` repository.

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
  },
  "lookup_warnings": []
}
```

`source` is `local` for the clone in the working folder. For a template from the API it is `HOST/OWNER/REPO`, for example `github.com/octo/.github`. With `--list --json`, the output holds a `templates` array of the same objects, without `body` and `form`.

`lookup_warnings` lists the template files that gh-tmpl skipped because it could not read or parse them. See [Templates that cannot be read](../template-discovery.md#templates-that-cannot-be-read).

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
