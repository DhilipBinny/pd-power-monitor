# Releasing

The tray code is cgo (GTK on Linux, Cocoa/IOKit on macOS), so release
binaries must be **built natively on each platform** — cross-compiling is
not possible. A complete release ships:

| Asset | Built on |
|---|---|
| `power-monitor-darwin-arm64` | macOS (Apple Silicon) |
| `power-monitor-linux-amd64`  | Linux (x86-64) |
| `SHA256SUMS`                 | regenerated to list **all** binaries |

`power-monitor upgrade` verifies downloads against `SHA256SUMS`; a binary
missing from that file cannot be installed by the upgrade command.

## Recommended: scripts/publish.sh

On a machine of each target platform, run:

```bash
scripts/publish.sh v1.4.0 "release notes"
```

The script:

1. vets and builds this platform's binary, stamped with the version
2. creates the GitHub release if it doesn't exist — otherwise downloads the
   binaries already uploaded by other platforms
3. regenerates `SHA256SUMS` to cover everything present and uploads

Run it **once per platform, in any order, at any time** — builders never
need to coordinate or be online simultaneously; GitHub is the rendezvous
point. Until every platform has contributed, `power-monitor upgrade` works
for the platforms already present and fails safely ("no checksum for ...")
for the rest.

Requires `go` and an authenticated `gh`. Set `PRERELEASE=1` to create a
pre-release (ignored by `upgrade` and `releases/latest`), useful for
testing the pipeline.

## Manual steps (what the script automates)

```bash
VERSION=v1.4.0

go vet .
go build -ldflags="-s -w -X main.version=$VERSION" -o power-monitor-<os>-<arch> .
./power-monitor-<os>-<arch> version    # must print $VERSION

# First platform — create the release:
shasum -a 256 power-monitor-* > SHA256SUMS
gh release create $VERSION power-monitor-<os>-<arch> SHA256SUMS --title "$VERSION" --notes "..."

# Each further platform — complete it:
gh release download $VERSION -p 'power-monitor-*'      # fetch the existing binaries
shasum -a 256 power-monitor-* > SHA256SUMS             # sums must cover ALL binaries
gh release upload $VERSION power-monitor-<os>-<arch> SHA256SUMS --clobber
```

(`sha256sum` on Linux, `shasum -a 256` on macOS.)

## After publishing

Installed machines update themselves — the installer is only for first-time
setup:

```bash
sudo power-monitor upgrade && power-monitor restart
```

## Checklist

- [ ] `go vet` clean on every target platform (build tags hide each
      platform's files from the others)
- [ ] binaries built with `-X main.version=$VERSION`, matching the tag
      exactly (`v` prefix included)
- [ ] `SHA256SUMS` lists every binary in the release
- [ ] `power-monitor upgrade --check` on an installed machine sees the new
      version
