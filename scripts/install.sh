#!/bin/sh
set -e

BINARY="tld"

# Detect OS and Architecture
OS_UNAME=$(uname -s)
ARCH_UNAME=$(uname -m)

case "$OS_UNAME" in
    Darwin) OS="Darwin" ;;
    Linux)  OS="Linux" ;;
    *) echo "Unsupported OS: $OS_UNAME"; exit 1 ;;
esac

case "$ARCH_UNAME" in
    x86_64) ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH_UNAME"; exit 1 ;;
esac

# Determine Installation Directory
if [ -z "$INSTALL_DIR" ]; then
    if [ -w "/usr/local/bin" ]; then
        INSTALL_DIR="/usr/local/bin"
    elif [ -d "$HOME/.local/bin" ] && [ -w "$HOME/.local/bin" ]; then
        INSTALL_DIR="$HOME/.local/bin"
    elif [ -d "$HOME/bin" ] && [ -w "$HOME/bin" ]; then
        INSTALL_DIR="$HOME/bin"
    else
        # Default to ~/.local/bin, will attempt to create it
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi

# Resolve the latest stable release (tags without a prerelease suffix).
# Do not trust /releases/latest alone: a mis-flagged prerelease would hijack fresh installs
# while the in-place updater (which filters semver prereleases) stays on stable.
VERSION=$(curl --retry 3 --connect-timeout 15 -LsSf -H "User-Agent: tld-installer" \
  "https://api.github.com/repos/mertcikla/tld/releases?per_page=100" \
  | grep -o '"tag_name": *"[^"]*"' | sed -E 's/.*"([^"]+)".*/\1/' | grep -v -- "-" | head -n 1)

if [ -z "$VERSION" ]; then
    echo "Could not find latest stable version for mertcikla/tld" >&2
    exit 1
fi

FILENAME="tld_${OS}_${ARCH}.tar.gz"
URL="https://github.com/mertcikla/tld/releases/download/$VERSION/$FILENAME"

echo "Downloading $BINARY $VERSION for $OS/$ARCH..."

# Download and Install
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/tld-install.XXXXXX")
readonly TMP_DIR
cleanup() {
    # Remove only the two files we create. Leave unexpected contents untouched.
    rm -f -- "$TMP_DIR/$FILENAME" "$TMP_DIR/$BINARY"
    rmdir -- "$TMP_DIR" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
curl --retry 3 --connect-timeout 15 -LsSf "$URL" -o "$TMP_DIR/$FILENAME"
# Stream only the executable into our own regular file; do not unpack paths,
# symlinks, or other archive contents into the temporary directory.
tar -xOzf "$TMP_DIR/$FILENAME" "$BINARY" > "$TMP_DIR/$BINARY"

# Validate before touching the existing installation.
chmod +x "$TMP_DIR/$BINARY"
"$TMP_DIR/$BINARY" version

install_binary() {
    mkdir -p "$INSTALL_DIR"
    stage=$(mktemp "$INSTALL_DIR/.tld-install.XXXXXX") || return 1
    if cp "$TMP_DIR/$BINARY" "$stage" && chmod 755 "$stage" && mv -f "$stage" "$INSTALL_DIR/$BINARY"; then
        return 0
    fi
    rm -f -- "$stage"
    return 1
}

if [ -d "$INSTALL_DIR" ] && [ ! -w "$INSTALL_DIR" ]; then
    # Stage on the destination filesystem before replacing a running binary.
    sudo sh -c '
      set -e
      stage=$(mktemp "$2/.tld-install.XXXXXX")
      cleanup_stage() { rm -f -- "$stage"; }
      trap cleanup_stage EXIT
      cp "$1" "$stage"
      chmod 755 "$stage"
      mv -f "$stage" "$2/tld"
    ' sh "$TMP_DIR/$BINARY" "$INSTALL_DIR"
else
    install_binary
fi

echo "Successfully installed! Run '$BINARY --help' to get started."

# Check if INSTALL_DIR is in PATH
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        echo "WARNING: $INSTALL_DIR is not in your PATH."
        echo "You may need to add it to your shell profile (e.g., ~/.bashrc or ~/.zshrc):"
        echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
        ;;
esac

# Execute arguments if provided (e.g., 'serve')
if [ $# -gt 0 ]; then
    echo "--------------------------------------------------"
    echo "Executing: $BINARY $*"
    cleanup
    exec "$INSTALL_DIR/$BINARY" "$@"
fi
