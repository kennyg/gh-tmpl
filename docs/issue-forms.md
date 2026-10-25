# Issue form conversion

An issue form is a YAML file in `.github/ISSUE_TEMPLATE/`. On github.com, a form shows input fields. When someone submits it, GitHub writes the issue body as Markdown, with one `### <label>` section per field.

`gh issue create --body` cannot fill a form. `gh tmpl show issue --form <name>` prints the Markdown that GitHub would write, with empty sections. You fill in the sections and pass the result to `gh issue create --body-file`. The issue then looks as if someone submitted the form.

## Example

This form:

```yaml
name: Bug report
description: Report something that does not work.
title: "[Bug]: "
labels: ["bug"]
body:
  - type: markdown
    attributes:
      value: Thanks for the report.
  - type: textarea
    id: what-happened
    attributes:
      label: What happened?
      description: Tell us what you saw and what you expected.
      placeholder: The command exits with code 2.
    validations:
      required: true
  - type: dropdown
    id: os
    attributes:
      label: Operating system
      multiple: true
      options: [macOS, Linux, Windows]
  - type: checkboxes
    id: terms
    attributes:
      label: Code of conduct
      options:
        - label: I agree to follow the code of conduct.
          required: true
```

converts to this Markdown:

```markdown
### What happened?

<!-- Required. Tell us what you saw and what you expected. Example: The command exits with code 2. -->

### Operating system

<!-- Optional. Choose one or more, separated by commas: macOS, Linux, Windows. -->

### Code of conduct

<!-- Required. You must check: I agree to follow the code of conduct. -->
- [ ] I agree to follow the code of conduct.
```

## Conversion rules

| Element | Markdown |
| --- | --- |
| `markdown` | Left out. GitHub shows it on the form only. |
| `input`, `textarea` | `### <label>`, then a comment. A `value` is filled in as the default text. |
| `dropdown` | `### <label>`, then a comment that lists the options. A `default` option is filled in. |
| `checkboxes` | `### <label>`, a comment that names the required options, then one `- [ ] <label>` line per option. |

The comment holds these parts in order:

1. `Required.` or `Optional.`
2. The `description`
3. For a dropdown, the options
4. For checkboxes, the options you must check
5. For an input or textarea, the `placeholder` as an example
6. For a textarea with `render`, the language of the code block to use

Replace each comment with your answer. [The check](check-rules.md#issue-forms) reports comments left in the body.

For an empty optional field, GitHub writes `_No response_`. You can write the same, or leave the section empty.

## Form metadata

The text output holds only the body. `--json` also gives the metadata, so a script can set the title and labels:

```sh
gh tmpl show issue --form bug_report --json > form.json
jq -r .body form.json > body.md
title="$(jq -r .form.title form.json)Crash on start"
labels="$(jq -r '.form.labels | join(",")' form.json)"
$EDITOR body.md
gh tmpl issue create --tmpl-name bug_report --title "$title" --label "$labels" --body-file body.md
```

## Safety and limits

gh-tmpl decodes forms into fixed Go types. It never evaluates the YAML. It rejects files over 256 KiB, forms with more than 200 elements, and elements with more than 200 options. It rejects a form with an unknown element type, a field without a label, or a dropdown or checkbox list without options. `gh tmpl show issue --list` warns about each form it skips.

gh-tmpl breaks up `-->` and `<!--` in form text, so form text cannot end a comment early.
