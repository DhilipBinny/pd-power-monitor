# Releasing

Release binaries are built **natively on each platform** — the tray code is
cgo (GTK on Linux, Cocoa/IOKit on macOS), so cross-compiling is not possible.
You need one build on a Mac and one on a Linux machine.

Every release MUST ship three assets:

| Asset | Built on |
|---|---|
| `power-monitor-darwin-arm64` | macOS (Apple Silicon) |
| `power-monitor-linux-amd64`  | Linux (x86-64) |
| `SHA256SUMS`                 | either — but it must list **all** binaries |

`power-monitor upgrade` verifies downloads against `SHA256SUMS`; a binary
missing from that file cannot be installed by the upgrade command.

## Build (run on each platform)

```bash
VERSION=v1.4.0   # the tag you are about to create

# macOS
go build -ldflags="-s -w -X main.version=$VERSION" -o power-monitor-darwin-arm64 .

# Linux
go build -ldflags="-s -w -X main.version=$VERSION" -o power-monitor-linux-amd64 .
```

Sanity-check each binary before publishing:

```bash
./power-monitor-<os>-<arch> version   # must print the new version
./power-monitor-<os>-<arch> status    # must read real power data
```

Also run `go vet ./...` on **both** platforms — build tags mean the Mac
never type-checks the Linux-only files and vice versa.

## Publish — both binaries at hand (preferred)

Collect both binaries in one directory (scp from the other machine), then:

```bash
shasum -a 256 power-monitor-darwin-arm64 power-monitor-linux-amd64 > SHA256SUMS
gh release create $VERSION power-monitor-darwin-arm64 power-monitor-linux-amd64 SHA256SUMS \
    --title "$VERSION" --notes "..."
```

## Publish — one platform now, the other later

Creating the release with only one binary is fine, **but the SHA256SUMS
uploaded later must list both binaries**. The flow:

```bash
# Machine A (e.g. Linux), release with what you have:
shasum -a 256 power-monitor-linux-amd64 > SHA256SUMS
gh release create $VERSION power-monitor-linux-amd64 SHA256SUMS --title "$VERSION" --notes "..."

# Machine B (e.g. Mac), later:
go build -ldflags="-s -w -X main.version=$VERSION" -o power-monitor-darwin-arm64 .
gh release download $VERSION -p 'power-monitor-*'          # fetch the other binary
shasum -a 256 power-monitor-darwin-arm64 power-monitor-linux-amd64 > SHA256SUMS
gh release upload $VERSION power-monitor-darwin-arm64
gh release upload $VERSION SHA256SUMS --clobber            # REPLACE the partial sums file
```

Until step B completes, `power-monitor upgrade` works only for the platform
already in `SHA256SUMS`; the other platform fails safely with
"no checksum for ... in SHA256SUMS".

## After publishing

Each machine updates itself — no installer needed:

```bash
sudo power-monitor upgrade && power-monitor restart
```

## Checklist

- [ ] `go vet` clean on macOS **and** Linux
- [ ] both binaries built with `-X main.version=$VERSION` (matching the tag exactly, `v` prefix included)
- [ ] `SHA256SUMS` lists every binary in the release
- [ ] `power-monitor-<os>-<arch> version` prints `$VERSION`
- [ ] after release: `power-monitor upgrade --check` on an installed machine sees the new version
