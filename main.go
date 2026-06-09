package main

import (
	"fmt"
	"os"
)

func runIndicator() {
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
