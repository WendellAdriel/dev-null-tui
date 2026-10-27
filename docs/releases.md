# Preparing and publishing releases

- [Prepare locally](#prepare-locally)
- [Publish a release](#publish-a-release)
- [Recover an interrupted publication](#recover-an-interrupted-publication)
- [Verification limits](#verification-limits)

This guide is for maintainers. User installation and update instructions are in [installation and updates](installation.md).

## Prepare locally

Run the release script with the version you want to build. Use a POSIX shell with the repository's Go version installed. On Windows, use Git Bash or WSL:

```shell
scripts/release.sh v0.1.0
```

The script runs tests and builds six cgo-disabled executables in `dist/release`, plus `checksums.txt`. Asset names match the [manual installation table](installation.md#manual-installation). They are executable files, not archives. The version is embedded through the existing CLI linker variable.

Preparation does not need GitHub credentials, create a tag, or publish anything. You can also build directly on any platform:

```shell
go run ./tools/build -version v0.1.0 -release
```

Run the host executable's `--version` and `--help`, verify the checksums, and finish the [quality checks](development.md#quality-checks) before publishing. For later releases, update `internal/release.Version` in the reviewed source to match the release version.

## Publish a release

Publishing needs Git, an authenticated [GitHub CLI](https://cli.github.com/), and write access to `WendellAdriel/dev-null-tui`. Create and configure the GitHub repository separately. The script does not create it or change its settings.

Commit the reviewed changes and push `main` to `origin`. The script requires a clean checkout on `main`, an `origin` URL for that exact repository, and matching local and remote commits.

> [!WARNING]
> The following command pushes an annotated tag and publishes a public release. Do not run it until the release is approved.

```shell
scripts/release.sh v0.1.0 --publish
```

The script repeats tests and builds, then asks you to type the version to confirm publication. It refuses an existing tag or release. It uploads the six executables, checksums, and MIT license to a draft release, then publishes it as the latest release with generated notes. No GitHub Actions release workflow is needed for cross-compilation.

The installers and `devnull update` use `WendellAdriel/dev-null-tui` as their fixed release source. The Go module and linker variable still use the existing `github.com/WendellAdriel/dev-null-cli` module path.

## Recover an interrupted publication

Publication is not transactional. A failed push can leave a local tag. A later failure can leave a remote tag or draft release. The script does not delete tags, overwrite assets, or retry publication automatically.

Inspect `git show <tag>`, the remote tag, and `gh release view <tag> --repo WendellAdriel/dev-null-tui` before taking further action. Resume an existing draft only with assets built from its tagged commit. Upload any missing assets with `gh release upload`, then publish the complete draft with `gh release edit --draft=false --latest`.

Do not move a published tag or replace its binaries. Correct a published release with a new version.

## Verification limits

Local Linux tests do not establish native macOS or Windows installation or self-update behavior. The native CI test matrix includes installer fixtures for its own platform, but remote results remain unverified until the repository and CI exist.

The tooling generates SHA-256 checksums. It does not provide code signing, macOS notarization, or independent release signatures. See [platform verification](development.md#platform-verification) for the remaining runtime checks.
