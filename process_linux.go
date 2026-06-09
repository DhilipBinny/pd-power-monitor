//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func daemonize(binary string) *exec.Cmd {
	cmd := exec.Command(binary, "--run")
	cmd.Stdout = nil

	logDir := os.Getenv("XDG_RUNTIME_DIR")
	if logDir == "" {
		logDir = "/tmp"
	}
	logFile, err := os.OpenFile(
		filepath.Join(logDir, "power-monitor.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600,
	)
	if err == nil {
		cmd.Stderr = logFile
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
