#!/bin/sh
# Installs the latest git-guess release into /usr/local/bin (or $BIN_DIR).
#   curl -fsSL https://raw.githubusercontent.com/Halleck45/git-guess/main/install.sh | sh
# Pin a version with VERSION=v0.2.0.
set -eu
REPO="Halleck45/git-guess"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (on Windows, download git-guess_windows_${arch}.exe from https://github.com/$REPO/releases)" >&2; exit 1 ;;
esac
name="git-guess_${os}_${arch}"
if [ -n "${VERSION:-}" ]; then
  base="https://github.com/$REPO/releases/download/$VERSION"
else
  base="https://github.com/$REPO/releases/latest/download"
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading git-guess ${VERSION:-latest} for $os/$arch"
curl -fsSL -o "$tmp/git-guess" "$base/$name"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
want=$(grep " $name\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/git-guess" | cut -d' ' -f1); else got=$(shasum -a 256 "$tmp/git-guess" | cut -d' ' -f1); fi
[ "$want" = "$got" ] || { echo "checksum mismatch for $name" >&2; exit 1; }
if [ -w "$BIN_DIR" ]; then
  install -m 755 "$tmp/git-guess" "$BIN_DIR/git-guess"
else
  echo "installing to $BIN_DIR needs sudo"
  sudo install -m 755 "$tmp/git-guess" "$BIN_DIR/git-guess"
fi
echo "installed $BIN_DIR/git-guess"
echo "try it: cd into a repository and run  git guess eval"
