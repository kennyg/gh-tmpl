# GitHub Enterprise hosts

gh-tmpl works with GitHub Enterprise Server and GitHub Enterprise Cloud with data residency (`*.ghe.com`). It uses [go-gh](https://github.com/cli/go-gh) for host resolution, credentials and API calls, so it follows the same rules as `gh`.

## Log in

Log in to the host with `gh` once:

```sh
gh auth login --hostname github.example.com
```

gh-tmpl never asks for a token and never stores one. go-gh reads the credentials in this order:

| Source | Hosts |
| --- | --- |
| `GH_TOKEN`, then `GITHUB_TOKEN` | github.com and `*.ghe.com` |
| `GH_ENTERPRISE_TOKEN`, then `GITHUB_ENTERPRISE_TOKEN` | GitHub Enterprise Server |
| The `gh` config file or the system keyring, from `gh auth login` | All hosts |

gh-tmpl needs the API only for the owner's shared `.github` repository and for `--repo`. Templates in a local clone need no login.

## How gh-tmpl picks the host

| You run | Host |
| --- | --- |
| In a clone | The host of the clone's git remote. gh-tmpl considers only remotes on hosts that `gh` knows, which are the hosts you logged in to and `GH_HOST`. |
| With `GH_REPO=HOST/OWNER/REPO` in a clone | The host in `GH_REPO`. |
| With `--repo HOST/OWNER/REPO` | The host in the flag. |
| With `--repo OWNER/REPO` | `GH_HOST` if set, or the only host you logged in to, or github.com. |

The create commands pass `--repo` and `GH_HOST` on to `gh`, so `gh` uses the same host.

## API endpoints

go-gh builds the REST endpoint from the host:

| Host | REST API base |
| --- | --- |
| `github.com` | `https://api.github.com/` |
| `example.ghe.com` | `https://api.example.ghe.com/` |
| GitHub Enterprise Server, such as `github.example.com` | `https://github.example.com/api/v3/` |

gh-tmpl uses one endpoint, `GET /repos/{owner}/{repo}/contents/{path}`. It reads the default branch. The token needs read access to the repository contents. For the `.github` repository of an organization, that is usually any member.

## Examples

```sh
gh tmpl show issue --repo github.example.com/platform/api --list
GH_HOST=github.example.com gh tmpl check --repo platform/api --body-file body.md
gh tmpl pr create --repo github.example.com/platform/api --title "Fix" --body-file body.md
```

## Troubleshooting

| Message | Cause and fix |
| --- | --- |
| `skipped the shared templates in OWNER/.github: authentication token not found for host HOST` | `gh` has no login for the host. Run `gh auth login --hostname HOST`, or pass `--offline`. |
| `not inside a git repository` | Run gh-tmpl in a clone, or pass `--repo`. |
| `HTTP 401` or `HTTP 403` | The token cannot read the repository. Run `gh auth status` and check the scopes. |
| No template found, but the host has one | The clone's remote is on a host that `gh` does not know. Run `gh auth login` for the host, or set `GH_REPO=HOST/OWNER/REPO`. |
