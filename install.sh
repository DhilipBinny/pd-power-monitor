#!/bin/sh
set -e

# Must stay in sync with repoSlug in upgrade.go
REPO="DhilipBinny/pd-power-monitor"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="power-monitor"

main() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    case "$OS" in
        linux|darwin) ;;
        *) echo "Unsupported OS: $OS"; exit 1 ;;
    esac

    # Install runtime dependencies (Linux only; macOS uses native frameworks)
    if [ "$OS" = "linux" ]; then
        echo "Installing dependencies..."
        if command -v apt-get >/dev/null 2>&1; then
            sudo apt-get update -qq
            sudo apt-get install -y -qq libgtk-3-0 libayatana-appindicator3-1 >/dev/null
        elif command -v dnf >/dev/null 2>&1; then
            sudo dnf install -y -q gtk3 libayatana-appindicator-gtk3 >/dev/null
        elif command -v pacman >/dev/null 2>&1; then
            sudo pacman -S --noconfirm --needed gtk3 libayatana-appindicator >/dev/null
        else
            echo "Warning: could not detect package manager."
            echo "Please install GTK3 and libayatana-appindicator3 manually."
        fi
    fi

    # GitHub's stable alias resolves the latest release in one request
    DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/$BINARY_NAME-$OS-$ARCH"
    echo "Downloading $BINARY_NAME latest ($OS/$ARCH)..."

    TMPFILE=$(mktemp)
    # -f makes curl fail on HTTP errors instead of saving the 404 body
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$DOWNLOAD_URL" -o "$TMPFILE" || DOWNLOAD_FAILED=1
    else
        wget -q "$DOWNLOAD_URL" -O "$TMPFILE" || DOWNLOAD_FAILED=1
    fi

    if [ -n "$DOWNLOAD_FAILED" ] || [ ! -s "$TMPFILE" ]; then
        rm -f "$TMPFILE"
        echo "Error: download failed — no release binary for $OS-$ARCH?"
        echo "You can build from source instead; see the README."
        exit 1
    fi

    # Install binary (-d creates /usr/local/bin if missing, e.g. fresh macOS)
    echo "Installing to $INSTALL_DIR/$BINARY_NAME..."
    sudo install -d "$INSTALL_DIR"
    sudo install -m 755 "$TMPFILE" "$INSTALL_DIR/$BINARY_NAME"
    rm -f "$TMPFILE"

    # Stop any instance from a previous install before setting up autostart
    "$INSTALL_DIR/$BINARY_NAME" stop >/dev/null 2>&1 || true

    # Set up autostart — the binary owns the platform-specific registration
    # (XDG .desktop on Linux, launchd LaunchAgent on macOS), so upgrades can
    # refresh it with 'power-monitor autostart on'
    if ! "$INSTALL_DIR/$BINARY_NAME" autostart on; then
        echo "Note: autostart setup failed; run 'power-monitor autostart on' later."
    fi

    echo ""
    echo "Installed successfully!"
    echo ""
    echo "  Start now:     power-monitor start"
    echo "  Check status:  power-monitor status"
    echo "  Auto-starts on login"
    echo ""
}

main
