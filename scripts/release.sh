#!/bin/sh
set -eu

usage() {
    echo 'Usage: scripts/release.sh vMAJOR.MINOR.PATCH [--publish]'
    echo 'Without --publish, only local binaries, archives, and checksums are built.'
}
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then usage >&2; exit 1; fi
version=$1
case "$version" in *[!v0-9.]*) usage >&2; exit 1 ;; esac
publish=${2:-}
if [ -n "$publish" ] && [ "$publish" != --publish ]; then usage >&2; exit 1; fi
if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    usage >&2
    exit 1
fi
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
summary=internal/release/notes/$version-summary.md
if [ ! -s "$summary" ]; then
    echo "Write $summary before releasing $version. What's new shows it after the upgrade." >&2
    exit 1
fi
repo=WendellAdriel/dev-null-tui
if [ "$publish" = --publish ]; then
    command -v gh >/dev/null 2>&1 || { echo 'GitHub CLI (gh) is required to publish.' >&2; exit 1; }
    if [ -n "$(git status --porcelain)" ]; then echo 'Commit all changes before publishing.' >&2; exit 1; fi
    if [ "$(git branch --show-current)" != main ]; then echo 'Publish from main.' >&2; exit 1; fi
    case "$(git remote get-url origin)" in
        "https://github.com/$repo"|"https://github.com/$repo.git"|"git@github.com:$repo.git"|"ssh://git@github.com/$repo.git") ;;
        *) echo "origin must point to $repo." >&2; exit 1 ;;
    esac
    gh auth status
    commit=$(git rev-parse HEAD)
    remote_head=$(git ls-remote --exit-code origin refs/heads/main | awk '{print $1}')
    if [ "$commit" != "$remote_head" ]; then echo 'Push main before publishing.' >&2; exit 1; fi
    if git show-ref --verify --quiet "refs/tags/$version"; then echo 'This tag already exists locally.' >&2; exit 1; fi
    remote_tag=$(git ls-remote origin "refs/tags/$version")
    if [ -n "$remote_tag" ]; then echo 'This tag already exists remotely.' >&2; exit 1; fi
    if gh release view "$version" --repo "$repo" >/dev/null 2>&1; then echo 'This release already exists.' >&2; exit 1; fi
fi

go test ./...
go run ./tools/build -version "$version" -release
if [ "$publish" != --publish ]; then
    printf 'Prepared %s in dist/release. No tag or release was created.\n' "$version"
    exit 0
fi

assets=$(awk '{ print "dist/release/" $2 }' dist/release/checksums.txt)
if [ -z "$assets" ]; then echo 'dist/release/checksums.txt lists no release files.' >&2; exit 1; fi
for asset in $assets; do
    if [ ! -f "$asset" ]; then echo "Missing $asset." >&2; exit 1; fi
done

printf 'Publish %s to %s? Type the version to confirm: ' "$version" "$repo"
read -r confirmation
if [ "$confirmation" != "$version" ]; then echo 'Publication canceled.' >&2; exit 1; fi
if [ "$(git rev-parse HEAD)" != "$commit" ] || [ -n "$(git status --porcelain)" ]; then
    echo 'The checkout changed during preparation. Review it and prepare again.' >&2
    exit 1
fi
git tag -a "$version" -m "Release $version" "$commit"
git push origin "refs/tags/$version"
# Unquoted on purpose: each release file name, which has no spaces, becomes its own argument.
gh release create "$version" --repo "$repo" --verify-tag --draft --title "$version" --generate-notes \
    $assets dist/release/checksums.txt LICENSE
gh release edit "$version" --repo "$repo" --draft=false --latest
