//go:build linux

package main

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// One directory scan per refresh cycle classifies every power_supply entry
// and reads its values; USBCPorts/Battery/ACOnline share that snapshot so
// the displayed state is internally consistent and sysfs is walked once
// per tick instead of three times.
type LinuxPowerSource struct {
	ports  []USBCPort
	bat    BatteryInfo
	ac     bool
	readAt time.Time
}

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

// sysfsInt reads an integer attribute; ok is false when the file is
// missing, empty, or unparseable.
func sysfsInt(path string) (int64, bool) {
	s := readSysfs(path)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (l *LinuxPowerSource) snapshot() {
	if !l.readAt.IsZero() && time.Since(l.readAt) < snapshotTTL {
		return
	}
	l.scan()
	l.readAt = time.Now()
}

func (l *LinuxPowerSource) scan() {
	entries, _ := filepath.Glob("/sys/class/power_supply/*")
	sort.Strings(entries)

	var usbDirs, batDirs []string
	ac := false
	for _, dir := range entries {
		switch readSysfs(filepath.Join(dir, "type")) {
		case "USB":
			usbDirs = append(usbDirs, dir)
		case "Battery":
			// scope=Device means peripheral (mouse, keyboard, headset)
			if readSysfs(filepath.Join(dir, "scope")) == "Device" {
				continue
			}
			batDirs = append(batDirs, dir)
		case "Mains":
			if readSysfs(filepath.Join(dir, "online")) == "1" {
				ac = true
			}
		}
	}

	l.ports = readPorts(usbDirs)
	l.bat = readBatteries(batDirs)
	l.ac = ac
}

func readPorts(dirs []string) []USBCPort {
	var ports []USBCPort
	for _, ps := range dirs {
		online := readSysfs(filepath.Join(ps, "online"))
		vNow, okV := sysfsInt(filepath.Join(ps, "voltage_now"))
		iMax, okI := sysfsInt(filepath.Join(ps, "current_max"))
		vMax, okM := sysfsInt(filepath.Join(ps, "voltage_max"))

		// A port without readable PD values can't be displayed meaningfully
		if !okV || !okI || !okM {
			continue
		}

		vNowF := float64(vNow) / 1e6
		iMaxF := float64(iMax) / 1e6
		vMaxF := float64(vMax) / 1e6

		// Number after filtering so the displayed ports are always C1..Cn
		n := strconv.Itoa(len(ports) + 1)
		ports = append(ports, USBCPort{
			Name:         "USB-C " + n,
			ShortName:    "C" + n,
			Online:       online == "1",
			Voltage:      vNowF,
			CurrentMax:   iMaxF,
			PDNegotiated: vNowF * iMaxF,
			PDMax:        vMaxF * iMaxF,
		})
	}
	return ports
}

// batteryReading carries the energy/charge counters needed to weight
// multi-battery capacity correctly (percentages of differently-sized
// packs are not additive).
type batteryReading struct {
	BatteryInfo
	now, full int64
}

func readOneBattery(bat string) batteryReading {
	status := readSysfs(filepath.Join(bat, "status"))
	if status == "" {
		status = statusUnknown
	}

	capacity, _ := sysfsInt(filepath.Join(bat, "capacity"))
	if capacity < 0 {
		capacity = 0
	}
	cs := readSysfs(filepath.Join(bat, "charge_control_start_threshold"))
	ce := readSysfs(filepath.Join(bat, "charge_control_end_threshold"))

	// Sign conventions are driver-specific: some report discharge as
	// negative power_now/current_now, so take magnitudes.
	var powerW float64
	if p, ok := sysfsInt(filepath.Join(bat, "power_now")); ok && p != 0 {
		powerW = math.Abs(float64(p)) / 1e6
	} else {
		i, okI := sysfsInt(filepath.Join(bat, "current_now"))
		v, okV := sysfsInt(filepath.Join(bat, "voltage_now"))
		if okI && okV && v > 0 {
			powerW = math.Abs(float64(i)) / 1e6 * float64(v) / 1e6
		}
	}

	now, okN := sysfsInt(filepath.Join(bat, "energy_now"))
	full, okF := sysfsInt(filepath.Join(bat, "energy_full"))
	if !okN || !okF {
		now, okN = sysfsInt(filepath.Join(bat, "charge_now"))
		full, okF = sysfsInt(filepath.Join(bat, "charge_full"))
	}
	if !okN || !okF {
		now, full = 0, 0
	}

	return batteryReading{
		BatteryInfo: BatteryInfo{
			Found:       true,
			Status:      status,
			PowerW:      powerW,
			Capacity:    int(capacity),
			ChargeStart: cs,
			ChargeEnd:   ce,
		},
		now:  now,
		full: full,
	}
}

func readBatteries(dirs []string) BatteryInfo {
	if len(dirs) == 0 {
		return BatteryInfo{Found: false, Status: statusUnknown}
	}
	if len(dirs) == 1 {
		return readOneBattery(dirs[0]).BatteryInfo
	}

	var totalPower float64
	var capSum, capCount int
	var nowSum, fullSum int64
	weighted := true
	var status string
	var cs, ce string

	for _, p := range dirs {
		b := readOneBattery(p)
		totalPower += b.PowerW
		capSum += b.Capacity
		capCount++
		if b.full > 0 {
			nowSum += b.now
			fullSum += b.full
		} else {
			weighted = false
		}
		// Use most active status (Discharging > Charging > Not charging)
		if status == "" || b.Status == statusDischarging || (b.Status == statusCharging && status != statusDischarging) {
			status = b.Status
		}
		if cs == "" {
			cs = b.ChargeStart
			ce = b.ChargeEnd
		}
	}

	// Weight by energy/charge capacity when every pack exposes it;
	// fall back to the plain average otherwise
	capacity := capSum / capCount
	if weighted && fullSum > 0 {
		capacity = int(nowSum * 100 / fullSum)
	}

	return BatteryInfo{
		Found:       true,
		Status:      status,
		PowerW:      totalPower,
		Capacity:    capacity,
		ChargeStart: cs,
		ChargeEnd:   ce,
	}
}

func (l *LinuxPowerSource) USBCPorts() []USBCPort {
	l.snapshot()
	return l.ports
}

func (l *LinuxPowerSource) Battery() BatteryInfo {
	l.snapshot()
	return l.bat
}

func (l *LinuxPowerSource) ACOnline() bool {
	l.snapshot()
	return l.ac
}
