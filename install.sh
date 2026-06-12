#!/bin/sh
set -e

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

    # Get latest release tag
    if command -v curl >/dev/null 2>&1; then
        LATEST=$(curl -sI "https://github.com/$REPO/releases/latest" | grep -i "^location:" | sed 's|.*/||' | tr -d '\r')
    elif command -v wget >/dev/null 2>&1; then
        LATEST=$(wget -qS --max-redirect=0 "https://github.com/$REPO/releases/latest" 2>&1 | grep -i "Location:" | sed 's|.*/||' | tr -d '\r')
    else
        echo "Error: curl or wget required"; exit 1
    fi

    if [ -z "$LATEST" ]; then
        echo "Error: could not determine latest release"
        exit 1
    fi

    DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST/$BINARY_NAME-$OS-$ARCH"
    echo "Downloading $BINARY_NAME $LATEST ($OS/$ARCH)..."

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

    # Set up autostart
    if [ "$OS" = "linux" ]; then
        AUTOSTART_DIR="$HOME/.config/autostart"
        mkdir -p "$AUTOSTART_DIR"
        cat > "$AUTOSTART_DIR/power-monitor.desktop" << 'DESKTOP'
[Desktop Entry]
Type=Application
Exec=/usr/local/bin/power-monitor start
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=5
Name=Power Monitor
Comment=Shows power delivery sources in the top bar
Icon=thunderbolt-symbolic
DESKTOP
    else
        LAUNCH_AGENT="$HOME/Library/LaunchAgents/com.dhilipbinny.power-monitor.plist"
        mkdir -p "$HOME/Library/LaunchAgents"
        cat > "$LAUNCH_AGENT" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>com.dhilipbinny.power-monitor</string>
	<key>ProgramArguments</key>
	<array><string>$INSTALL_DIR/$BINARY_NAME</string><string>--run</string></array>
	<key>RunAtLoad</key><true/>
	<key>StandardOutPath</key><string>/tmp/power-monitor.log</string>
	<key>StandardErrorPath</key><string>/tmp/power-monitor.log</string>
</dict>
</plist>
PLIST
        launchctl unload "$LAUNCH_AGENT" 2>/dev/null || true
        if ! launchctl load "$LAUNCH_AGENT" 2>/dev/null; then
            echo "Note: could not load the LaunchAgent now (no GUI session?);"
            echo "it will start automatically at next login."
        fi
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
