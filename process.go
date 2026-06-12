//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func pidFilePath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/tmp/power-monitor-%d", os.Getuid())
		_ = os.MkdirAll(dir, 0700)
	}
	return filepath.Join(dir, "power-monitor.pid")
}

func writePID() {
	_ = os.WriteFile(pidFilePath(), []byte(strconv.Itoa(os.Getpid())), 0600)
}

func readPID() int {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0
	}
	proc, _ := os.FindProcess(pid)
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return pid
	}
	// EPERM means process exists but owned by another user
	if errors.Is(err, syscall.EPERM) {
		return pid
	}
	// Process doesn't exist, clean stale PID file
	os.Remove(pidFilePath())
	return 0
}

func removePID() {
	os.Remove(pidFilePath())
}

var quitFunc func()

func installSignalHandler(quit func()) {
	quitFunc = quit
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		removePID()
		if quitFunc != nil {
			quitFunc()
		}
	}()
}

func cmdStop() {
	pid := readPID()
	if pid == 0 {
		fmt.Println("power-monitor is not running")
		os.Exit(1)
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("failed to stop pid %d: %v\n", pid, err)
		os.Exit(1)
	}
	// Wait for process to exit, checking the saved PID not re-probing
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		err := proc.Signal(syscall.Signal(0))
		if err != nil && !errors.Is(err, syscall.EPERM) {
			fmt.Printf("power-monitor stopped (pid %d)\n", pid)
			os.Remove(pidFilePath())
			return
		}
	}
	fmt.Printf("power-monitor pid %d did not exit in time, killing\n", pid)
	proc.Signal(syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	removePID()
}

func cmdStart() {
	pid := readPID()
	if pid != 0 {
		fmt.Printf("power-monitor is already running (pid %d)\n", pid)
		os.Exit(1)
	}
	cmd := daemonize(os.Args[0])
	if err := cmd.Start(); err != nil {
		fmt.Printf("failed to start: %v\n", err)
		os.Exit(1)
	}
	childPid := cmd.Process.Pid
	// Wait briefly to verify child didn't crash immediately
	time.Sleep(500 * time.Millisecond)
	proc, _ := os.FindProcess(childPid)
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		fmt.Printf("power-monitor failed to start (pid %d exited immediately)\n", childPid)
		logPath := os.Getenv("XDG_RUNTIME_DIR")
		if logPath == "" {
			logPath = "/tmp"
		}
		fmt.Printf("check log: %s/power-monitor.log\n", logPath)
		os.Exit(1)
	}
	fmt.Printf("power-monitor started (pid %d)\n", childPid)
}

func cmdRestart() {
	pid := readPID()
	if pid != 0 {
		cmdStop()
	}
	cmdStart()
}

func cmdStatus() {
	source := NewPowerSource()
	pid := readPID()

	if pid != 0 {
		fmt.Printf("power-monitor is running (pid %d)\n", pid)
	} else {
		fmt.Println("power-monitor is not running")
	}

	ports := source.USBCPorts()
	bat := source.Battery()
	ac := source.ACOnline()
	fmt.Println()

	for _, p := range ports {
		if p.Online {
			fmt.Printf("  %s: %.0fW (%.0fV @ %.1fA) [max %.0fW]\n",
				p.Name, p.PDNegotiated, p.Voltage, p.CurrentMax, p.PDMax)
		} else {
			fmt.Printf("  %s: disconnected\n", p.Name)
		}
	}

	if bat.Found {
		fmt.Printf("  Battery: %d%% | %s | %.1fW\n", bat.Capacity, bat.Status, bat.PowerW)
		if bat.ChargeStart != "" && bat.ChargeEnd != "" {
			fmt.Printf("  Charge range: %s%% - %s%%\n", bat.ChargeStart, bat.ChargeEnd)
		}
	} else {
		fmt.Println("  Battery: not present")
	}

	if ac && len(ports) == 0 {
		fmt.Println("  AC adapter: connected")
	}
}

func printHelp() {
	autostart := "~/.config/autostart/"
	if runtime.GOOS == "darwin" {
		autostart = "a launchd LaunchAgent in ~/Library/LaunchAgents/"
	}
	fmt.Printf(`power-monitor - USB-C & battery power indicator for the system tray

Usage:
  power-monitor start     Start the indicator (background)
  power-monitor stop      Stop the indicator
  power-monitor restart   Restart the indicator
  power-monitor status    Show running status and current power info

The indicator auto-starts on login via %s
`, autostart)
}
