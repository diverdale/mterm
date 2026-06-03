#!/usr/bin/env bash
# Build .deb installers for Debian / Ubuntu / Linux Mint / etc.
# Drops the mterm binary into /usr/bin and the README into
# /usr/share/doc/mterm. Two outputs:
#
#   dist/mterm_<version>_amd64.deb
#   dist/mterm_<version>_arm64.deb
#
# Prereq:
#   ./build.sh has produced dist/mterm-linux-amd64 and
#   dist/mterm-linux-arm64.
#
#   dpkg-deb is available:
#     macOS:  brew install dpkg
#     Linux:  already built in
#
# Environment overrides:
#   DEB_VERSION       package version (default: derived from git describe,
#                                       falling back to 0.1.0)
#   DEB_MAINTAINER    "Name <email>" string for the control file
#                       (default: from git config user.name + user.email,
#                                  falling back to a placeholder)

set -euo pipefail

if ! command -v dpkg-deb >/dev/null 2>&1; then
    cat >&2 <<EOF
dpkg-deb not found. Install it first:

  macOS:   brew install dpkg
  Linux:   already built in (apt install dpkg if somehow missing)

EOF
    exit 1
fi

# Version: derive from git unless explicitly set. Strip a leading "v"
# (common for git tags like v1.2.3) since debian version policy expects
# pure digits-and-dots at the start. If the derived value still doesn't
# start with a digit (e.g. there are no git tags, so describe returns a
# raw commit SHA), prepend "0.0.0+" so debian's parser accepts it while
# the SHA stays visible for traceability.
VERSION="${DEB_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"
VERSION="${VERSION#v}"
if [[ ! "$VERSION" =~ ^[0-9] ]]; then
    VERSION="0.0.0+${VERSION}"
fi

# Maintainer: prefer git config, fall back to a clearly-placeholder string
# so the user notices and overrides via DEB_MAINTAINER if they ship widely.
if [[ -z "${DEB_MAINTAINER:-}" ]]; then
    git_name="$(git config --get user.name 2>/dev/null || true)"
    git_email="$(git config --get user.email 2>/dev/null || true)"
    if [[ -n "$git_name" && -n "$git_email" ]]; then
        MAINTAINER="$git_name <$git_email>"
    else
        MAINTAINER="Unknown <unknown@example.invalid>"
    fi
else
    MAINTAINER="$DEB_MAINTAINER"
fi

build_deb() {
    local arch="$1"     # mterm's arch suffix (amd64 / arm64)
    local debArch="$2"  # debian's arch label (amd64 / arm64)
    local binary="dist/mterm-linux-${arch}"

    if [[ ! -f "$binary" ]]; then
        echo "missing $binary — run ./build.sh first" >&2
        return 1
    fi

    local stage="dist/deb-stage-${arch}"
    rm -rf "$stage"
    mkdir -p "$stage/DEBIAN" "$stage/usr/bin" "$stage/usr/share/doc/mterm"

    cp "$binary" "$stage/usr/bin/mterm"
    chmod 755 "$stage/usr/bin/mterm"

    if [[ -f README.md ]]; then
        cp README.md "$stage/usr/share/doc/mterm/README.md"
        chmod 644 "$stage/usr/share/doc/mterm/README.md"
    fi

    cat > "$stage/DEBIAN/control" <<EOF
Package: mterm
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${debArch}
Maintainer: ${MAINTAINER}
Homepage: https://github.com/diverdale/mterm
Description: Multi-connection SSH TUI
 mterm is a Go-powered multi-connection SSH TUI with tabbed SSH
 sessions, a fuzzy host picker, full-fidelity VT terminal, and
 tmux-style prefix navigation. Reads ~/.ssh/config and
 ~/.config/mterm/hosts.yaml (host overlay).
EOF

    local output="dist/mterm_${VERSION}_${debArch}.deb"
    printf "building %-36s " "$output"
    # --root-owner-group ensures the package contents are owned by
    # root:root regardless of the build user, which is what dpkg
    # expects for /usr installs.
    dpkg-deb --build --root-owner-group "$stage" "$output" >/dev/null
    printf "%s\n" "$(du -h "$output" | awk '{print $1}')"
    rm -rf "$stage"
}

build_deb amd64 amd64
build_deb arm64 arm64

# Refresh SHA256SUMS so the .deb files are covered.
(cd dist && shasum -a 256 mterm-* *.zip *.pkg *.deb 2>/dev/null | tee SHA256SUMS >/dev/null)

cat <<NOTE

Distribute:
  Hand colleagues the .deb for their architecture:
    sudo apt install ./mterm_${VERSION}_amd64.deb     # or _arm64
  Or with plain dpkg (no auto-deps, fine here since mterm has none):
    sudo dpkg -i mterm_${VERSION}_amd64.deb
  mterm lands at /usr/bin/mterm; first run creates ~/.config/mterm/.

Uninstall:
  sudo apt remove mterm        # or
  sudo dpkg -r mterm
NOTE
