#!/bin/sh
# publish.sh — contribute THIS machine's binary to a release.
#
# Run independently on each platform, in any order, whenever the machine
# is online. The machines never need to reach each other: the first run
# creates the GitHub release, later runs download the existing assets
# from GitHub, add their own binary, and regenerate SHA256SUMS.
#
#   scripts/publish.sh v1.4.0 "release notes"
#
# Requires: go, gh (authenticated). Optional: PRERELEASE=1 for a
# --prerelease (ignored by 'power-monitor upgrade' and /releases/latest).
set -e

# Must stay in sync with repoSlug in upgrade.go
REPO="DhilipBinny/pd-power-monitor"

VERSION="$1"
if [ -z "$VERSION" ]; then
    echo "usage: scripts/publish.sh vX.Y.Z [release notes]"
    exit 1
fi
case "$VERSION" in
    v*) ;;
    *) echo "error: version must look like vX.Y.Z (with the v)"; exit 1 ;;
esac
NOTES="${2:-power-monitor $VERSION}"

# Locate go even in shells without the usual PATH additions
GO=$(command -v go || true)
[ -z "$GO" ] && [ -x /usr/local/go/bin/go ] && GO=/usr/local/go/bin/go
[ -z "$GO" ] && [ -x /opt/homebrew/bin/go ] && GO=/opt/homebrew/bin/go
[ -z "$GO" ] && { echo "error: go not found"; exit 1; }

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) echo "error: unsupported arch $ARCH"; exit 1 ;;
esac
ASSET="power-monitor-$OS-$ARCH"

sha256() {
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$@"
    else
        sha256sum "$@"
    fi
}

REPO_DIR=$(cd "$(dirname "$0")/.." && pwd)
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

echo "== vet + build $ASSET $VERSION"
cd "$REPO_DIR"
"$GO" vet .
"$GO" build -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/$ASSET" .

BUILT=$("$STAGE/$ASSET" version | awk '{print $2}')
if [ "$BUILT" != "$VERSION" ]; then
    echo "error: built binary reports '$BUILT', expected '$VERSION'"
    exit 1
fi

# The Linux tray is pure Go (D-Bus), so Linux binaries cross-compile from
# anywhere. Only macOS requires a native (cgo/Cocoa) build.
for TARGET in linux/amd64 linux/arm64; do
    TOS=${TARGET%/*}
    TARCH=${TARGET#*/}
    TASSET="power-monitor-$TOS-$TARCH"
    [ "$TASSET" = "$ASSET" ] && continue
    echo "== cross-building $TASSET (pure Go)"
    CGO_ENABLED=0 GOOS="$TOS" GOARCH="$TARCH" \
        "$GO" build -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/$TASSET" .
done

cd "$STAGE"
if gh release view "$VERSION" -R "$REPO" >/dev/null 2>&1; then
    echo "== release $VERSION exists; merging this binary into it"
    # Fetch the other platforms' binaries so SHA256SUMS covers everything
    mkdir existing
    (cd existing && gh release download "$VERSION" -R "$REPO" -p 'power-monitor-*' 2>/dev/null) || true
    for f in power-monitor-*; do
        rm -f "existing/$f"   # what we just built is fresher
    done
    mv existing/* . 2>/dev/null || true
    rmdir existing

    sha256 power-monitor-* > SHA256SUMS
    gh release upload "$VERSION" power-monitor-* SHA256SUMS -R "$REPO" --clobber
else
    echo "== creating release $VERSION"
    sha256 power-monitor-* > SHA256SUMS
    EXTRA=""
    [ -n "$PRERELEASE" ] && EXTRA="--prerelease"
    gh release create "$VERSION" power-monitor-* SHA256SUMS -R "$REPO" \
        --title "$VERSION" --notes "$NOTES" $EXTRA
fi

echo "== assets now in $VERSION:"
gh release view "$VERSION" -R "$REPO" --json assets -q '.assets[].name'
echo ""
echo "Each platform present in SHA256SUMS can now 'power-monitor upgrade'."
echo "Run this same script on the other machine (any time) to complete the release."
