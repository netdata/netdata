// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import "fmt"

// DSP0134 7.18.2 Memory Device: Type.
var memoryTypeNames = map[byte]string{
	3:  "DRAM",
	4:  "EDRAM",
	5:  "VRAM",
	6:  "SRAM",
	7:  "RAM",
	8:  "ROM",
	9:  "FLASH",
	10: "EEPROM",
	11: "FEPROM",
	12: "EPROM",
	13: "CDRAM",
	14: "3DRAM",
	15: "SDRAM",
	16: "SGRAM",
	17: "RDRAM",
	18: "DDR",
	19: "DDR2",
	20: "DDR2 FB-DIMM",
	24: "DDR3",
	25: "FBD2",
	26: "DDR4",
	27: "LPDDR",
	28: "LPDDR2",
	29: "LPDDR3",
	30: "LPDDR4",
	32: "HBM",
	33: "HBM2",
	34: "DDR5",
	35: "LPDDR5",
	36: "HBM3",
}

// DSP0134 7.18.1 Memory Device: Form Factor.
var formFactorNames = map[byte]string{
	3:  "SIMM",
	4:  "SIP",
	5:  "Chip",
	6:  "DIP",
	7:  "ZIP",
	8:  "Proprietary card",
	9:  "DIMM",
	10: "TSOP",
	11: "Row of chips",
	12: "RIMM",
	13: "SODIMM",
	14: "SRIMM",
	15: "FB-DIMM",
	16: "Die",
}

func memoryTypeName(v byte) string { return enumName(v, memoryTypeNames) }
func formFactorName(v byte) string { return enumName(v, formFactorNames) }

// enumName maps the shared SMBIOS enumeration prefix: 00h is not set, 01h is
// Other and 02h is Unknown; the rest is table-specific.
func enumName(v byte, names map[byte]string) string {
	switch v {
	case 0, 2:
		return ""
	case 1:
		return "Other"
	}
	if name, ok := names[v]; ok {
		return name
	}
	return fmt.Sprintf("SMBIOS code 0x%02x", v)
}
