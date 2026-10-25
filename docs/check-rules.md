# How the check decides pass or fail

The check compares the body with the template. Each problem is a finding with a severity. An `error` fails the check. A `warning` does not fail it, unless you pass `--strict`. The check passes if there are no errors.

## Markdown templates

gh-tmpl reads the template and the body line by line. It looks for four things: ATX headings such as `## Testing`, task list items such as `- [ ] Tests added`, HTML comments, and fenced code blocks. Headings, checkboxes and comments inside a fenced code block do not count.

| Code | Severity | The check reports it when |
| --- | --- | --- |
| `empty-body` | error | The body is empty or only white space. |
| `missing-heading` | error | A template heading is not in the body. |
| `empty-section` | error | A section has no text in the body, but has text, a comment or a checklist in the template. |
| `unchanged-section` | error | A section holds the same example text as the template. The text must not be only checkboxes. |
| `placeholder-comment` | error | An HTML comment from the template is still in the body. |
| `untouched-checklist` | error | Every checkbox of a template checklist is still in the body, and none is checked. |
| `unchecked-checkbox` | warning | Some checkboxes of a checklist are checked or removed, and this one is still unchecked. |

### Headings

Headings match by text. The level, the order, the case, emphasis markers such as `**` or `_`, backticks, extra spaces and a trailing colon do not matter. So `# testing:` in the body matches `## Testing` in the template.

A body section runs from its heading to the next heading that matches a template heading. You can add your own headings, such as `### Screenshots`, inside a section.

A template heading with no text below it, such as a heading followed by a subheading, does not need text in the body.

### Comments

Template comments usually tell the writer what to put in a section. A comment from the template that is still in the body is an error. White space inside the comment does not matter for the match. Your own comments are fine.

Tool comments may stay. gh-tmpl ignores comments that start with `markdownlint`, `prettier-ignore`, `cspell`, `vale`, `textlint`, `toc`, `all-contributors`, `lint`, `eslint` or `remark`.

### Checklists

A checklist is the set of template checkboxes under one heading. gh-tmpl counts a checkbox as touched if the body has it checked, with `[x]` or `[X]`, or if the body does not have it.

- If no checkbox of a checklist is touched, the check reports `untouched-checklist`. That is an error, or a warning with `--allow-unchecked`.
- If some are touched, each checkbox that is still unchecked is an `unchecked-checkbox` warning.

So a writer can check the items that apply and delete the items that do not. A checklist of choices, such as "Bug fix" and "New feature", passes when one is checked.

## Issue forms

For an issue form, the body must have the layout that GitHub writes for a submitted form. See [Issue form conversion](issue-forms.md).

| Code | Severity | The check reports it when |
| --- | --- | --- |
| `empty-body` | error | The body is empty. |
| `missing-heading` | error | A field has no `### <label>` heading. |
| `required-field-empty` | error | A field with `validations: required: true` is empty. `_No response_`, `None` for a dropdown, and an empty code block count as empty. |
| `required-checkbox` | error | A checkbox option with `required: true` is not checked. |
| `unknown-option` | warning | A dropdown value is not one of the options. For a multiple choice dropdown, gh-tmpl splits the value on commas. |
| `placeholder-comment` | error | A comment from `gh tmpl show issue --form` is still in the body. |

Optional fields can be empty. Optional checkboxes can stay unchecked.

## Options

| Flag | Effect |
| --- | --- |
| `--strict` | Every warning becomes an error. |
| `--allow-unchecked` | An untouched checklist becomes a warning. |

## What the check does not do

- It does not judge the quality of the text. "N/A" passes as section content.
- It does not read setext headings, which are text underlined with `===` or `---`. It also does not read bold text used as a heading. Use ATX headings such as `## Summary` in templates.
- It does not check the title, labels or assignees.
