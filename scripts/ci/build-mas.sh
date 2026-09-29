#!/usr/bin/env bash
#
# Build and package the tlDiagram macOS desktop app for the Apple Mac App Store.
#
# Produces a signed installer package ready to upload to App Store Connect:
#   cmd/tld-app/build/release/tlDiagram-<version>-mas-<build>.pkg
#
# Requirements:
#   - macOS with Xcode command line tools (codesign, productbuild, PlistBuddy)
#   - wails CLI, Go, Node/npm
#   - "3rd Party Mac Developer Application" and
#     "3rd Party Mac Developer Installer" certificates in the login keychain
#   - cmd/tld-app/build/darwin/embedded.provisionprofile for com.mertcikla.tldiagram
#
# Usage:
#   scripts/ci/build-mas.sh [version]
#
# Environment overrides:
#   MAS_BUILD_NUMBER  CFBundleVersion / build number (default: current timestamp)
#   MAS_APP_IDENTITY  Application signing identity (default: auto-detected)
#   MAS_INSTALLER_IDENTITY  Installer signing identity (default: auto-detected)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

APP_ID="com.mertcikla.tldiagram"
APP_CATEGORY="public.app-category.developer-tools"
BUNDLE_NAME="tld"
DISPLAY_NAME="tlDiagram"
APP_DIR="cmd/tld-app"
BUILD_DIR="$APP_DIR/build"
RELEASE_DIR="$BUILD_DIR/release"
APP_BUNDLE="$BUILD_DIR/bin/$BUNDLE_NAME.app"
ENTITLEMENTS_TEMPLATE="$APP_DIR/packaging/darwin/entitlements.mas.plist"
ENTITLEMENTS_OUT="$BUILD_DIR/darwin/entitlements.mas.signed.plist"
PROVISION_PROFILE="$BUILD_DIR/darwin/embedded.provisionprofile"
WAILS_JSON="$APP_DIR/wails.json"
PB="/usr/libexec/PlistBuddy"

if [[ ! -f "$PROVISION_PROFILE" ]]; then
	echo "missing provisioning profile: $PROVISION_PROFILE" >&2
	exit 1
fi

VERSION="${1:-$(sed -n 's/^var Version = "\(.*\)"/\1/p' cmd/version/version.go)}"
if [[ -z "$VERSION" ]]; then
	echo "could not determine version; pass one as the first argument" >&2
	exit 1
fi

BUILD_NUMBER="${MAS_BUILD_NUMBER:-$(date +%Y%m%d%H%M%S)}"

TEAM_ID="$(security cms -D -i "$PROVISION_PROFILE" 2>/dev/null |
	plutil -extract ApplicationIdentifierPrefix.0 raw -o - - 2>/dev/null)"
if [[ -z "$TEAM_ID" ]]; then
	echo "could not determine Team ID from $PROVISION_PROFILE" >&2
	exit 1
fi

APP_IDENTITY="${MAS_APP_IDENTITY:-$(security find-identity -v -p basic |
	grep "3rd Party Mac Developer Application:" | head -n 1 | sed -E 's/.*"(.*)"/\1/')}"
INSTALLER_IDENTITY="${MAS_INSTALLER_IDENTITY:-$(security find-identity -v -p basic |
	grep "3rd Party Mac Developer Installer:" | head -n 1 | sed -E 's/.*"(.*)"/\1/')}"
if [[ -z "$APP_IDENTITY" ]]; then
	echo "no '3rd Party Mac Developer Application' identity found" >&2
	exit 1
fi
if [[ -z "$INSTALLER_IDENTITY" ]]; then
	echo "no '3rd Party Mac Developer Installer' identity found" >&2
	exit 1
fi

echo "Building $DISPLAY_NAME $VERSION (build $BUILD_NUMBER)"
echo "  team:      $TEAM_ID"
echo "  app:       $APP_IDENTITY"
echo "  installer: $INSTALLER_IDENTITY"

WAILS_JSON_BACKUP="$(mktemp)"
cp "$WAILS_JSON" "$WAILS_JSON_BACKUP"
restore_wails_json() {
	cp "$WAILS_JSON_BACKUP" "$WAILS_JSON"
	rm -f "$WAILS_JSON_BACKUP"
}
trap restore_wails_json EXIT

echo "==> Installing frontend dependencies"
npm --prefix frontend ci

echo "==> Building frontend assets"
npm --prefix frontend run build:app

