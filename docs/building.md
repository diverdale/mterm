# Building & distribution

The repo ships three scripts that produce installable artifacts in `dist/`:

| Script         | Output                                              |
|----------------|-----------------------------------------------------|
| `build.sh`     | Cross-compiled binaries (darwin arm64/amd64, linux arm64/amd64) + `SHA256SUMS`; optionally signed + notarized .zips for macOS |
| `installer.sh` | Universal macOS `.pkg` (signed, notarized, stapled) that installs `mterm` to `/usr/local/bin/` |
| `debian.sh`    | `.deb` packages for amd64 + arm64; installs `mterm` to `/usr/bin/` |

Windows is excluded because mterm uses `SIGUSR1` for the in-app goroutine
dump.

## Cross-compile (`build.sh`)

```bash
./build.sh                 # all default platforms
./build.sh darwin/arm64    # build just one
```

Without signing env vars set, this just produces stripped binaries +
`SHA256SUMS`. macOS users unzipping unsigned binaries will get a
Gatekeeper warning that requires `xattr -d com.apple.quarantine` to clear.

## Codesign + notarize (macOS)

For Gatekeeper-clean macOS distribution you need a Developer ID Application
certificate. With one in the keychain, `build.sh` automatically signs
and notarizes the darwin .zips.

One-time keychain setup:

```bash
# Generate an app-specific password at
# https://appleid.apple.com → Sign-In and Security → App-Specific Passwords.
xcrun notarytool store-credentials mterm-notary \
    --apple-id <your-apple-id> \
    --team-id <10-char-team-id> \
    --password <app-specific-password>
```

Build + sign + notarize:

```bash
export APPLE_SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export APPLE_KEYCHAIN_PROFILE="mterm-notary"
./build.sh
```

Output goes to `dist/mterm-darwin-<arch>.zip` — that's the file to hand
out. First launch on a colleague's machine does an online notarization
check (~1s) then runs cleanly with no Gatekeeper prompt.

To inspect available identities:

```bash
security find-identity -v -p codesigning
```

## macOS `.pkg` installer

For a one-click install experience, `installer.sh` fuses the two darwin
binaries into a universal binary, wraps it in a signed + notarized `.pkg`
that drops `mterm` into `/usr/local/bin/`, and staples the notarization
ticket. Colleagues double-click the `.pkg`, enter their password, then
run `mterm` from any terminal.

Requires a Developer ID **Installer** certificate (different from the
Application cert — Apple issues them separately):

```bash
# See your installer identities:
security find-identity -v -p basic | grep "Developer ID Installer"

# Then build + sign + notarize the installer:
export APPLE_SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export APPLE_INSTALLER_SIGNING_IDENTITY="Developer ID Installer: Your Name (TEAMID)"
export APPLE_KEYCHAIN_PROFILE="mterm-notary"
./build.sh && ./installer.sh
```

Output: `dist/mterm-installer.pkg` (~12 MiB; universal binary inside).

Override defaults via:

- `PKG_IDENTIFIER` (default `dev.mterm`) — reverse-DNS bundle id
- `PKG_VERSION`  (default derived from `git describe`) — installer version

Uninstall (for colleagues if asked):

```bash
sudo rm /usr/local/bin/mterm
pkgutil --forget dev.mterm
```

## Debian / Ubuntu / Mint `.deb`

`debian.sh` wraps the cross-compiled Linux binaries into installable
`.deb` packages — one per architecture. Colleagues install with `apt` or
`dpkg`; mterm lands at `/usr/bin/mterm`.

```bash
# macOS:  brew install dpkg
# Linux:  already built in
./build.sh && ./debian.sh
```

Output: `dist/mterm_<version>_amd64.deb` and `dist/mterm_<version>_arm64.deb`.

The version string defaults to `git describe`; if there are no tags yet,
the SHA is prefixed with `0.0.0+` so Debian's version parser accepts it.
Override via env vars when you ship:

```bash
DEB_VERSION=1.0.0 \
DEB_MAINTAINER="Your Name <you@example.com>" \
    ./debian.sh
```

Colleagues install:

```bash
sudo apt install ./mterm_<version>_amd64.deb   # or _arm64
# or, equivalently with plain dpkg (no auto-deps; mterm has none):
sudo dpkg -i mterm_<version>_amd64.deb
```

Uninstall:

```bash
sudo apt remove mterm    # or: sudo dpkg -r mterm
```

The `.deb` is not signed — for internal distribution this is fine, and
the `SHA256SUMS` file in `dist/` covers integrity verification. Signing
requires a Debian package-signing key and `dpkg-sig`; deferred until
there's a reason to.
