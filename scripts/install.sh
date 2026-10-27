#!/bin/sh
set -eu

repo=WendellAdriel/dev-null-tui
install_dir=${INSTALL_DIR:-"$HOME/.local/bin"}
version=${1:-latest}
case "$version" in *[!v0-9.a-z]*) echo 'Invalid version.' >&2; exit 1 ;; esac
if [ "$#" -gt 1 ]; then
    echo 'Usage: install.sh [vMAJOR.MINOR.PATCH]' >&2
    exit 1
fi

case "$(uname -s)" in
    Linux) platform=linux ;;
    Darwin) platform=darwin ;;
    *) echo 'Use install.ps1 for native Windows. This installer supports Linux, macOS, and WSL.' >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo 'Only amd64 and arm64 binaries are available.' >&2; exit 1 ;;
esac

command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
    hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    hash_tool=shasum
else
    echo 'sha256sum or shasum is required.' >&2
    exit 1
fi

if [ "$version" = latest ]; then
    latest=$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 15 --max-time 180 --output /dev/null --write-out '%{url_effective}' \
        "https://github.com/$repo/releases/latest")
    version=${latest##*/}
fi
if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    echo 'Expected a stable version such as v0.1.0.' >&2
    exit 1
fi

asset=devnull_${platform}_${arch}
base=https://github.com/$repo/releases/download/$version
mkdir -p "$install_dir"
if [ -d "$install_dir/devnull" ]; then
    echo 'The installation destination is a directory.' >&2
    exit 1
fi
tmp=$(mktemp -d "$install_dir/.devnull-install.XXXXXX")
cleanup() {
    rm -f "$tmp/binary" "$tmp/checksums.txt"
    rmdir "$tmp"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --max-time 180 --max-filesize 65536 --output "$tmp/checksums.txt" "$base/checksums.txt"
expected=$(awk -v name="$asset" '$2 == name { print $1; count++ } END { if (count != 1) exit 1 }' "$tmp/checksums.txt")
if ! printf '%s\n' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$'; then
    echo 'The release checksum is invalid.' >&2
    exit 1
fi
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --max-time 180 --max-filesize 134217728 --output "$tmp/binary" "$base/$asset"
if [ "$hash_tool" = sha256sum ]; then
    actual=$(sha256sum "$tmp/binary" | awk '{print $1}')
else
    actual=$(shasum -a 256 "$tmp/binary" | awk '{print $1}')
fi
expected=$(printf '%s' "$expected" | tr 'ABCDEF' 'abcdef')
if [ ! -s "$tmp/binary" ] || [ "$actual" != "$expected" ]; then
    echo 'Checksum verification failed. The installed binary was not changed.' >&2
    exit 1
fi
chmod 755 "$tmp/binary"
mv -f "$tmp/binary" "$install_dir/devnull"
printf 'Installed devnull %s to %s/devnull\n' "$version" "$install_dir"
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Add %s to your PATH, then run devnull.\n' "$install_dir" ;;
esac
