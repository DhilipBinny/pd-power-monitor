//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// logFilePath returns a per-user log location: the session runtime dir on
// Linux, the user cache dir elsewhere (macOS: ~/Library/Caches). The old
// world-shared /tmp fallback was symlink-attackable on multi-user hosts.
func logFilePath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "power-monitor.log")
	}
	if dir, err := os.UserCacheDir(); err == nil {
		sub := filepath.Join(dir, "power-monitor")
		if os.MkdirAll(sub, 0700) == nil {
			return filepath.Join(sub, "power-monitor.log")
		}
	}
	return filepath.Join(os.TempDir(), "power-monitor.log")
}

func daemonize(binary string) *exec.Cmd {
	cmd := exec.Command(binary, "--run")
	cmd.Stdout = nil

	logFile, err := os.OpenFile(
		logFilePath(),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600,
	)
	if err == nil {
		cmd.Stderr = logFile
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
