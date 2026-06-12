//go:build darwin

package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>

typedef struct {
	long long voltage_mv;   // battery pack voltage
	long long amperage_ma;  // signed: positive charging, negative discharging
	long long cur_cap;
	long long max_cap;
	int is_charging;
	int external_connected;
	int fully_charged;

	int adapter_present;
	long long adapter_watts;
	long long adapter_voltage_mv;
	long long adapter_current_ma;
	long long adapter_max_watts; // best PD profile from UsbHvcMenu
} power_info;

static long long dict_ll(CFDictionaryRef d, CFStringRef key, long long def) {
	CFTypeRef v = CFDictionaryGetValue(d, key);
	long long out = def;
	if (v && CFGetTypeID(v) == CFNumberGetTypeID())
		CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, &out);
	return out;
}

static int dict_bool(CFDictionaryRef d, CFStringRef key) {
	CFTypeRef v = CFDictionaryGetValue(d, key);
	return v && CFGetTypeID(v) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)v);
}

// The battery service never disappears on a laptop; look it up once.
static io_service_t batt_service(void) {
	static io_service_t service = 0;
	if (!service)
		service = IOServiceGetMatchingService(
			0, IOServiceMatching("AppleSmartBattery"));
	return service;
}

static int read_power(power_info *out) {
	memset(out, 0, sizeof(*out));

	io_service_t service = batt_service();
	if (!service)
		return -1;

	CFMutableDictionaryRef props = NULL;
	kern_return_t kr = IORegistryEntryCreateCFProperties(
		service, &props, kCFAllocatorDefault, 0);
	if (kr != KERN_SUCCESS || !props)
		return -1;

	out->voltage_mv = dict_ll(props, CFSTR("Voltage"), 0);
	out->amperage_ma = dict_ll(props, CFSTR("Amperage"), 0);
	out->cur_cap = dict_ll(props, CFSTR("CurrentCapacity"), 0);
	out->max_cap = dict_ll(props, CFSTR("MaxCapacity"), 0);
	out->is_charging = dict_bool(props, CFSTR("IsCharging"));
	out->external_connected = dict_bool(props, CFSTR("ExternalConnected"));
	out->fully_charged = dict_bool(props, CFSTR("FullyCharged"));

	CFTypeRef ad = CFDictionaryGetValue(props, CFSTR("AdapterDetails"));
	if (ad && CFGetTypeID(ad) == CFDictionaryGetTypeID()) {
		CFDictionaryRef adapter = (CFDictionaryRef)ad;
		out->adapter_watts = dict_ll(adapter, CFSTR("Watts"), 0);
		out->adapter_voltage_mv = dict_ll(adapter, CFSTR("AdapterVoltage"), 0);
		out->adapter_current_ma = dict_ll(adapter, CFSTR("Current"), 0);
		// Some adapters report voltage/current but no Watts key
		if (out->adapter_watts > 0 ||
		    (out->adapter_voltage_mv > 0 && out->adapter_current_ma > 0))
			out->adapter_present = 1;

		// UsbHvcMenu lists the PD source capabilities; the largest
		// voltage*current entry is the adapter's max wattage.
		CFTypeRef menu = CFDictionaryGetValue(adapter, CFSTR("UsbHvcMenu"));
		if (menu && CFGetTypeID(menu) == CFArrayGetTypeID()) {
			CFArrayRef arr = (CFArrayRef)menu;
			long long best = 0;
			for (CFIndex i = 0; i < CFArrayGetCount(arr); i++) {
				CFTypeRef e = CFArrayGetValueAtIndex(arr, i);
				if (!e || CFGetTypeID(e) != CFDictionaryGetTypeID())
					continue;
				long long mv = dict_ll((CFDictionaryRef)e, CFSTR("MaxVoltage"), 0);
				long long ma = dict_ll((CFDictionaryRef)e, CFSTR("MaxCurrent"), 0);
				long long uw = mv * ma; // mV * mA = uW
				if (uw > best)
					best = uw;
			}
			out->adapter_max_watts = best / 1000000;
		}
	}

	CFRelease(props);
	return 0;
}
*/
import "C"
import (
	"math"
	"time"
)

// Each IORegistry read copies the entire AppleSmartBattery property dict,
// so USBCPorts/Battery/ACOnline share one snapshot per refresh cycle
// instead of taking three.
type DarwinPowerSource struct {
	info   C.power_info
	infoOK bool
	readAt time.Time
}

func NewPowerSource() PowerSource {
	return &DarwinPowerSource{}
}

func (d *DarwinPowerSource) snapshot() (C.power_info, bool) {
	if !d.readAt.IsZero() && time.Since(d.readAt) < time.Second {
		return d.info, d.infoOK
	}
	d.infoOK = C.read_power(&d.info) == 0
	d.readAt = time.Now()
	return d.info, d.infoOK
}

func (d *DarwinPowerSource) USBCPorts() []USBCPort {
	info, ok := d.snapshot()
	if !ok || info.adapter_present == 0 {
		return nil
	}

	negotiated := float64(info.adapter_watts)
	voltage := float64(info.adapter_voltage_mv) / 1000
	current := float64(info.adapter_current_ma) / 1000
	if negotiated == 0 {
		negotiated = voltage * current
	}
	pdMax := float64(info.adapter_max_watts)
	if pdMax < negotiated {
		pdMax = negotiated
	}

	return []USBCPort{{
		Name:         "USB-C 1",
		Online:       true,
		Voltage:      voltage,
		CurrentMax:   current,
		PDNegotiated: negotiated,
		PDMax:        pdMax,
	}}
}

func (d *DarwinPowerSource) Battery() BatteryInfo {
	info, ok := d.snapshot()
	if !ok {
		return BatteryInfo{Found: false, Status: "Unknown"}
	}

	status := "Discharging"
	if info.external_connected != 0 {
		switch {
		case info.amperage_ma < 0:
			// Underpowered adapter: battery supplies the deficit
			status = "Discharging"
		case info.is_charging != 0:
			status = "Charging"
		case info.fully_charged != 0:
			status = "Full"
		default:
			status = "Not charging"
		}
	}

	// Amperage (mA, signed) * Voltage (mV) = uW
	powerW := math.Abs(float64(info.amperage_ma)*float64(info.voltage_mv)) / 1e6

	capacity := 0
	if info.max_cap > 0 {
		capacity = int(info.cur_cap * 100 / info.max_cap)
	}

	// macOS manages charge thresholds itself (Optimized Battery Charging)
	// and doesn't expose them; empty ChargeStart/ChargeEnd hides the row.
	return BatteryInfo{
		Found:    true,
		Status:   status,
		PowerW:   powerW,
		Capacity: capacity,
	}
}

func (d *DarwinPowerSource) ACOnline() bool {
	info, ok := d.snapshot()
	return ok && info.external_connected != 0
}
