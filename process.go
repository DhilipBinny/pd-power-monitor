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
		// MkdirAll succeeds on a pre-existing dir without checking who owns
		// it; refuse a squatted dir rather than follow planted symlinks
		if fi, err := os.Lstat(dir); err == nil {
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !fi.IsDir() || (ok && int(st.Uid) != os.Getuid()) {
				fmt.Fprintf(os.Stderr, "error: %s exists but is not owned by you; remove it first\n", dir)
				os.Exit(1)
			}
			if fi.Mode().Perm()&0077 != 0 {
				_ = os.Chmod(dir, 0700)
			}
		}
	}
	return filepath.Join(dir, "power-monitor.pid")
}

// acquirePID atomically claims the PID file for this process. O_EXCL plus
// the stale-check-and-retry closes the read-then-write race between two
// starting instances (e.g. launchd RunAtLoad vs a manual start at login).
func acquirePID() error {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(pidFilePath(),
			os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
		if err == nil {
			_, werr := f.WriteString(strconv.Itoa(os.Getpid()))
			f.Close()
			return werr
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		// File exists: live instance, or a stale leftover readPID cleans up
		if pid := readPID(); pid != 0 {
			return fmt.Errorf("already running (pid %d)", pid)
		}
	}
	return fmt.Errorf("could not claim PID file %s", pidFilePath())
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
	if proc.Signal(syscall.Signal(0)) == nil {
		return pid
	}
	// EPERM means the PID exists but belongs to another user — our daemon
	// always runs as the user who reads this file, so the PID was recycled.
	// Either way the file is stale; clean it up.
	os.Remove(pidFilePath())
	return 0
}

// removeOwnPID deletes the PID file only when it still names this process,
// so a losing instance's exit can't unregister the winner.
func removeOwnPID() {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid == os.Getpid() {
		os.Remove(pidFilePath())
	}
}

func installSignalHandler(quit func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		// Quit first: it marshals onto the UI loop and lets the main
		// goroutine's deferred cleanup run. Removing the PID file first
		// would leave an unmanageable live process if the quit is lost.
		if quit != nil {
			quit()
		}
		removeOwnPID()
	}()
}

func cmdStop() {
	pid := readPID()
	if pid == 0 {
		fmt.Println("power-monitor is not running")
		os.Exit(1)
	}
	if launchdStop() {
		return
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("failed to stop pid %d: %v\n", pid, err)
		os.Exit(1)
	}
	if !waitProcessExit(proc) {
		fmt.Printf("power-monitor pid %d did not exit in time, killing\n", pid)
		proc.Signal(syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond)
	} else {
		fmt.Printf("power-monitor stopped (pid %d)\n", pid)
	}
	os.Remove(pidFilePath())
}

// waitProcessExit polls for up to 3s; true when the process is gone.
func waitProcessExit(proc *os.Process) bool {
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		err := proc.Signal(syscall.Signal(0))
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return true
		}
	}
	return false
}

func cmdStart() {
	pid := readPID()
	if pid != 0 {
		fmt.Printf("power-monitor is already running (pid %d)\n", pid)
		os.Exit(1)
	}
	if launchdStart() {
		return
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
		fmt.Printf("check log: %s\n", logFilePath())
		os.Exit(1)
	}
	fmt.Printf("power-monitor started (pid %d)\n", childPid)
}

func cmdRestart() {
	if readPID() != 0 {
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

	state := ComputeDisplay(source.USBCPorts(), source.Battery(), source.ACOnline())
	fmt.Println()
	for _, l := range state.PortLabels {
		fmt.Println("  " + l)
	}
	fmt.Println("  " + state.BatLabel)
	fmt.Println("  " + state.TotalLabel)
	if state.ThreshLabel != "" {
		fmt.Println("  " + state.ThreshLabel)
	}
}

func printHelp() {
	autostart := "an XDG autostart entry"
	if runtime.GOOS == "darwin" {
		autostart = "a launchd LaunchAgent"
	}
	fmt.Printf(`power-monitor - USB-C & battery power indicator for the system tray

Usage:
  power-monitor start          Start the indicator (background)
  power-monitor stop           Stop the indicator
  power-monitor restart        Restart the indicator
  power-monitor status         Show running status and current power info
  power-monitor autostart on   Enable start-on-login (%s)
  power-monitor autostart off  Disable start-on-login
  power-monitor upgrade        Self-update to the latest GitHub release
                               (--check to preview, --to vX.Y.Z to pin)
  power-monitor version        Show the installed version
`, autostart)
}
