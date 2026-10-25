# Security policy

## Report a problem

Report security problems privately through GitHub. Open the [private vulnerability report form](https://github.com/kennyg/gh-tmpl/security/advisories/new) for this repository. Do not open a public issue or pull request for a security problem.

Give the version (`gh tmpl --version`), the steps to reproduce, and what an attacker can do. You get a first answer within seven days.

## Supported versions

Only the latest release gets security fixes. Run `gh extension upgrade tmpl` to get it.

## Scope

gh-tmpl reads templates from repositories and bodies from files. It passes the other flags to `gh`. These are in scope:

- gh-tmpl reads a file outside the repository, or prints a file that is not a template.
- Template or body content makes gh-tmpl run a command, or changes the arguments it gives to `gh`.
- gh-tmpl prints, logs or sends a token.
- Template or body content sends terminal escape sequences through gh-tmpl output.
- A template or body makes gh-tmpl use memory or time without limit.
- The release process lets someone change a published binary.

These are out of scope:

- Problems in `gh` or in go-gh. Report them to [cli/cli](https://github.com/cli/cli/security/policy).
- The text of a template. gh-tmpl shows templates as they are. A template that tells a reader or an AI agent to do something harmful is a problem in that repository.
- A check result that you disagree with. Open a normal issue.

## Security design

gh-tmpl treats every template and body as untrusted input.

| Risk | What gh-tmpl does |
| --- | --- |
| Code in templates | gh-tmpl never executes, renders or evaluates a template or body. It reads them line by line. |
| YAML parsing | gh-tmpl decodes issue forms and front matter into fixed Go types with `gopkg.in/yaml.v3`. That parser limits alias expansion. gh-tmpl rejects files over 256 KiB and turns a parser panic into an error. |
| Large input | Templates are limited to 256 KiB, bodies to 1 MiB, API responses to 2 MiB, and folder listings to 500 entries. |
| Symbolic links | gh-tmpl opens the local clone with `os.Root`. A link that points outside the clone fails. |
| Remote paths | gh-tmpl skips API entries whose names hold `/`, `\`, `..` or control characters. It skips symbolic links and submodules. |
| Terminal output | gh-tmpl removes control characters and bidirectional overrides from text output. JSON output escapes them. |
| Tokens | gh-tmpl uses go-gh, which reads the token that `gh auth login` stored. gh-tmpl never reads, prints or logs the token. It turns off go-gh HTTP logging, so `GH_DEBUG` does not print request headers. |
| Running gh | The create commands pass arguments to the `gh` binary as a list with `os/exec`. No shell parses them. |

The project also uses these controls:

- Every GitHub Action is pinned to a full commit SHA.
- Each workflow job has only the `permissions:` it needs. No workflow uses `pull_request_target`.
- CI runs `govulncheck`, `golangci-lint` with `gosec`, `actionlint` and `zizmor` on every pull request.
- Dependabot updates Go modules and Actions every week.
- Each release has a GitHub artifact attestation, signed through Sigstore, for every binary. See [docs/release.md](docs/release.md) to verify one.
