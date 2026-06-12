package main

import (
	"fmt"
	"os"
)

func runIndicator() {
	// Atomically claim the PID file; closes the race between launchd
	// RunAtLoad and a manual start at login.
	if err := acquirePID(); err != nil {
		fmt.Printf("power-monitor: %v\n", err)
		os.Exit(1)
	}
	defer removeOwnPID()

	source := NewPowerSource()
	tray := NewTray()
	tray.Init(source)

	installSignalHandler(tray.Quit)

	tray.Run()
}

func cmdAutostart(args []string) {
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	var err error
	switch mode {
	case "on", "enable":
		err = autostartEnable()
	case "off", "disable":
		err = autostartDisable()
	default:
		fmt.Println("usage: power-monitor autostart on|off")
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("autostart %s failed: %v\n", mode, err)
		os.Exit(1)
	}
}

func main() {
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "restart":
		cmdRestart()
	case "status":
		cmdStatus()
	case "autostart":
		cmdAutostart(os.Args[2:])
	case "upgrade":
		cmdUpgrade(os.Args[2:])
	case "version", "--version":
		fmt.Println("power-monitor", versionString())
	case "--run":
		runIndicator()
	case "-h", "--help", "help":
		printHelp()
	default:
		fmt.Printf("unknown command: %s\n", cmd)
		printHelp()
		os.Exit(1)
	}
}
