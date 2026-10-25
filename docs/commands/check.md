# gh tmpl check

`gh tmpl check` checks an issue or pull request body against its template. It exits 1 if the body fails, and it tells you how to fix each problem.

```text
gh tmpl check [pr|issue] (--body-file <file> | --body <text>) [flags]
```

The kind is `pr` or `issue`. It defaults to `pr`.

## Flags

[The generated reference for `gh tmpl check`](../reference/check.md) lists every flag. These need more words:

- `--body-file -` reads the body from standard input. `--body` and `--body-file` cannot be given together. An empty body is a usage error, exit 2.
- `--template-file` checks against a template file on disk instead of discovering one. A `.yml` or `.yaml` file is an issue form.
- `--strict` fails on warnings too. `--allow-unchecked` reports an untouched checklist as a warning, not an error.
- `--strict`, `--allow-unchecked`, `--json` and `--offline` are switches. `=true` or `=1` turns a switch on, and `=false` or `=0` turns it off, as in `--strict=false`. The last one wins. Any other value is a usage error, exit 2.

## Text output

The first line starts with `PASS` or `FAIL`. Each finding follows, with its severity, its code, the body line if there is one, and a fix:

```console
$ gh tmpl check --body-file body.md
FAIL: the body does not follow .github/pull_request_template.md (1 error, 1 warning).

error[placeholder-comment] line 7: The template comment "<!-- How did you test the change? List the commands you ran. -->" is still in the body.
  fix: Follow the comment on line 7, then delete it.

warning[unchecked-checkbox] line 13: The item "- [ ] I updated the docs in docs/ or the README." is not checked.
  fix: Check it with "- [x]" if it applies. Delete the line if it does not.
```

## JSON output

```json
{
  "ok": false,
  "errors": 1,
  "warnings": 0,
  "findings": [
    {
      "severity": "error",
      "code": "missing-heading",
      "message": "The heading \"## Testing\" is missing.",
      "section": "## Testing",
      "fix": "Add the heading \"## Testing\" and write that section. Keep the heading text as it is in the template."
    }
  ],
  "template": {
    "kind": "pr",
    "id": "pull_request_template",
    "path": ".github/pull_request_template.md",
    "source": "local",
    "format": "markdown",
    "default": true
  }
}
```

| Field | Meaning |
| --- | --- |
| `ok` | `true` if there are no errors. |
| `findings[].severity` | `error` fails the check. `warning` does not, unless you pass `--strict`. |
| `findings[].code` | A stable code from [the rules](../check-rules.md). |
| `findings[].section` | The template heading the finding is about. |
| `findings[].line` | The 1-based line in the body. It is left out when the problem has no line, such as a missing heading. |
| `findings[].fix` | What to change. |

If the repository has no template of the kind, the check passes. The JSON then has `"template": null` and a `note`. On a usage error, the JSON has `"ok": false` and an `error` string. If there are several templates and no `--name`, or no template matches `--name`, the JSON also has a `templates` array with the ID, name and path of each template.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | The body passes, or there is no template of this kind. |
| 1 | The body fails the check. |
| 2 | Bad flags, an unreadable body, several templates and no `--name`, no match for `--name`, or a `--repo` repository that does not exist or that the token cannot read. |

## Examples

```sh
gh tmpl check --body-file body.md
gh tmpl check issue --name bug_report --body-file body.md --json
gh tmpl check --template-file .github/pull_request_template.md --body "$(cat body.md)"
gh pr view 42 --json body --jq .body | gh tmpl check --body-file -
```
