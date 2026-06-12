package main

import (
	"fmt"
	"strings"
)

type DisplayState struct {
	BarLabel    string
	PortLabels  []string
	BatLabel    string
	TotalLabel  string
	ThreshLabel string
}

func ComputeDisplay(ports []USBCPort, bat BatteryInfo, ac bool) DisplayState {

	var labelParts []string
	var totalInput float64
	var portLabels []string

	for _, port := range ports {
		if port.Online {
			totalInput += port.PDNegotiated
			portLabels = append(portLabels, fmt.Sprintf(
				"%s: %.0fW (%.0fV @ %.1fA) [max %.0fW]",
				port.Name, port.PDNegotiated,
				port.Voltage, port.CurrentMax, port.PDMax,
			))
			labelParts = append(labelParts, fmt.Sprintf("%s:%.0fW", port.ShortName, port.PDNegotiated))
		} else {
			portLabels = append(portLabels, fmt.Sprintf("%s: disconnected", port.Name))
		}
	}

	if ac && totalInput == 0 {
		labelParts = append(labelParts, "S:AC")
	}

	var batLabel string
	if !bat.Found {
		batLabel = "Battery: not present"
	} else {
		// Some drivers report "Unknown" while actually draining; show the
		// draw rather than pretending there's no power flow
		draining := bat.Status == statusDischarging ||
			(bat.Status == statusUnknown && !ac && bat.PowerW > 0)
		if draining {
			labelParts = append(labelParts, fmt.Sprintf("BAT:%.1fW", bat.PowerW))
		} else if bat.Status == statusCharging {
			labelParts = append(labelParts, fmt.Sprintf("CHG:%.1fW", bat.PowerW))
		}
		batLabel = fmt.Sprintf("Battery: %d%% | %s | %.1fW", bat.Capacity, bat.Status, bat.PowerW)
	}

	var totalLabel string
	if totalInput > 0 {
		totalLabel = fmt.Sprintf("Power input: %.0fW", totalInput)
	} else if ac {
		totalLabel = "Power input: AC adapter"
	} else {
		totalLabel = "Power input: none"
	}

	var threshLabel string
	if bat.ChargeStart != "" && bat.ChargeEnd != "" {
		threshLabel = fmt.Sprintf("Charge range: %s%% - %s%%", bat.ChargeStart, bat.ChargeEnd)
	}

	barLabel := "No power"
	if len(labelParts) > 0 {
		barLabel = strings.Join(labelParts, "  |  ")
	}

	return DisplayState{
		BarLabel:    barLabel,
		PortLabels:  portLabels,
		BatLabel:    batLabel,
		TotalLabel:  totalLabel,
		ThreshLabel: threshLabel,
	}
}
