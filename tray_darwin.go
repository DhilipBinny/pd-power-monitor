//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "tray_darwin.h"
*/
import "C"
import (
	"runtime"
)

// AppKit requires the run loop on the process main thread; lock the main
// goroutine to it before anything else runs.
func init() {
	runtime.LockOSThread()
}

var trayInstance *DarwinTray

type DarwinTray struct {
	source    PowerSource
	portCount int

	// last rendered state, to skip cgo/AppKit work when nothing changed
	lastState DisplayState
	rendered  bool
}

func NewTray() TrayUI {
	t := &DarwinTray{}
	trayInstance = t
	return t
}

func (t *DarwinTray) Init(source PowerSource) {
	t.source = source
	C.tray_init()
}

func (t *DarwinTray) update() {
	ports := t.source.USBCPorts()
	bat := t.source.Battery()
	ac := t.source.ACOnline()

	if len(ports) != t.portCount {
		t.portCount = len(ports)
		C.tray_set_port_count(C.int(t.portCount))
		t.rendered = false
	}

	state := ComputeDisplay(ports, bat, ac)
	prev, force := t.lastState, !t.rendered

	for i, label := range state.PortLabels {
		if force || i >= len(prev.PortLabels) || prev.PortLabels[i] != label {
			withCStr(label, func(cs *C.char) {
				C.tray_set_port_label(C.int(i), cs)
			})
		}
	}

	if force || prev.BatLabel != state.BatLabel {
		withCStr(state.BatLabel, func(cs *C.char) { C.tray_set_bat(cs) })
	}
	if force || prev.TotalLabel != state.TotalLabel {
		withCStr(state.TotalLabel, func(cs *C.char) { C.tray_set_total(cs) })
	}
	// Re-setting the status item title triggers AppKit layout; skip when
	// unchanged. ThreshLabel is intentionally unused: macOS manages charge
	// limits itself, so the menu has no charge-range row.
	if force || prev.BarLabel != state.BarLabel {
		withCStr(state.BarLabel, func(cs *C.char) { C.tray_set_title(cs) })
	}

	t.lastState = state
	t.rendered = true
}

func (t *DarwinTray) Run() {
	t.update()
	C.tray_run(C.double(refreshPeriod.Seconds()))
}

func (t *DarwinTray) Quit() {
	C.tray_quit()
}

//export goOnUpdate
func goOnUpdate() {
	if trayInstance != nil {
		trayInstance.update()
	}
}
