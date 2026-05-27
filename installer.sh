#!/usr/bin/env bash
# Build a signed + notarized macOS .pkg installer that drops mterm into
# /usr/local/bin so colleagues can just run `mterm` from any terminal.
#
# Prerequisite: ./build.sh has already produced dist/mterm-darwin-arm64
# and dist/mterm-darwin-amd64. This script fuses them into a universal
# binary, wraps it in a .pkg, signs with your Developer ID Installer
# cert, notarizes via notarytool, and staples the ticket.
#
# Required environment:
#
#   APPLE_INSTALLER_SIGNING_IDENTITY
#     Your Developer ID INSTALLER certificate (not the Application one
#     you used for the binary). Apple issues these separately. Run
#       security find-identity -v -p basic | grep "Developer ID Installer"
#     to see what's available.
#
#   APPLE_KEYCHAIN_PROFILE
#     notarytool keychain profile name — same one build.sh uses. Set up via:
#       xcrun notarytool store-credentials <profile> ...
#
# Optional environment:
#
#   PKG_IDENTIFIER   reverse-DNS bundle id (default: dev.mterm)
#   PKG_VERSION      installer version string (default: derived from git)

set -euo pipefail

: "${APPLE_INSTALLER_SIGNING_IDENTITY:?set this to your Developer ID Installer identity}"
: "${APPLE_KEYCHAIN_PROFILE:?set this to your notarytool keychain profile name}"

PKG_IDENTIFIER="${PKG_IDENTIFIER:-dev.mterm}"
PKG_VERSION="${PKG_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"

ARM_BINARY="dist/mterm-darwin-arm64"
AMD_BINARY="dist/mterm-darwin-amd64"

if [[ ! -f "$ARM_BINARY" || ! -f "$AMD_BINARY" ]]; then
    echo "missing darwin binaries — run ./build.sh first" >&2
    exit 1
fi

STAGE="dist/installer-stage"
rm -rf "$STAGE"
mkdir -p "$STAGE/usr/local/bin"
mkdir -p "$STAGE/usr/local/share/doc/mterm"

# Fuse arm64 + amd64 into one universal binary. ~12 MiB vs ~6 MiB per
# arch, but a single .pkg works on every Mac.
echo "fusing universal binary..."
lipo -create -output "$STAGE/usr/local/bin/mterm" "$ARM_BINARY" "$AMD_BINARY"
chmod 755 "$STAGE/usr/local/bin/mterm"

# Bundle the README so colleagues can read it offline at any point with
# `open /usr/local/share/doc/mterm/README.md` or via their editor.
if [[ -f README.md ]]; then
    cp README.md "$STAGE/usr/local/share/doc/mterm/README.md"
    chmod 644 "$STAGE/usr/local/share/doc/mterm/README.md"
fi

# Re-sign the universal binary with the Application cert if one is set;
# pkgbuild preserves whatever signature is already on it. Without this,
# /usr/local/bin/mterm will be unsigned after install — works but loses
# the hardened-runtime guarantees.
if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
    echo "codesigning universal binary..."
    codesign --force --sign "$APPLE_SIGNING_IDENTITY" \
        --options=runtime --timestamp "$STAGE/usr/local/bin/mterm"
fi

UNSIGNED_PKG="dist/mterm-installer-unsigned.pkg"
SIGNED_PKG="dist/mterm-installer.pkg"

echo "building component pkg..."
pkgbuild --quiet \
    --root "$STAGE" \
    --identifier "$PKG_IDENTIFIER" \
    --version "$PKG_VERSION" \
    --install-location "/" \
    "$UNSIGNED_PKG"

echo "signing pkg..."
productsign --sign "$APPLE_INSTALLER_SIGNING_IDENTITY" \
    "$UNSIGNED_PKG" "$SIGNED_PKG"
rm -f "$UNSIGNED_PKG"

echo "notarizing pkg (this takes ~30s)..."
xcrun notarytool submit "$SIGNED_PKG" \
    --keychain-profile "$APPLE_KEYCHAIN_PROFILE" \
    --wait >/dev/null

echo "stapling ticket..."
xcrun stapler staple "$SIGNED_PKG"

# Refresh the SHA256SUMS to include the installer.
(cd dist && shasum -a 256 mterm-* *.zip *.pkg 2>/dev/null | tee SHA256SUMS >/dev/null)

cat <<NOTE

Built: ${SIGNED_PKG}
       version: ${PKG_VERSION}
       bundle:  ${PKG_IDENTIFIER}
       size:    $(du -h "$SIGNED_PKG" | awk '{print $1}')

Distribute:
  Hand colleagues the .pkg. They double-click → installer wizard →
  enter their password → mterm lands in /usr/local/bin. They can run
  'mterm' from any terminal afterwards.

Uninstall (document for colleagues if asked):
  sudo rm /usr/local/bin/mterm
  sudo rm -rf /usr/local/share/doc/mterm
  pkgutil --forget ${PKG_IDENTIFIER}
NOTE
