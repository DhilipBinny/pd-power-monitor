//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const launchdLabel = "com.dhilipbinny.power-monitor"

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library/LaunchAgents", launchdLabel+".plist")
}

func launchdDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// launchdLoaded reports whether the agent is currently known to launchd.
func launchdLoaded() bool {
	return exec.Command("launchctl", "print", launchdDomain()+"/"+launchdLabel).Run() == nil
}

// bootstrapWithRetry tolerates launchd's asynchronous teardown: a bootstrap
// issued right after a bootout can fail with EIO until the old job is gone.
func bootstrapWithRetry(plist string) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		out, err := exec.Command("launchctl", "bootstrap", launchdDomain(), plist).CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}

// launchdStart starts the indicator through launchd when a LaunchAgent is
// installed, so the process stays under launchd's supervision instead of
// migrating to an unsupervised setsid daemon. Returns false to fall back
// to the generic daemonizer.
func launchdStart() bool {
	plist := launchAgentPath()
	if _, err := os.Stat(plist); err != nil {
		return false
	}
	if launchdLoaded() {
		// Loaded but not running (RunAtLoad already consumed): kick it
		if err := exec.Command("launchctl", "kickstart", launchdDomain()+"/"+launchdLabel).Run(); err != nil {
			fmt.Printf("launchctl kickstart failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := bootstrapWithRetry(plist); err != nil {
			fmt.Printf("launchctl bootstrap failed: %v\n", err)
			os.Exit(1)
		}
	}
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if pid := readPID(); pid != 0 {
			fmt.Printf("power-monitor started via launchd (pid %d)\n", pid)
			return true
		}
	}
	fmt.Printf("launchd accepted the job but the indicator didn't come up; check log: %s\n", logFilePath())
	os.Exit(1)
	return true
}

// launchdStop stops a launchd-managed instance via bootout so launchd's
// view stays consistent; the plist file remains, so RunAtLoad still fires
// at next login. Returns false when the instance isn't launchd-managed.
func launchdStop() bool {
	if !launchdLoaded() {
		return false
	}
	pid := readPID()
	if out, err := exec.Command("launchctl", "bootout", launchdDomain()+"/"+launchdLabel).CombinedOutput(); err != nil {
		fmt.Printf("launchctl bootout failed: %v\n%s", err, out)
		os.Exit(1)
	}
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if readPID() == 0 {
			fmt.Printf("power-monitor stopped (pid %d)\n", pid)
			return true
		}
	}
	fmt.Println("power-monitor stopped (launchd)")
	os.Remove(pidFilePath())
	return true
}

func autostartEnable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	plist := launchAgentPath()
	if err := os.MkdirAll(filepath.Dir(plist), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>--run</string></array>
	<key>RunAtLoad</key><true/>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel, exe, logFilePath(), logFilePath())
	if err := os.WriteFile(plist, []byte(content), 0644); err != nil {
		return err
	}

	// An indicator running outside launchd would make the RunAtLoad spawn
	// exit on the PID guard and mark the job failed; just register for
	// next login instead.
	if readPID() != 0 && !launchdLoaded() {
		fmt.Printf("autostart enabled (%s)\n", plist)
		fmt.Println("note: the current indicator runs outside launchd; it will be launchd-managed from next login")
		return nil
	}

	// Reload so a stale registration doesn't shadow the new plist
	_ = exec.Command("launchctl", "bootout", launchdDomain()+"/"+launchdLabel).Run()
	if err := bootstrapWithRetry(plist); err != nil {
		// Outside a GUI session bootstrap fails; the plist still takes
		// effect at next login
		fmt.Printf("autostart enabled (%s)\nnote: could not load it now (%v); it will start at next login\n", plist, err)
		return nil
	}
	fmt.Printf("autostart enabled (%s)\n", plist)
	return nil
}

// autostartDisable only affects login behavior — a running indicator keeps
// running (matching the Linux semantics). The loaded launchd job, if any,
// disappears at logout since its plist is gone.
func autostartDisable() error {
	plist := launchAgentPath()
	if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("autostart disabled")
	if readPID() != 0 {
		fmt.Println("(the running indicator was left running; 'power-monitor stop' stops it)")
	}
	return nil
}
