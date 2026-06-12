package main

import "time"

// One refresh cycle for both trays; the GTK timeout (ms) and NSTimer
// interval (s) are derived from this single constant.
const refreshPeriod = 3 * time.Second

// snapshotTTL lets the three PowerSource accessors within one tick share a
// single hardware read. It must stay well below refreshPeriod so every tick
// still observes fresh data.
const snapshotTTL = time.Second

type USBCPort struct {
	Name         string
	ShortName    string // bar-label prefix, e.g. "C1", "MS"
	Online       bool
	Voltage      float64
	CurrentMax   float64
	PDNegotiated float64
	PDMax        float64
}

// Battery status strings follow the Linux power-supply sysfs ABI
// (the kernel emits these exact spellings); the darwin backend
// produces the same set so ComputeDisplay can compare against them.
const (
	statusCharging    = "Charging"
	statusDischarging = "Discharging"
	statusNotCharging = "Not charging"
	statusFull        = "Full"
	statusUnknown     = "Unknown"
)

type BatteryInfo struct {
	Found       bool
	Status      string
	PowerW      float64
	Capacity    int
	ChargeStart string
	ChargeEnd   string
}

type PowerSource interface {
	USBCPorts() []USBCPort
	Battery() BatteryInfo
	ACOnline() bool
}

type TrayUI interface {
	Init(source PowerSource)
	Run()
	Quit()
}
