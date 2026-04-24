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

# Get the latest release version
VERSION=$(curl -s "https://api.github.com/repos/Mertcikla/tld/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "Could not find latest version for Mertcikla/tld"
    exit 1
fi

# Construct the Download URL
FILENAME="tld_${OS}_${ARCH}.tar.gz"
URL="https://github.com/mertcikla/tld/releases/download/$VERSION/$FILENAME"

echo "Downloading $BINARY $VERSION for $OS/$ARCH..."

# Download and Install
TMP_DIR=$(mktemp -d)
curl -LsSf "$URL" -o "$TMP_DIR/$FILENAME"
tar -xzf "$TMP_DIR/$FILENAME" -C "$TMP_DIR"

if [ ! -d "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR" || true
fi

if [ -w "$INSTALL_DIR" ]; then
    echo "Installing to $INSTALL_DIR..."
    mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
    echo "Installing to $INSTALL_DIR (requires sudo)..."
    sudo mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

# Cleanup and Verify
rm -rf "$TMP_DIR"
if [ -w "$INSTALL_DIR/$BINARY" ]; then
    chmod +x "$INSTALL_DIR/$BINARY"
else
    sudo chmod +x "$INSTALL_DIR/$BINARY"
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
    echo "Executing: $BINARY $@"
    exec "$INSTALL_DIR/$BINARY" "$@"
fi
