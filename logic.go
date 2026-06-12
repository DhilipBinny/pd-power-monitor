package main

import "fmt"

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

	for i, port := range ports {
		if port.Online {
			totalInput += port.PDNegotiated
			portLabels = append(portLabels, fmt.Sprintf(
				"%s: %.0fW (%.0fV @ %.1fA) [max %.0fW]",
				port.Name, port.PDNegotiated,
				port.Voltage, port.CurrentMax, port.PDMax,
			))
			labelParts = append(labelParts, fmt.Sprintf("C%d:%.0fW", i+1, port.PDNegotiated))
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
		if bat.Status == "Discharging" {
			labelParts = append(labelParts, fmt.Sprintf("BAT:%.1fW", bat.PowerW))
		} else if bat.Status == "Charging" {
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
		barLabel = ""
		for i, p := range labelParts {
			if i > 0 {
				barLabel += "  |  "
			}
			barLabel += p
		}
	}

	return DisplayState{
		BarLabel:    barLabel,
		PortLabels:  portLabels,
		BatLabel:    batLabel,
		TotalLabel:  totalLabel,
		ThreshLabel: threshLabel,
	}
}
