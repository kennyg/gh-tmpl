# Proofs of the core rules

The `verify/` folder holds a [Lean 4](https://lean-lang.org) model of three parts of gh-tmpl. Lean checks proofs about the model. Lean cannot read Go, so Go tests compare the Go code with the model on thousands of generated cases. These tests are differential tests.

The Go build does not use `verify/`. Only the Go tests read the cases that the model writes.

## What is proved

Each theorem is about the Lean model, not about the Go code.

| Area | Go code | Theorem | Statement |
| --- | --- | --- | --- |
| Argument split | `splitCreate` | `words_tokenize` | The tokens hold every input word once, in order. No word is dropped, split, joined or moved. |
| Argument split | `splitCreate` | `valid_tokenize` | The tokens follow the grammar. A flag that takes a value, of gh-tmpl or of gh, takes the next word. A value is never read as a flag. |
| Argument split | `splitCreate` | `tokenize_unique` | No other token list follows the grammar and holds the same words. |
| Argument split | `splitCreate` | `split_interleave`, `split_keeps_words` | The gh-tmpl words and the gh words interleave to the input. Each side keeps the input order. |
| Check rules | `check.Markdown` | `pass_keeps_headings` | If the check passes, the body has each template heading at least as often as the template has it. |
| Check rules | `check.Markdown` | `missing_heading_count` | The check reports one `missing-heading` finding for each heading that the body lacks. |
| Check rules | `check.Markdown` | `pass_has_no_placeholder`, `placeholder_reported` | A pass means that no template comment is left. Each comment that is left is reported at its line. |
| Check rules | `check.Markdown` | `pass_touches_checklists` | Without `--allow-unchecked`, a pass means that each checklist has a checked or removed item. |
| Check rules | `check.Markdown` | `strict_pass_touches_every_checkbox` | With `--strict`, a pass means that every template checkbox is checked or removed. |
| Check rules | `check.Markdown` | `untouched_reported` | Each untouched checkbox is in a finding. |
| Lookup order | `discover.Find` | `find_unreadable_first` | A first source that is missing or that cannot be read is an error, never a pass with no templates. |
| Lookup order | `discover.Find` | `find_first_with_templates` | The templates come from the first source that has any. Each source before it was skipped, a missing fallback, or empty. The problems of that source and of each source before it are kept, so a fallback cannot hide a broken template. |
| Lookup order | `discover.Find` | `find_no_templates_problems` | A lookup without templates keeps the problems of every source. |
| Lookup order | `discover.FindIn` | `findIn_defaults` | The single-file template is the first candidate in the documented order. |
| Lookup order | `discover.FindIn` | `findIn_folder` | The folder templates come from the first template folder in that order that holds a template. |

The documented order is `.github/`, then the repository root, then `docs/`, and by name inside each location. See [Where gh-tmpl looks for templates](template-discovery.md).

## What is only tested

Some parts of the model have no theorems. The differential tests still compare them with the Go code.

| Model | Go code | Not proved |
| --- | --- | --- |
| `Argv.scan` | `scanCreate` | Which body, body file and repository the scan finds in the gh words |
| `Argv.parses`, `Argv.tmplName` | `Run` and the generated parser | Whether the parser accepts the gh-tmpl words, and which `--tmpl-name` it reads |
| `Check.headingPart` | `check.Markdown` | The `empty-section` and `unchanged-section` rules |
| `Check.check` | `check.Markdown` | The order, sections and lines of the findings |
| `Lookup.findIn` | `discover.FindIn` | The shadowed paths and the count of problems in one repository |

The issue form check, `check.Form`, has no model.

## The trust gap

A proof covers the model. Three things link the model to the Go code, and none of them is a proof.

The first link is the differential tests. They cover only the generated cases. The check cases use five kinds of lines: headings, text, comments, checkboxes and blank lines. The Go scanner handles more, such as code fences, comments over several lines, setext headings and emphasis in headings. The model does not have these, and no generated case has them.

The second link is the flag tables. The model has its own copy of the gh-tmpl flags and of `ghFlags`. The argument test compares both copies with the Go tables.

The third link is review. A person must read the model next to the Go code and judge that they say the same thing. The comments in each Lean file name the Go function that it models.

Some parts are outside the model completely:

- The generated usage parser. The tests run it on every case, but no proof covers it. gh-tmpl gives it the gh words after a `--`, where it keeps every word. The parser rejects a separate value that starts with `-`, so gh-tmpl attaches each gh-tmpl value to its flag, as in `--tmpl-name=-x`.
- `gh` itself. The model assumes that `gh` reads a flag and its value the way the grammar says.
- The file readers. The lookup model uses a repository in memory. It does not cover the API client, the file size limit or the limit of 100 templates per folder. In the model, every issue form fails to parse.
- Text handling such as Unicode case folding. The cases use ASCII.

## Run the proofs

Install [elan](https://github.com/leanprover/elan), the Lean version manager:

```sh
brew install elan-init
```

The file `verify/lean-toolchain` pins the Lean version. elan downloads it on the first build. The model uses only the Lean core library, so the build takes a few seconds.

| Task | What it runs |
| --- | --- |
| `mise run verify` | `lake build --wfail` in `verify/`, then `lake exe corpus`, which writes the cases again |
| `mise run verify:check` | `mise run verify`, then fails if a case file changed, then runs the differential tests |
| `go test -run MatchesLean ./internal/...` | The differential tests alone. They need no Lean. |

`lake build --wfail` fails on any warning, so a proof with `sorry` fails the build.

CI runs the differential tests in every `go test` job. The CI job "lean proofs and test cases" installs elan and runs `mise run verify:check`. `mise run check` does not run the proofs, so it works without elan.

## Change the code

When you change one of the three parts, change the model too:

1. Edit the Go code and the Lean file that models it.
2. Run `mise run verify:check`.
3. If a proof fails, fix the proof, or decide that the theorem no longer holds and update this page.
4. Commit the Go code, the model and the case files together.

If you change only the Go code, the differential tests fail. That failure is the point of the tests.

## Files

| Path | Contents |
| --- | --- |
| `verify/GhTmpl/Argv.lean` | The argument split model and its theorems |
| `verify/GhTmpl/Check.lean` | The check rule model and its theorems |
| `verify/GhTmpl/Lookup.lean` | The lookup order model and its theorems |
| `verify/Corpus.lean` | The case generator |
| `internal/*/testdata/lean/*.jsonl` | The generated cases, one JSON object per line |
| `internal/leantest` | The Go helper that reads the cases |
| `internal/*/lean_test.go` | The differential tests |
