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
	"unsafe"
)

// AppKit requires the run loop on the process main thread; lock the main
// goroutine to it before anything else runs.
func init() {
	runtime.LockOSThread()
}

const refreshSeconds = 3.0

var trayInstance *DarwinTray

type DarwinTray struct {
	source    PowerSource
	portCount int
}

func NewTray() TrayUI {
	t := &DarwinTray{}
	trayInstance = t
	return t
}

func withCStr(s string, fn func(*C.char)) {
	cs := C.CString(s)
	fn(cs)
	C.free(unsafe.Pointer(cs))
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
	}

	state := ComputeDisplay(ports, bat, ac)

	for i, label := range state.PortLabels {
		withCStr(label, func(cs *C.char) {
			C.tray_set_port_label(C.int(i), cs)
		})
	}

	withCStr(state.BatLabel, func(cs *C.char) { C.tray_set_bat(cs) })
	withCStr(state.TotalLabel, func(cs *C.char) { C.tray_set_total(cs) })
	withCStr(state.BarLabel, func(cs *C.char) { C.tray_set_title(cs) })
}

func (t *DarwinTray) Run() {
	t.update()
	C.tray_run(C.double(refreshSeconds))
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
