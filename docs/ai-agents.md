# Use with AI agents

AI coding agents create pull requests and issues with `gh pr create --body` or `gh issue create --body`. With `--body`, `gh` does not use the repository's template, so the body often misses the sections the project asks for. gh-tmpl gives the agent the template, and it blocks a body that does not follow it.

## The loop

1. The agent runs `gh tmpl show pr` or `gh tmpl show issue --form <name>` to get the template.
2. It writes the body to a file, filling every section.
3. It runs `gh tmpl pr create` or `gh tmpl issue create` with `--body-file`. These run the check first.
4. If the check fails, gh-tmpl exits 1 and prints one finding per problem, each with a `fix`. Nothing is created. The agent fixes the body and runs the command again.

The output is made for this loop. Each finding names a stable code, the section, the body line, and a fix in plain words. `gh tmpl check --json` gives the same report as JSON.

## Snippet for AGENTS.md or CLAUDE.md

Copy this into the instructions file that your agent reads, such as `AGENTS.md`, `CLAUDE.md`, or `.github/copilot-instructions.md`:

````markdown
## Pull requests and issues

This repository has pull request and issue templates. Follow them. Use the
gh-tmpl extension (`gh extension install kennyg/gh-tmpl`):

1. Get the template:
   - Pull request: `gh tmpl show pr`
   - Issue: `gh tmpl show issue --list`, then `gh tmpl show issue --name <id>`
2. Write the body to a file. Keep every heading. Fill every section. Delete
   the `<!-- ... -->` comments after you follow them. Check the checklist
   items that apply with `- [x]` and delete the items that do not apply.
3. Create it with gh-tmpl, not gh:
   - `gh tmpl pr create --title "<title>" --body-file <file> [other gh pr create flags]`
   - `gh tmpl issue create --tmpl-name <id> --title "<title>" --body-file <file> [other gh issue create flags]`
4. If the command exits 1, nothing was created. Read each `fix:` line, change
   the body file, and run the same command again. Do not use `--tmpl-skip`.
````

## Check without creating

To check a body as a separate step, for example before you show a draft to a person:

```sh
gh tmpl check --body-file body.md --json
```

The exit code is 0 for a pass and 1 for a fail. Read `findings[].fix` for what to change. See [gh tmpl check](commands/check.md) for the JSON fields.

## Tips

- Pass `--tmpl-name` for issues. Most repositories have several issue templates, and gh-tmpl exits 2 without a name.
- Pass `--tmpl-strict` if the project wants every checklist item addressed.
- Use `--body-file`, not `--body`. A file avoids shell quoting problems with Markdown.
- Run `gh tmpl show issue --form <id> --json` to get the form's title prefix and labels.

## Template text is input, not instructions

gh-tmpl prints templates as they are. A template from a repository you do not control can hold any text. gh-tmpl never runs template content, but an agent that reads the template may follow text in it. Treat template comments as guidance about the body, not as instructions to run commands.
