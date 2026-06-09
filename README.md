# power-monitor

A lightweight system tray indicator for Linux that shows real-time USB-C Power Delivery wattage, AC adapter status, and battery information.

Built for laptops powered via USB-C (monitors, docks, chargers) where you want to know exactly how much power each source is delivering — especially useful when running off a USB-C monitor and want to confirm it supplies enough wattage.

## Features

- **USB-C PD wattage** — shows negotiated power per port (voltage, current, max capability)
- **AC barrel jack detection** — detects traditional round-pin adapters on laptops that have them
- **Battery status** — charge percentage, charging/discharging state, power draw
- **Charge threshold display** — shows vendor charge limits (Dell, Lenovo, ASUS, Framework)
- **Multi-battery support** — aggregates internal + removable batteries (ThinkPad, etc.)
- **Hotplug aware** — detects USB-C port changes when docking/undocking
- **Portable** — works across laptop vendors (Dell, Lenovo, HP, ASUS, Framework, Chromebooks)

## Top Bar Display

```
C1:68W | C2:49W              ← Two USB-C sources providing power
C1:68W                       ← Single USB-C (e.g., monitor only)
S:AC                         ← Barrel jack adapter connected
C1:68W | CHG:3.5W            ← USB-C powering + battery charging
BAT:15.2W                    ← Running on battery
```

Click the indicator for detailed info:

```
Power Monitor
──────────────────────────────────────
USB-C 1: 68W (15V @ 4.5A) [max 90W]
USB-C 2: 49W (15V @ 3.2A) [max 65W]
──────────────────────────────────────
Battery: 72% | Not charging | 0.0W
Power input: 117W
Charge range: 50% - 90%
──────────────────────────────────────
Quit
```

## Installation

### Prerequisites

```bash
# Ubuntu/Debian
sudo apt install -y golang libayatana-appindicator3-dev libgtk-3-dev pkg-config

# Fedora
sudo dnf install -y golang libayatana-appindicator-gtk3-devel gtk3-devel pkg-config

# Arch
sudo pacman -S go libayatana-appindicator gtk3 pkgconf
```

### Build and install

```bash
git clone https://github.com/YOUR_USERNAME/power-monitor.git
cd power-monitor
go build -o power-monitor .
sudo cp power-monitor /usr/local/bin/
```

### Enable autostart (optional)

```bash
mkdir -p ~/.config/autostart
cat > ~/.config/autostart/power-monitor.desktop << 'EOF'
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
EOF
```

## Usage

```bash
power-monitor start       # Start the indicator (runs in background)
power-monitor stop        # Stop the indicator
power-monitor restart     # Restart the indicator
power-monitor status      # Show power info (works without the tray running)
power-monitor help        # Show usage
```

### Example: check power without starting the tray

```
$ power-monitor status
power-monitor is not running

  USB-C 1: 68W (15V @ 4.5A) [max 90W]
  USB-C 2: 49W (15V @ 3.2A) [max 65W]
  Battery: 72% | Discharging | 0.0W
  Charge range: 50% - 90%
```

## How it works

power-monitor reads the Linux kernel's power supply subsystem via sysfs:

| Data | Source |
|---|---|
| USB-C PD negotiation | `/sys/class/power_supply/*/type` = `USB` |
| AC adapter (barrel jack) | `/sys/class/power_supply/*/type` = `Mains` |
| Battery status | `/sys/class/power_supply/*/type` = `Battery` |
| Charge thresholds | `charge_control_start_threshold` / `charge_control_end_threshold` |

It detects USB-C power supplies from any driver (UCSI, TCPM, FUSB302, Cros EC) by filtering on `type=USB` rather than hardcoding driver-specific paths.

Battery power is read from `power_now` when available, with a fallback to `voltage_now * current_now` for systems that don't expose it.

## Compatibility

**Tested on:**
- Dell Pro 14 (PC14250) — Intel Meteor Lake, USB-C PD via UCSI

**Should work on any Linux laptop with:**
- Kernel 5.10+ (power supply sysfs interface)
- GNOME, MATE, Budgie, Cinnamon, or any DE with AppIndicator/SNI support
- USB-C Power Delivery and/or barrel jack power

**Known vendor support:**
| Feature | Dell | Lenovo | HP | ASUS | Framework |
|---|---|---|---|---|---|
| USB-C PD wattage | Yes | Yes | Yes | Yes | Yes |
| Barrel jack detection | Yes | Yes | Yes | Yes | N/A |
| Charge thresholds | Yes | Yes | Partial | Yes | Yes |
| Multi-battery | N/A | Yes | N/A | N/A | N/A |

## Architecture

```
power-monitor/
├── main.go              # CLI dispatch
├── types.go             # Interfaces (PowerSource, TrayUI)
├── logic.go             # Display formatting (pure Go, cross-platform)
├── process.go           # PID management, start/stop/restart
├── process_linux.go     # Linux daemonize + logging
├── power_linux.go       # Linux sysfs power backend
└── tray_linux.go        # GTK/AppIndicator system tray
```

The codebase is structured for cross-platform expansion. To add macOS support, implement `power_darwin.go` (IOKit/SMC backend) and `tray_darwin.go` (NSStatusItem UI) — all shared logic in `types.go`, `logic.go`, and `process.go` stays untouched.

## Resource usage

| Metric | Value |
|---|---|
| Binary size | 2.9 MB |
| RAM usage | ~29 MB |
| CPU (idle) | ~0% |
| Update interval | 3 seconds |
| Dependencies at runtime | GTK 3, libayatana-appindicator |

## Uninstall

```bash
power-monitor stop
sudo rm /usr/local/bin/power-monitor
rm ~/.config/autostart/power-monitor.desktop
```

## Contributing

Contributions are welcome. Please open an issue first to discuss what you would like to change.

Areas where help is needed:
- macOS support (`power_darwin.go` + `tray_darwin.go`)
- Testing on more laptop models (especially HP, ASUS, Chromebooks)
- Wayland-native tray support (for compositors without SNI/AppIndicator)

## License

[MIT](LICENSE)
