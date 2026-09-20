#!/usr/bin/env sh
set -eu

REPO="scottdsnr/ssh-manager"
BIN="ssh-manager"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"

fail() { echo "error: $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) fail "unsupported OS: $os" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ "$VERSION" = latest ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || fail "could not determine latest release"
fi

asset="$BIN-$os-$arch"
url="https://github.com/$REPO/releases/download/$VERSION/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $BIN $VERSION ($os/$arch)..."
curl -fsSL "$url" -o "$tmp/$BIN" || fail "download failed: $url"

if curl -fsSL "https://github.com/$REPO/releases/download/$VERSION/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
  expected=$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual=$(sha256sum "$tmp/$BIN" | awk '{print $1}')
    else
      actual=$(shasum -a 256 "$tmp/$BIN" | awk '{print $1}')
    fi
    [ "$actual" = "$expected" ] || fail "checksum mismatch"
    echo "Checksum verified."
  fi
fi

chmod +x "$tmp/$BIN"
mkdir -p "$INSTALL_DIR"
mv "$tmp/$BIN" "$INSTALL_DIR/$BIN"

echo "Installed $INSTALL_DIR/$BIN"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: $INSTALL_DIR is not on your PATH. Add it:"
     echo "  export PATH=\"\$PATH:$INSTALL_DIR\"" ;;
esac
