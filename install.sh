#!/bin/sh
set -eu

repo=tiagokriok/herdr-triggers
bin_dir=${BIN_DIR:-$HOME/.local/bin}
version=${VERSION:-latest}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${BASE_URL:-}" ]; then
  base=$BASE_URL
elif [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/$version"
fi
archive="herdr-triggers_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "checksum mismatch for $archive" >&2
  exit 1
fi

mkdir -p "$tmp/x" "$bin_dir"
tar -xzf "$tmp/$archive" -C "$tmp/x"
install -m 755 "$tmp/x/herdr-triggers" "$bin_dir/herdr-triggers"

echo "installed $("$bin_dir/herdr-triggers" version) to $bin_dir/herdr-triggers"
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "add $bin_dir to your PATH" ;;
esac
echo "next: herdr-triggers service install"
echo "optional: herdr-triggers mod install --keys (app panel), herdr-triggers skill install (agent skill)"
