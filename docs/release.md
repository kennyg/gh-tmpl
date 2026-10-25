# Release process

A release is a git tag. Pushing a tag that starts with `v` runs the [release workflow](../.github/workflows/release.yml). The workflow builds the binaries with GoReleaser, publishes a GitHub release, and attests the binaries.

## Make a release

1. Check that `main` is green in CI.
2. Run the checks locally:

   ```sh
   mise run check
   mise run release:snapshot
   ```

3. Tag the commit and push the tag:

   ```sh
   git tag -a v1.2.0 -m "v1.2.0"
   git push origin v1.2.0
   ```

4. Watch the workflow with `gh run watch`. When it ends, the release is live.

Use [semantic versioning](https://semver.org). A tag with a suffix, such as `v1.2.0-rc.1`, makes a pre-release. `gh extension install` skips pre-releases unless you ask for one with `--pin`.

## What the workflow does

| Step | Detail |
| --- | --- |
| Install tools | `jdx/mise-action` installs Go and GoReleaser at the versions in `mise.toml`. The release job does not use a cache. |
| Test | `mise run test` |
| Build and publish | `goreleaser release --clean`, with the config in `.goreleaser.yaml` |
| Attest | `actions/attest-build-provenance` signs a SLSA build provenance attestation for each binary listed in the checksums file |

The job has `contents: write` to publish the release, `id-token: write` to get a Sigstore signing certificate, and `attestations: write` to store the attestation. Every action is pinned to a commit SHA.

## Release assets

`gh extension install` picks the asset whose name ends in `<os>-<arch>`, or `<os>-<arch>.exe` on Windows. GoReleaser uploads each binary on its own, without an archive, with that name:

```text
gh-tmpl_v1.2.0_darwin-amd64
gh-tmpl_v1.2.0_darwin-arm64
gh-tmpl_v1.2.0_freebsd-386
gh-tmpl_v1.2.0_freebsd-amd64
gh-tmpl_v1.2.0_freebsd-arm
gh-tmpl_v1.2.0_freebsd-arm64
gh-tmpl_v1.2.0_linux-386
gh-tmpl_v1.2.0_linux-amd64
gh-tmpl_v1.2.0_linux-arm
gh-tmpl_v1.2.0_linux-arm64
gh-tmpl_v1.2.0_windows-386.exe
gh-tmpl_v1.2.0_windows-amd64.exe
gh-tmpl_v1.2.0_windows-arm64.exe
gh-tmpl_v1.2.0_checksums.txt
```

The binaries are static, with `CGO_ENABLED=0`, and built with `-trimpath`. The `-X main.version` linker flag sets the version that `gh tmpl --version` prints. The checksums file holds the SHA-256 hash of each binary.

## Verify a binary

Check the hash against the checksums file:

```sh
gh release download v1.2.0 --repo kennyg/gh-tmpl --pattern 'gh-tmpl_v1.2.0_linux-amd64' --pattern '*checksums.txt'
sha256sum --check --ignore-missing gh-tmpl_v1.2.0_checksums.txt
```

Check that the release workflow of this repository built it:

```sh
gh attestation verify gh-tmpl_v1.2.0_linux-amd64 --repo kennyg/gh-tmpl
```

## Fix a bad release

Do not move or reuse a tag. People who installed the release would get a different binary for the same version. Release a new patch version with the fix instead. You can mark the bad release as a pre-release, or delete it, so that `gh extension install` skips it.
