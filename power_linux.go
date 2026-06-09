//go:build linux

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type LinuxPowerSource struct{}

func NewPowerSource() PowerSource {
	return &LinuxPowerSource{}
}

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func sysfsInt(path string) int64 {
	s := readSysfs(path)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (l *LinuxPowerSource) USBCPorts() []USBCPort {
	var matches []string
	all, _ := filepath.Glob("/sys/class/power_supply/*/type")
	for _, t := range all {
		typ := readSysfs(t)
		if typ == "USB" {
			matches = append(matches, filepath.Dir(t))
		}
	}
	sort.Strings(matches)

	var ports []USBCPort
	for i, ps := range matches {
		online := readSysfs(filepath.Join(ps, "online"))
		vNow := sysfsInt(filepath.Join(ps, "voltage_now"))
		iMax := sysfsInt(filepath.Join(ps, "current_max"))
		vMax := sysfsInt(filepath.Join(ps, "voltage_max"))

		// Skip entries with unreadable values
		if vNow < 0 || iMax < 0 || vMax < 0 {
			continue
		}

		vNowF := float64(vNow) / 1e6
		iMaxF := float64(iMax) / 1e6
		vMaxF := float64(vMax) / 1e6

		ports = append(ports, USBCPort{
			Name:         "USB-C " + strconv.Itoa(i+1),
			Online:       online == "1",
			Voltage:      vNowF,
			CurrentMax:   iMaxF,
			PDNegotiated: vNowF * iMaxF,
			PDMax:        vMaxF * iMaxF,
		})
	}
	return ports
}

func findBatteryPaths() []string {
	var paths []string
	matches, _ := filepath.Glob("/sys/class/power_supply/*/type")
	for _, m := range matches {
		if readSysfs(m) != "Battery" {
			continue
		}
		dir := filepath.Dir(m)
		// scope=Device means peripheral (mouse, keyboard, headset)
		// scope=System or absent means laptop battery
		if readSysfs(filepath.Join(dir, "scope")) == "Device" {
			continue
		}
		paths = append(paths, dir)
	}
	sort.Strings(paths)
	return paths
}

func readOneBattery(bat string) BatteryInfo {
	status := readSysfs(filepath.Join(bat, "status"))
	if status == "" {
		status = "Unknown"
	}

	cap := sysfsInt(filepath.Join(bat, "capacity"))
	if cap < 0 {
		cap = 0
	}
	cs := readSysfs(filepath.Join(bat, "charge_control_start_threshold"))
	ce := readSysfs(filepath.Join(bat, "charge_control_end_threshold"))

	var powerW float64
	pNow := sysfsInt(filepath.Join(bat, "power_now"))
	if pNow > 0 {
		powerW = float64(pNow) / 1e6
	} else {
		i := sysfsInt(filepath.Join(bat, "current_now"))
		v := sysfsInt(filepath.Join(bat, "voltage_now"))
		if i > 0 && v > 0 { // sysfsInt returns -1 on parse error, 0 on missing
			powerW = (float64(v) / 1e6) * (float64(i) / 1e6)
		}
	}
	if powerW < 0 {
		powerW = 0
	}

	return BatteryInfo{
		Found:       true,
		Status:      status,
		PowerW:      powerW,
		Capacity:    int(cap),
		ChargeStart: cs,
		ChargeEnd:   ce,
	}
}

func (l *LinuxPowerSource) Battery() BatteryInfo {
	paths := findBatteryPaths()
	if len(paths) == 0 {
		return BatteryInfo{Found: false, Status: "Unknown"}
	}

	// Single battery: return directly
	if len(paths) == 1 {
		return readOneBattery(paths[0])
	}

	// Multi-battery: aggregate
	var totalPower float64
	var totalCap, totalCount int
	var status string
	var cs, ce string

	for _, p := range paths {
		b := readOneBattery(p)
		totalPower += b.PowerW
		totalCap += b.Capacity
		totalCount++
		// Use most active status (Discharging > Charging > Not charging)
		if status == "" || b.Status == "Discharging" || (b.Status == "Charging" && status != "Discharging") {
			status = b.Status
		}
		if cs == "" {
			cs = b.ChargeStart
			ce = b.ChargeEnd
		}
	}

	return BatteryInfo{
		Found:       true,
		Status:      status,
		PowerW:      totalPower,
		Capacity:    totalCap / totalCount,
		ChargeStart: cs,
		ChargeEnd:   ce,
	}
}

func (l *LinuxPowerSource) ACOnline() bool {
	matches, _ := filepath.Glob("/sys/class/power_supply/*/type")
	for _, m := range matches {
		if readSysfs(m) == "Mains" {
			dir := filepath.Dir(m)
			if readSysfs(filepath.Join(dir, "online")) == "1" {
				return true
			}
		}
	}
	return false
}
