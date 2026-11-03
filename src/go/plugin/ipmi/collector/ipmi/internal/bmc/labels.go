// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	componentOther = "Other"
	// sensorTypeOEMFirst is the first sensor type code of the OEM range.
	sensorTypeOEMFirst = 0xc0
)

// sensorTypeLabel is the C collector's name of an IPMI sensor type and the
// component it forces regardless of the sensor name, if any.
type sensorTypeLabel struct {
	name      string
	component string
}

// sensorTypeLabels follows freeipmi_plugin.c, keyed by sensor type code.
var sensorTypeLabels = map[uint8]sensorTypeLabel{
	0:  {"Reserved", ""},
	1:  {"Temperature", ""},
	2:  {"Voltage", ""},
	3:  {"Current", ""},
	4:  {"Fan", ""},
	5:  {"Physical Security", "System"},
	6:  {"Platform Security Violation Attempt", "System"},
	7:  {"Processor", "Processor"},
	8:  {"Power Supply", "Power Supply"},
	9:  {"Power Unit", "Power Supply"},
	10: {"Cooling Device", "System"},
	11: {"Other Units Based Sensor", ""},
	12: {"Memory", "Memory"},
	13: {"Drive Slot", "Storage"},
	14: {"POST Memory Resize", "Memory"},
	15: {"System Firmware Progress", "System"},
	16: {"Event Logging Disabled", "System"},
	17: {"Watchdog 1", "System"},
	18: {"System Event", "System"},
	19: {"Critical Interrupt", ""},
	20: {"Button/Switch", "System"},
	21: {"Module/Board", ""},
	22: {"Microcontroller/Coprocessor", "Processor"},
	23: {"Add In Card", ""},
	24: {"Chassis", "System"},
	25: {"Chip Set", "System"},
	26: {"Other Fru", ""},
	27: {"Cable/Interconnect", ""},
	28: {"Terminator", ""},
	29: {"System Boot Initiated", "System"},
	30: {"Boot Error", "System"},
	31: {"OS Boot", "System"},
	32: {"OS Critical Stop", "System"},
	33: {"Slot/Connector", ""},
	34: {"System ACPI Power State", ""},
	35: {"Watchdog 2", "System"},
	36: {"Platform Alert", "System"},
	37: {"Entity Presence", ""},
	38: {"Monitor ASIC/IC", ""},
	39: {"LAN", "Network"},
	40: {"Management Subsystem Health", "System"},
	41: {"Battery", ""},
	42: {"Session Audit", ""},
	43: {"Version Change", ""},
	44: {"FRU State", ""},
}

type componentPattern struct {
	match     matcher.Matcher
	component string
}

// componentPatterns are tried in order against the upper-cased sensor name.
// Order and case-insensitivity are part of the C collector's label contract.
var componentPatterns = []componentPattern{
	newComponentPattern("*DIMM* *_DIM* *VTT* *VDDQ* *ECC* *MEM*CRC* *MEM*BD*", "Memory Module"),
	newComponentPattern("*CPU* SOC_* *VDDCR* P*_VDD* *_DTS *VCORE* *PROC*", "Processor"),
	newComponentPattern("IPU*", "Image Processor"),
	newComponentPattern("M2_* *SSD* *HSC* *HDD* *NVME*", "Storage"),
	newComponentPattern("MB_* *PCH* *VBAT* *I/O*BD* *IO*BD*", "Motherboard"),
	newComponentPattern("WATCHDOG SEL SYS_* *CHASSIS*", "System"),
	newComponentPattern("PS* P_* *PSU* *PWR* *TERMV* *D2D*", "Power Supply"),
	newComponentPattern("VR_P* *VRMP*", "Processor"),
	newComponentPattern("*VSB* *PS*", "Power Supply"),
	newComponentPattern("*MEM* *MEM*RAID*", "Memory"),
	newComponentPattern("*RAID*", "Storage"),
	newComponentPattern("*PERIPHERAL* *USB*", "Peripheral"),
	newComponentPattern("*FAN* *12V* *VCC* *PCI* *CHIPSET* *AMP* *BD*", "System"),
}

func newComponentPattern(patterns, component string) componentPattern {
	return componentPattern{
		match:     matcher.Must(matcher.NewSimplePatternsMatcher(patterns)),
		component: component,
	}
}

// sensorTypeAndComponent returns the type and component labels of a sensor.
// A component forced by the sensor type wins over the name patterns.
func sensorTypeAndComponent(sensorType uint8, name string) (string, string) {
	component := componentOther
	upper := strings.ToUpper(name)
	for _, p := range componentPatterns {
		if p.match.MatchString(upper) {
			component = p.component
			break
		}
	}

	label, ok := sensorTypeLabels[sensorType]
	switch {
	case ok:
		if label.component != "" {
			component = label.component
		}
		return label.name, component
	case sensorType >= sensorTypeOEMFirst:
		return "OEM", component
	default:
		return "Unrecognized", component
	}
}
