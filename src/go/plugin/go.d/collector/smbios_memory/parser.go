// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// These offsets and encodings are defined by DSP0134 3.8, sections 5.2, 7.17 and 7.18.
// The table is a boot-time firmware description, not a live memory probe.
func parseTable(entry, data []byte) (*inventory.Table, error) {
	length, count, v3, err := tableExtent(entry)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > uint64(length) || (!v3 && len(data) != int(length)) {
		return nil, fmt.Errorf("DMI table length disagrees with entry point")
	}
	type record struct {
		data    []byte
		strings []string
	}
	var records []record
	handles := make(map[uint16]bool)
	ended := false
	for offset := 0; offset < len(data); {
		if len(data)-offset < 4 {
			return nil, fmt.Errorf("truncated DMI header")
		}
		n := int(data[offset+1])
		if n < 4 || n > len(data)-offset {
			return nil, fmt.Errorf("invalid DMI structure length")
		}
		end := bytes.Index(data[offset+n:], []byte{0, 0})
		if end < 0 {
			return nil, fmt.Errorf("unterminated DMI string set")
		}
		d := data[offset : offset+n]
		h := binary.LittleEndian.Uint16(d[2:4])
		if handles[h] {
			return nil, fmt.Errorf("duplicate DMI handle")
		}
		handles[h] = true
		var ss []string
		if end > 0 {
			ss = strings.Split(string(data[offset+n:offset+n+end]), "\x00")
		}
		records = append(records, record{d, ss})
		offset += n + end + 2
		if d[0] == 127 {
			ended = true
			break
		}
	}
	if v3 && !ended {
		return nil, fmt.Errorf("SMBIOS 3 table has no end marker")
	}
	if !v3 && len(records) != count {
		return nil, fmt.Errorf("SMBIOS structure count disagrees with table")
	}

	type array struct {
		use            byte
		expected, seen int
	}
	arrays := make(map[uint16]*array)
	for _, r := range records {
		if r.data[0] != 16 {
			continue
		}
		if len(r.data) < 15 {
			return nil, fmt.Errorf("short physical memory array")
		}
		arrays[binary.LittleEndian.Uint16(r.data[2:4])] = &array{
			use:      r.data[5],
			expected: int(binary.LittleEndian.Uint16(r.data[13:15])),
		}
	}
	out := &inventory.Table{
		Comparable:  true,
		CountsKnown: true,
	}
	var total uint64
	knownCapacity := true
	locators := make(map[string]bool)
	for _, r := range records {
		d := r.data
		if d[0] != 17 {
			continue
		}
		if len(d) < 21 {
			return nil, fmt.Errorf("short memory device")
		}
		a := arrays[binary.LittleEndian.Uint16(d[4:6])]
		if a == nil {
			return nil, fmt.Errorf("memory device references missing physical array")
		}
		a.seen++
		// Other arrays (video, cache, etc.) and logical devices are not physical system RAM.
		if a.use != 3 || d[18] == 0x1f {
			continue
		}
		str := func(offset int) string {
			if offset >= len(d) || d[offset] == 0 || int(d[offset]) > len(r.strings) {
				return ""
			}
			return strings.TrimSpace(r.strings[d[offset]-1])
		}
		size := binary.LittleEndian.Uint16(d[12:14])
		device := inventory.Device{
			Handle:       binary.LittleEndian.Uint16(d[2:4]),
			Locator:      str(16),
			Bank:         str(17),
			Population:   "populated",
			MemoryType:   memoryType(d[18]),
			FormFactor:   formFactor(d[14]),
			Manufacturer: str(23),
			Serial:       str(24),
			Part:         str(26),
		}
		switch size {
		case 0:
			device.Population = "empty"
			device.Capacity = uintPtr(0)
		case 0xffff:
			// Unknown size does not mean an empty socket (7.18.5).
			device.Capacity = nil
			device.Population = "unknown"
		case 0x7fff:
			if len(d) < 32 {
				return nil, fmt.Errorf("missing extended memory size")
			}
			ext := binary.LittleEndian.Uint32(d[28:32]) & 0x7fffffff
			if ext != 0 {
				device.Capacity = uintPtr(uint64(ext) * 1024 * 1024)
			} else {
				device.Population = "unknown"
			}
		default:
			unit := uint64(1024 * 1024)
			if size&0x8000 != 0 {
				unit = 1024
			}
			device.Capacity = uintPtr(uint64(size&0x7fff) * unit)
		}
		if len(d) > 27 && d[27]&0x0f != 0 {
			device.Ranks = uintPtr(uint64(d[27] & 0x0f))
		}
		device.RatedSpeed = memorySpeed(d, 21, 84)
		device.ConfiguredSpeed = memorySpeed(d, 32, 88)
		if device.Population == "empty" {
			out.Empty++
		} else if device.Population == "populated" {
			out.Populated++
		} else {
			out.CountsKnown = false
		}
		if device.Capacity == nil {
			knownCapacity = false
			out.Comparable = false
			out.Reason = "Unknown device capacity"
		} else {
			total += *device.Capacity
		}
		if device.Locator == "" || locators[device.Locator] {
			out.Comparable = false
			out.Reason = "Slot locators are missing or duplicated"
		}
		locators[device.Locator] = true
		out.Devices = append(out.Devices, device)
	}
	systemArrays := 0
	for _, a := range arrays {
		if a.expected != a.seen {
			return nil, fmt.Errorf("physical array device count disagrees with table")
		}
		if a.use == 3 {
			systemArrays++
		}
	}
	if systemArrays == 0 {
		return nil, fmt.Errorf("no system memory array")
	}
	if knownCapacity {
		out.Capacity = uintPtr(total)
	}
	return out, nil
}

