// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"strings"
)

// Type labels and fixed component overrides follow freeipmi_plugin.c.
var typeLabels = map[uint8][2]string{
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

// Order and case-insensitivity are part of the C collector's label contract.
var componentPatterns = []struct {
	pattern matcher.Matcher
	label   string
}{
	{matcher.Must(matcher.NewSimplePatternsMatcher("*DIMM* *_DIM* *VTT* *VDDQ* *ECC* *MEM*CRC* *MEM*BD*")), "Memory Module"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*CPU* SOC_* *VDDCR* P*_VDD* *_DTS *VCORE* *PROC*")), "Processor"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("IPU*")), "Image Processor"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("M2_* *SSD* *HSC* *HDD* *NVME*")), "Storage"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("MB_* *PCH* *VBAT* *I/O*BD* *IO*BD*")), "Motherboard"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("WATCHDOG SEL SYS_* *CHASSIS*")), "System"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("PS* P_* *PSU* *PWR* *TERMV* *D2D*")), "Power Supply"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("VR_P* *VRMP*")), "Processor"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*VSB* *PS*")), "Power Supply"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*MEM* *MEM*RAID*")), "Memory"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*RAID*")), "Storage"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*PERIPHERAL* *USB*")), "Peripheral"},
	{matcher.Must(matcher.NewSimplePatternsMatcher("*FAN* *12V* *VCC* *PCI* *CHIPSET* *AMP* *BD*")), "System"},
}

func sensorLabels(kind uint8, name string) (string, string) {
	component := "Other"
	name = strings.ToUpper(name)
	for _, p := range componentPatterns {
		if p.pattern.MatchString(name) {
			component = p.label
			break
		}
	}
	if v, ok := typeLabels[kind]; ok {
		if v[1] != "" {
			component = v[1]
		}
		return v[0], component
	}
	if kind >= 0xc0 {
		return "OEM", component
	}
	return "Unrecognized", component
}
