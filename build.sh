#!/usr/bin/env bash
# Cross-compile mterm for distribution. Outputs stripped binaries to dist/
# named "mterm-<os>-<arch>". Run from the repo root:
#
#   ./build.sh                    # build all default platforms
#   ./build.sh darwin/arm64       # build just one
#
# Defaults skip Windows because mterm relies on SIGUSR1 (Unix-only).
#
# Optional codesigning + notarization for the darwin binaries. Set these
# environment variables to enable:
#
#   APPLE_SIGNING_IDENTITY   e.g. "Developer ID Application: Jane Doe (ABC123XYZ)"
#                            (run `security find-identity -v -p codesigning`
#                             to see what's installed in your keychain)
#
#   APPLE_KEYCHAIN_PROFILE   name of a notarytool keychain profile, set up once via:
#                              xcrun notarytool store-credentials <profile-name> \
#                                --apple-id <your-apple-id> \
#                                --team-id <10-char-team-id> \
#                                --password <app-specific-password>
#                            (generate an app-specific password at
#                             https://appleid.apple.com → Sign-In and Security →
#                             App-Specific Passwords)
#                            If unset, the script signs but skips notarization.
#
# When both are set the script signs each darwin binary, zips it, submits to
# notarytool, and waits for Apple to clear it. The resulting `dist/mterm-darwin-*.zip`
# files are what you give colleagues — first launch checks the notarization
# ticket online and Gatekeeper stays out of the way.

set -euo pipefail

PLATFORMS_DEFAULT=(
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "linux/arm64"
)

if [[ $# -gt 0 ]]; then
    PLATFORMS=("$@")
else
    PLATFORMS=("${PLATFORMS_DEFAULT[@]}")
fi

# -s strips the symbol table; -w drops DWARF debug info. ~30% size reduction.
LDFLAGS="-s -w"

mkdir -p dist

for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    output="dist/mterm-${os}-${arch}"
    printf "building %-24s " "$output"
    GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="$LDFLAGS" -o "$output" .
    size=$(du -h "$output" | awk '{print $1}')
    printf "%s\n" "$size"
done

# Optional: codesign + notarize the darwin binaries so Gatekeeper accepts
# them on a colleague's machine. Requires a Developer ID Application
# certificate in your keychain and (for notarization) a notarytool
# keychain profile. See header comment for setup.
if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
    for arch in arm64 amd64; do
        binary="dist/mterm-darwin-${arch}"
        [[ -f "$binary" ]] || continue

        printf "codesigning %-24s " "$binary"
        codesign --force --sign "$APPLE_SIGNING_IDENTITY" \
            --options=runtime --timestamp "$binary"
        echo "ok"

        zip_path="${binary}.zip"
        rm -f "$zip_path"
        (cd dist && zip -qj "$(basename "$zip_path")" "$(basename "$binary")")

        if [[ -n "${APPLE_KEYCHAIN_PROFILE:-}" ]]; then
            printf "notarizing  %-24s " "$zip_path"
            xcrun notarytool submit "$zip_path" \
                --keychain-profile "$APPLE_KEYCHAIN_PROFILE" \
                --wait >/dev/null
            echo "ok"
        else
            echo "notarize    skipped ($zip_path — APPLE_KEYCHAIN_PROFILE not set)"
        fi
    done
fi

echo
echo "binaries in dist/:"
ls -lh dist/mterm-* dist/*.zip 2>/dev/null

# Per-file SHA256 so colleagues can verify what they downloaded.
echo
echo "SHA256:"
(cd dist && shasum -a 256 mterm-* *.zip 2>/dev/null | tee SHA256SUMS)

if [[ -n "${APPLE_KEYCHAIN_PROFILE:-}" ]]; then
    cat <<'NOTE'

Distribute:
  - Hand out the .zip files for macOS — colleagues unzip and run.
  - First launch does an online notarization check (~1s) then runs cleanly.
  - First run auto-creates ~/.config/mterm/. Drop a hosts.yaml there;
    see README.md for the schema.
NOTE
else
    cat <<'NOTE'

Distribute:
  - Send the appropriate binary for the colleague's machine.
  - macOS users will hit Gatekeeper on the unsigned binary. Either:
      xattr -d com.apple.quarantine mterm-darwin-arm64
    or right-click → Open → Open Anyway (once per binary).
  - For a smoother experience, set APPLE_SIGNING_IDENTITY and
    APPLE_KEYCHAIN_PROFILE and rerun this script (see header comment).
  - First run auto-creates ~/.config/mterm/. Drop a hosts.yaml there;
    see README.md for the schema.
NOTE
fi