echo "==> Preparing app icon"
mkdir -p "$BUILD_DIR"
cp frontend/logo/macos/Icon-iOS-Default-1024x1024@1x.png "$BUILD_DIR/appicon.png"
rm -f "$BUILD_DIR/windows/icon.ico"

echo "==> Setting wails.json productVersion to $VERSION"
jq --arg ver "$VERSION" '.info.productVersion = $ver' "$WAILS_JSON" > "$WAILS_JSON.tmp"
mv "$WAILS_JSON.tmp" "$WAILS_JSON"

echo "==> Building universal Wails app (appstore tag)"
LDFLAGS="-s -w -X github.com/mertcikla/tld/v2/cmd/version.Version=$VERSION"
(
	cd "$APP_DIR"
	wails build -clean -platform darwin/universal -tags appstore -s -ldflags "$LDFLAGS"
)

if [[ ! -d "$APP_BUNDLE" ]]; then
	echo "expected app bundle not found: $APP_BUNDLE" >&2
	exit 1
fi

INFO_PLIST="$APP_BUNDLE/Contents/Info.plist"

echo "==> Updating app Info.plist"
$PB -c "Set :CFBundleIdentifier $APP_ID" "$INFO_PLIST"
$PB -c "Set :CFBundleVersion $BUILD_NUMBER" "$INFO_PLIST"
$PB -c "Set :CFBundleShortVersionString $VERSION" "$INFO_PLIST"
$PB -c "Add :LSApplicationCategoryType string $APP_CATEGORY" "$INFO_PLIST" 2>/dev/null ||
	$PB -c "Set :LSApplicationCategoryType $APP_CATEGORY" "$INFO_PLIST"
$PB -c "Add :NSAppTransportSecurity dict" "$INFO_PLIST" 2>/dev/null || true
$PB -c "Add :NSAppTransportSecurity:NSAllowsLocalNetworking bool true" "$INFO_PLIST" 2>/dev/null || true

echo "==> Embedding provisioning profile"
# Downloaded provisioning profiles carry com.apple.quarantine (and other
# provenance metadata). App Store Connect rejects any package that contains
# those attributes (error 91109), so strip them from the source before copying.
xattr -c "$PROVISION_PROFILE" 2>/dev/null || true
cp "$PROVISION_PROFILE" "$APP_BUNDLE/Contents/embedded.provisionprofile"

echo "==> Removing extended attributes from app bundle"
xattr -cr "$APP_BUNDLE"

if xattr -lr "$APP_BUNDLE" 2>/dev/null | grep -q "com.apple.quarantine"; then
	echo "quarantine attribute still present in $APP_BUNDLE" >&2
	exit 1
fi

echo "==> Generating entitlements"
mkdir -p "$(dirname "$ENTITLEMENTS_OUT")"
sed "s/__TEAM_ID__/$TEAM_ID/g" "$ENTITLEMENTS_TEMPLATE" > "$ENTITLEMENTS_OUT"
plutil -lint "$ENTITLEMENTS_OUT"

echo "==> Signing app bundle"
codesign --force --timestamp --sign "$APP_IDENTITY" \
	--entitlements "$ENTITLEMENTS_OUT" "$APP_BUNDLE"
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"

echo "==> Building installer package"
mkdir -p "$RELEASE_DIR"
PKG="$RELEASE_DIR/${DISPLAY_NAME}-${VERSION}-mas-${BUILD_NUMBER}.pkg"
rm -f "$PKG"
productbuild --component "$APP_BUNDLE" /Applications \
	--sign "$INSTALLER_IDENTITY" "$PKG"

echo "==> Verifying package signature"
pkgutil --check-signature "$PKG"

echo "==> Verifying package payload has no quarantine attributes"
VERIFY_DIR="$(mktemp -d)"
pkgutil --expand-full "$PKG" "$VERIFY_DIR/expanded" >/dev/null 2>&1 || true
if xattr -lr "$VERIFY_DIR/expanded" 2>/dev/null | grep -q "com.apple.quarantine"; then
	xattr -lr "$VERIFY_DIR/expanded" 2>/dev/null | grep "com.apple.quarantine" >&2
	rm -rf "$VERIFY_DIR"
	echo "quarantine attribute present in package payload; App Store Connect would reject it (91109)" >&2
	exit 1
fi
rm -rf "$VERIFY_DIR"

echo
echo "Created $PKG"
