//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// launchd hooks are darwin-only; on Linux start/stop always use the
// generic setsid daemonizer.
func launchdStart() bool { return false }
func launchdStop() bool  { return false }

func desktopEntryPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "power-monitor.desktop")
}

func autostartEnable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	path := desktopEntryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Exec=%s start
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=5
Name=Power Monitor
Comment=Shows power delivery sources in the top bar
Icon=thunderbolt-symbolic
`, exe)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("autostart enabled (%s)\n", path)
	return nil
}

func autostartDisable() error {
	if err := os.Remove(desktopEntryPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("autostart disabled")
	return nil
}