func tableExtent(entry []byte) (uint32, int, bool, error) {
	validSum := func(data []byte) bool {
		var sum byte
		for _, b := range data {
			sum += b
		}
		return sum == 0
	}
	if bytes.HasPrefix(entry, []byte("_SM3_")) {
		if len(entry) < 24 || int(entry[6]) < 24 || int(entry[6]) > len(entry) || !validSum(entry[:entry[6]]) {
			return 0, 0, true, fmt.Errorf("invalid SMBIOS 3 entry point")
		}
		return binary.LittleEndian.Uint32(entry[12:16]), 0, true, nil
	}
	if bytes.HasPrefix(entry, []byte("_SM_")) {
		if len(entry) < 31 || int(entry[5]) < 31 || int(entry[5]) > len(entry) || !validSum(entry[:entry[5]]) ||
			string(entry[16:21]) != "_DMI_" || !validSum(entry[16:31]) {
			return 0, 0, false, fmt.Errorf("invalid SMBIOS 2 entry point")
		}
		return uint32(
				binary.LittleEndian.Uint16(entry[22:24]),
			), int(
				binary.LittleEndian.Uint16(entry[28:30]),
			), false, nil
	}
	return 0, 0, false, fmt.Errorf("unsupported SMBIOS entry point")
}

func uintPtr(v uint64) *uint64 { return &v }

func memorySpeed(data []byte, offset, extended int) *uint64 {
	if len(data) < offset+2 {
		return nil
	}
	v := uint64(binary.LittleEndian.Uint16(data[offset : offset+2]))
	if v == 0xffff {
		if len(data) < extended+4 {
			return nil
		}
		v = uint64(binary.LittleEndian.Uint32(data[extended : extended+4]))
	}
	if v == 0 {
		return nil
	}
	return uintPtr(v)
}

func memoryType(v byte) string {
	return enumName(
		v,
		map[byte]string{
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
		},
	)
}
func formFactor(v byte) string {
	return enumName(
		v,
		map[byte]string{
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
		},
	)
}
func enumName(v byte, names map[byte]string) string {
	if v == 0 || v == 2 {
		return ""
	}
	if v == 1 {
		return "Other"
	}
	if name, ok := names[v]; ok {
		return name
	}
	return fmt.Sprintf("SMBIOS code 0x%02x", v)
}
