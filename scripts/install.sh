#!/bin/sh
set -e

BINARY="tld"
INSTALL_DIR="/usr/local/bin"

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

echo "Installing to $INSTALL_DIR (may require sudo)..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
    sudo mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

# Cleanup and Verify
rm -rf "$TMP_DIR"
chmod +x "$INSTALL_DIR/$BINARY"

echo "--------------------------------------------------"
echo "Successfully installed! Run 'tld' to get started."
