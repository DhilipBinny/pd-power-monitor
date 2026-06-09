package main

type USBCPort struct {
	Name         string
	Online       bool
	Voltage      float64
	CurrentMax   float64
	PDNegotiated float64
	PDMax        float64
}

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
