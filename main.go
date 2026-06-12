package main

import (
	"fmt"
	"os"
)

func runIndicator() {
	// Guard against a second instance (e.g. launchd RunAtLoad alongside a
	// manually started one); cmdStart checks too, but --run can be invoked
	// directly.
	if pid := readPID(); pid != 0 && pid != os.Getpid() {
		fmt.Printf("power-monitor is already running (pid %d)\n", pid)
		os.Exit(1)
	}
	writePID()
	defer removePID()

	source := NewPowerSource()
	tray := NewTray()
	tray.Init(source)

	installSignalHandler(tray.Quit)

	tray.Run()
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
