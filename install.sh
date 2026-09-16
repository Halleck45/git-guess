#!/bin/sh
# Installs the latest git-guess release into /usr/local/bin (or $BIN_DIR).
#   curl -fsSL https://raw.githubusercontent.com/Halleck45/git-guess/main/install.sh | sh
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
version="${VERSION:-$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')}"
[ -n "$version" ] || { echo "could not determine the latest version" >&2; exit 1; }
url="https://github.com/$REPO/releases/download/$version/git-guess_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading git-guess $version for $os/$arch"
curl -fsSL "$url" | tar -xz -C "$tmp"
if [ -w "$BIN_DIR" ]; then
  install -m 755 "$tmp/git-guess" "$BIN_DIR/git-guess"
else
  echo "installing to $BIN_DIR needs sudo"
  sudo install -m 755 "$tmp/git-guess" "$BIN_DIR/git-guess"
fi
echo "installed $BIN_DIR/git-guess"
echo "try it: cd into a repository and run  git guess eval"
