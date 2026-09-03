#!/usr/bin/env bash
# Install discord-cli from the GitHub release binaries.
#
#   curl -fsSL https://raw.githubusercontent.com/arnoldadlv/discord-cli/main/install.sh | bash
#
# Environment:
#   DISCORD_VERSION       release tag to install (default: the latest release)
#   DISCORD_INSTALL_DIR   where to put the binary (default: ~/.local/bin)
#
# Re-running the script upgrades to the latest release. Every binary is
# verified against the release's checksums.txt before it is installed.
set -euo pipefail

REPO="arnoldadlv/discord-cli"
BASE="${DISCORD_RELEASE_BASE:-https://github.com/${REPO}/releases}"
VERSION="${DISCORD_VERSION:-}"
INSTALL_DIR="${DISCORD_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

# asset_name prints the release asset for this machine, or fails.
asset_name() {
  local os arch
  os="${DISCORD_INSTALL_OS:-$(uname -s)}"
  arch="${DISCORD_INSTALL_ARCH:-$(uname -m)}"
  case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported operating system: $os. Windows binaries are on the releases page: https://github.com/${REPO}/releases" ;;
  esac
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) fail "unsupported architecture: $arch" ;;
  esac
  printf 'discord-%s-%s\n' "$os" "$arch"
}

if [ "${1:-}" = "--print-asset" ]; then
  asset_name
  exit 0
fi

asset="$(asset_name)"
if [ -n "$VERSION" ]; then
  url_base="${BASE}/download/${VERSION}"
else
  url_base="${BASE}/latest/download"
fi

command -v curl >/dev/null 2>&1 || fail "curl is required"
if command -v sha256sum >/dev/null 2>&1; then
  sum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  sum_cmd="shasum -a 256"
else
  fail "sha256sum or shasum is required to verify the download"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "Downloading ${asset} from ${url_base}"
curl -fsSL "${url_base}/${asset}" -o "${tmp}/${asset}" || fail "download failed: ${url_base}/${asset} (is DISCORD_VERSION a real release tag?)"
curl -fsSL "${url_base}/checksums.txt" -o "${tmp}/checksums.txt" || fail "download failed: ${url_base}/checksums.txt"

expected="$(awk -v a="$asset" '$2 == a { print $1 }' "${tmp}/checksums.txt")"
[ -n "$expected" ] || fail "checksums.txt has no entry for ${asset}"
actual="$($sum_cmd "${tmp}/${asset}" | awk '{ print $1 }')"
[ "$expected" = "$actual" ] || fail "checksum mismatch for ${asset}: expected ${expected}, got ${actual}. Nothing was installed."

mkdir -p "$INSTALL_DIR"
chmod 0755 "${tmp}/${asset}"
mv -f "${tmp}/${asset}" "${INSTALL_DIR}/discord"

say "Installed $("${INSTALL_DIR}/discord" --version 2>/dev/null | head -n 1 || printf 'discord') to ${INSTALL_DIR}/discord"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    say ""
    say "${INSTALL_DIR} is not on your PATH. Add this line to ~/.zshrc or ~/.bashrc, then open a new terminal:"
    say "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac
say ""
say "Next: discord auth set, then discord config set default-guild \"<your guild>\"."
