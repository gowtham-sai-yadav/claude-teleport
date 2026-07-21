#!/bin/sh
# claude-teleport installer.
#
#   curl -fsSL https://raw.githubusercontent.com/gowtham-sai-yadav/claude-teleport/main/install.sh | sh
#
# Downloads the right prebuilt binary for your machine from the latest GitHub
# release, verifies its SHA-256 checksum, and installs it onto your PATH.
#
# Overridable with environment variables:
#   VERSION=v0.3.0      install a specific release (default: latest)
#   INSTALL_DIR=~/bin   install location (default: /usr/local/bin, or ~/.local/bin without sudo)
set -eu

REPO="gowtham-sai-yadav/claude-teleport"
BINARY="claude-teleport"
VERSION="${VERSION:-latest}"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "this installer needs '$1' but it was not found"; }
need uname
need mktemp
# One of curl/wget is required.
if command -v curl >/dev/null 2>&1; then
	dl() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	dl() { wget -qO "$2" "$1"; }
else
	die "this installer needs 'curl' or 'wget'"
fi

# Detect OS.
os=$(uname -s)
case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS '$os'. On Windows, use install.ps1 or 'go install $REPO@latest'." ;;
esac

# Detect architecture.
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported architecture '$arch'" ;;
esac

asset="${BINARY}-${os}-${arch}"
if [ "$VERSION" = "latest" ]; then
	base="https://github.com/${REPO}/releases/latest/download"
else
	base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "Downloading ${asset} (${VERSION})..."
dl "${base}/${asset}" "${tmp}/${BINARY}" || die "download failed for ${base}/${asset}"

# Verify the checksum if a checksum tool and the sums file are available.
if command -v sha256sum >/dev/null 2>&1; then
	sha() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	sha() { echo ""; }
fi
if [ -n "$(sha "${tmp}/${BINARY}")" ] && dl "${base}/SHA256SUMS.txt" "${tmp}/SHA256SUMS.txt" 2>/dev/null; then
	want=$(grep " ${asset}\$" "${tmp}/SHA256SUMS.txt" 2>/dev/null | awk '{print $1}' | head -1)
	got=$(sha "${tmp}/${BINARY}")
	if [ -n "$want" ] && [ "$want" != "$got" ]; then
		die "checksum mismatch for ${asset} (expected ${want}, got ${got})"
	fi
	[ -n "$want" ] && say "Checksum verified."
fi

chmod +x "${tmp}/${BINARY}"

# Choose an install directory: an explicit one, else /usr/local/bin (with sudo
# if needed), else ~/.local/bin without touching root.
dest="${INSTALL_DIR:-}"
use_sudo=""
if [ -z "$dest" ]; then
	if [ -w /usr/local/bin ] 2>/dev/null; then
		dest=/usr/local/bin
	elif command -v sudo >/dev/null 2>&1; then
		dest=/usr/local/bin
		use_sudo=sudo
	else
		dest="${HOME}/.local/bin"
	fi
fi
mkdir -p "$dest" 2>/dev/null || ${use_sudo} mkdir -p "$dest"

if [ -n "$use_sudo" ]; then
	say "Installing to ${dest} (needs sudo)..."
	$use_sudo mv "${tmp}/${BINARY}" "${dest}/${BINARY}"
else
	mv "${tmp}/${BINARY}" "${dest}/${BINARY}"
fi

say "Installed ${BINARY} to ${dest}/${BINARY}"

# Confirm it runs, and warn if the location is not on PATH.
if command -v "$BINARY" >/dev/null 2>&1 && [ "$(command -v "$BINARY")" = "${dest}/${BINARY}" ]; then
	"$BINARY" version 2>/dev/null || true
else
	case ":${PATH}:" in
		*":${dest}:"*) "${dest}/${BINARY}" version 2>/dev/null || true ;;
		*)
			say ""
			say "Note: ${dest} is not on your PATH. Add it, for example:"
			say "  echo 'export PATH=\"${dest}:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
			;;
	esac
fi

say "Done. Run '${BINARY}' to get started."
