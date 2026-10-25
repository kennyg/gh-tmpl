# Where gh-tmpl looks for templates

gh-tmpl searches the same places as GitHub, in the same order. If the repository has no template of the kind, gh-tmpl reads the owner's shared `.github` repository. `gh` itself does not read that repository.

## Locations

gh-tmpl searches three locations in this order of precedence:

1. `.github/`
2. The repository root
3. `docs/`

In each location it looks for two things.

| Kind | Single-file template | Template folder |
| --- | --- | --- |
| Pull request | `pull_request_template.md` | `PULL_REQUEST_TEMPLATE/` |
| Issue | `issue_template.md` | `ISSUE_TEMPLATE/` |

Names match without regard to case, so `PULL_REQUEST_TEMPLATE.md` also counts. A single-file template can end in `.md`, `.markdown`, `.txt`, or have no extension.

In a template folder, gh-tmpl reads these files:

| File | Kind | Format |
| --- | --- | --- |
| `*.md`, `*.markdown`, `*.txt` | Pull request and issue | Markdown, with optional YAML front matter |
| `*.yml`, `*.yaml` | Issue only | [Issue form](issue-forms.md) |
| `config.yml`, `config.yaml` | None | GitHub's template chooser settings. gh-tmpl skips it. |

gh-tmpl skips subfolders, other file types, and files over 256 KiB.

## Precedence

The first location that has a single-file template wins. Single-file templates in later locations are ignored. The same rule applies to template folders. A folder counts only if it holds at least one template.

For example, take a repository with these files:

```text
.github/pull_request_template.md
pull_request_template.md
docs/PULL_REQUEST_TEMPLATE/release.md
```

gh-tmpl finds `.github/pull_request_template.md` and `docs/PULL_REQUEST_TEMPLATE/release.md`. It ignores the root `pull_request_template.md`, because `.github/` has a single-file template. `gh tmpl show --list` reports ignored files at the end of the list.

## Which template gh-tmpl uses

With one template, gh-tmpl uses it. With several, it needs a name, with two exceptions:

- For a pull request, gh-tmpl uses the single-file template. GitHub does the same when you open a pull request.
- With `--name`, gh-tmpl uses the template that matches the ID, file name, path, or front matter or form `name`.

The ID is the file name without its extension.

## The owner's shared .github repository

GitHub uses the templates in an owner's `.github` repository for every repository of that owner that has no templates of its own. gh-tmpl reads that repository too.

| You run gh-tmpl | First source | Fallback |
| --- | --- | --- |
| In a clone | The working tree on disk | `OWNER/.github` on the host of the clone's remote |
| With `--repo OWNER/REPO` | `OWNER/REPO` through the API | `OWNER/.github` through the API |

gh-tmpl reads the fallback only if the first source has no template of the kind. It searches the fallback with the same locations and precedence.

The first source must exist. If the `--repo` repository does not exist, or the token cannot read it, gh-tmpl stops with exit code 2. It does not read the fallback, so a typing error in `--repo` cannot make a check pass. A missing `OWNER/.github` repository is normal, and gh-tmpl treats it as having no templates.

gh-tmpl reads the clone's remote, or `GH_REPO`, to find the owner and host. It uses the GitHub REST contents API on the repository's default branch. If there is no GitHub remote, gh-tmpl skips the fallback. If `gh` has no login for the host, gh-tmpl prints a note and skips the fallback.

Use `--offline` to skip the fallback. With it, gh-tmpl makes no API requests in a clone.

## The local clone

In a clone, gh-tmpl reads files from the working tree, not from a branch. It finds the clone root by walking up from the working folder to the folder that holds `.git`. Uncommitted changes to a template count.

gh-tmpl opens the clone with Go's `os.Root`. A symbolic link that points outside the clone fails, so a template link cannot make gh-tmpl print another file.
