// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// Offsets and encodings below are defined by DMTF DSP0134 3.8, sections 5.2
// (entry points), 7.17 (Physical Memory Array) and 7.18 (Memory Device). The
// table is a boot-time firmware description, not a live memory probe.
const (
	structPhysicalMemoryArray = 16
	structMemoryDevice        = 17
	structEndOfTable          = 127

	arrayUseSystemMemory = 3 // Table 73

	deviceSizeUnknown  = 0xffff // 7.18.5: unknown size, not an empty socket
	deviceSizeExtended = 0x7fff // the size is in the Extended Size field
	deviceTypeLogical  = 0x1f   // logical non-volatile device, not physical RAM
)

// record is one SMBIOS structure: its formatted area and its string set.
type record struct {
	data    []byte
	strings []string
}

func (r record) kind() byte       { return r.data[0] }
func (r record) handle() uint16   { return r.u16(2) }
func (r record) u16(o int) uint16 { return binary.LittleEndian.Uint16(r.data[o : o+2]) }
func (r record) u32(o int) uint32 { return binary.LittleEndian.Uint32(r.data[o : o+4]) }

// str resolves the string-set index stored at a formatted-area offset.
func (r record) str(o int) string {
	if o >= len(r.data) || r.data[o] == 0 || int(r.data[o]) > len(r.strings) {
		return ""
	}
	return strings.TrimSpace(r.strings[r.data[o]-1])
}

// speed reads a 16-bit MT/s field, falling back to its 32-bit extended field
// when the 16-bit value is 0xffff. Zero means unknown.
func (r record) speed(o, extended int) *uint64 {
	if len(r.data) < o+2 {
		return nil
	}
	v := uint64(r.u16(o))
	if v == 0xffff {
		if len(r.data) < extended+4 {
			return nil
		}
		v = uint64(r.u32(extended))
	}
	if v == 0 {
		return nil
	}
	return uintPtr(v)
}

type entryPoint struct {
	length uint32 // table length (SMBIOS 2) or maximum table size (SMBIOS 3)
	count  int    // structure count (SMBIOS 2 only)
	v3     bool
}

type memoryArray struct {
	use            byte
	expected, seen int
}

func parseTable(entry, data []byte) (*inventory.Table, error) {
	ep, err := parseEntryPoint(entry)
	if err != nil {
		return nil, err
	}
	records, err := splitRecords(data, ep)
	if err != nil {
		return nil, err
	}
	arrays, err := indexArrays(records)
	if err != nil {
		return nil, err
	}
	table, err := collectDevices(records, arrays)
	if err != nil {
		return nil, err
	}
	if err := classifyArrays(arrays, table); err != nil {
		return nil, err
	}
	return table, nil
}

func parseEntryPoint(entry []byte) (entryPoint, error) {
	validSum := func(data []byte) bool {
		var sum byte
		for _, b := range data {
			sum += b
		}
		return sum == 0
	}
	if bytes.HasPrefix(entry, []byte("_SM3_")) {
		if len(entry) < 24 || int(entry[6]) < 24 || int(entry[6]) > len(entry) || !validSum(entry[:entry[6]]) {
			return entryPoint{}, fmt.Errorf("invalid SMBIOS 3 entry point")
		}
		return entryPoint{length: binary.LittleEndian.Uint32(entry[12:16]), v3: true}, nil
	}
	if bytes.HasPrefix(entry, []byte("_SM_")) {
		if len(entry) < 31 || int(entry[5]) < 31 || int(entry[5]) > len(entry) || !validSum(entry[:entry[5]]) ||
			string(entry[16:21]) != "_DMI_" || !validSum(entry[16:31]) {
			return entryPoint{}, fmt.Errorf("invalid SMBIOS 2 entry point")
		}
		return entryPoint{
			length: uint32(binary.LittleEndian.Uint16(entry[22:24])),
			count:  int(binary.LittleEndian.Uint16(entry[28:30])),
		}, nil
	}
	return entryPoint{}, fmt.Errorf("unsupported SMBIOS entry point")
}

// splitRecords frames the structures and checks them against the entry point.
func splitRecords(data []byte, ep entryPoint) ([]record, error) {
	if uint64(len(data)) > uint64(ep.length) || (!ep.v3 && len(data) != int(ep.length)) {
		return nil, fmt.Errorf("DMI table length disagrees with entry point")
	}
	var records []record
	handles := make(map[uint16]bool)
	ended := false
	for offset := 0; offset < len(data) && !ended; {
		r, n, err := nextRecord(data[offset:])
		if err != nil {
			return nil, err
		}
		if handles[r.handle()] {
			return nil, fmt.Errorf("duplicate DMI handle")
		}
		handles[r.handle()] = true
		records = append(records, r)
		offset += n
		ended = r.kind() == structEndOfTable
	}
	if ep.v3 && !ended {
		return nil, fmt.Errorf("SMBIOS 3 table has no end marker")
	}
	if !ep.v3 && len(records) != ep.count {
		return nil, fmt.Errorf("SMBIOS structure count disagrees with table")
	}
	return records, nil
}

// nextRecord decodes the structure at the start of data and returns its total size.
func nextRecord(data []byte) (record, int, error) {
	if len(data) < 4 {
		return record{}, 0, fmt.Errorf("truncated DMI header")
	}
	n := int(data[1])
	if n < 4 || n > len(data) {
		return record{}, 0, fmt.Errorf("invalid DMI structure length")
	}
	end := bytes.Index(data[n:], []byte{0, 0})
	if end < 0 {
		return record{}, 0, fmt.Errorf("unterminated DMI string set")
	}
	r := record{data: data[:n]}
	if end > 0 {
		r.strings = strings.Split(string(data[n:n+end]), "\x00")
	}
	return r, n + end + 2, nil
}

func indexArrays(records []record) (map[uint16]*memoryArray, error) {
	arrays := make(map[uint16]*memoryArray)
	for _, r := range records {
		if r.kind() != structPhysicalMemoryArray {
			continue
		}
		if len(r.data) < 15 {
			return nil, fmt.Errorf("short physical memory array")
		}
		arrays[r.handle()] = &memoryArray{use: r.data[5], expected: int(r.u16(13))}
	}
	return arrays, nil
}

// collectDevices decodes the physical devices of system-memory arrays. Every
// device is counted against its array; other arrays (video, cache, etc.) and
// logical devices are not physical system RAM and are not inventoried.
func collectDevices(records []record, arrays map[uint16]*memoryArray) (*inventory.Table, error) {
	var b tableBuilder
	for _, r := range records {
		if r.kind() != structMemoryDevice {
			continue
		}
		if len(r.data) < 21 {
			return nil, fmt.Errorf("short memory device")
		}
		a := arrays[r.u16(4)]
		if a == nil {
			return nil, fmt.Errorf("memory device references missing physical array")
		}
		a.seen++
		if a.use != arrayUseSystemMemory || r.data[18] == deviceTypeLogical {
			continue
		}
		d, err := parseDevice(r)
		if err != nil {
			return nil, err
		}
		b.add(d)
	}
	return b.build(), nil
}

func parseDevice(r record) (inventory.Device, error) {
	d := inventory.Device{
		Handle:       r.handle(),
		Locator:      r.str(16),
		Bank:         r.str(17),
		MemoryType:   memoryTypeName(r.data[18]),
		FormFactor:   formFactorName(r.data[14]),
		Manufacturer: r.str(23),
		Serial:       r.str(24),
		Part:         r.str(26),
	}
	var err error
	if d.Capacity, d.Population, err = decodeSize(r); err != nil {
		return inventory.Device{}, err
	}
	if len(r.data) > 27 && r.data[27]&0x0f != 0 {
		d.Ranks = uintPtr(uint64(r.data[27] & 0x0f))
	}
	d.RatedSpeed = r.speed(21, 84)
	d.ConfiguredSpeed = r.speed(32, 88)
	return d, nil
}

// decodeSize applies the Size and Extended Size encodings of 7.18.5.
func decodeSize(r record) (*uint64, string, error) {
	size := r.u16(12)
	switch size {
	case 0:
		return uintPtr(0), inventory.PopulationEmpty, nil
	case deviceSizeUnknown:
		return nil, inventory.PopulationUnknown, nil
	case deviceSizeExtended:
		if len(r.data) < 32 {
			return nil, "", fmt.Errorf("missing extended memory size")
		}
		ext := r.u32(28) & 0x7fffffff
		if ext == 0 {
			return nil, inventory.PopulationUnknown, nil
		}
		return uintPtr(uint64(ext) * 1024 * 1024), inventory.PopulationPopulated, nil
	default:
		unit := uint64(1024 * 1024)
		if size&0x8000 != 0 {
			unit = 1024
		}
		capacity := uint64(size&0x7fff) * unit
		if capacity == 0 {
			// "0 KB" describes neither an empty socket (size 0) nor a device the
			// baseline can accept; it is not comparable evidence.
			return nil, inventory.PopulationUnknown, nil
		}
		return uintPtr(capacity), inventory.PopulationPopulated, nil
	}
}

// tableBuilder accumulates devices into totals and comparability verdicts.
type tableBuilder struct {
	devices          []inventory.Device
	populated, empty int
	total            uint64
	countsUnknown    bool
	capacityUnknown  bool
	reason           string
	slots            map[slotKey]bool
}

func (b *tableBuilder) add(d inventory.Device) {
	switch d.Population {
	case inventory.PopulationEmpty:
		b.empty++
	case inventory.PopulationPopulated:
		b.populated++
	default:
		b.countsUnknown = true
	}
	if d.Capacity == nil {
		b.capacityUnknown = true
		b.reason = "Unknown device capacity"
	} else {
		b.total += *d.Capacity
	}
	if b.slots == nil {
		b.slots = make(map[slotKey]bool)
	}
	slot := slotOf(d)
	if d.Locator == "" || b.slots[slot] {
		b.reason = "Slot locators are missing or duplicated within a bank"
	}
	b.slots[slot] = true
	b.devices = append(b.devices, d)
}

func (b *tableBuilder) build() *inventory.Table {
	t := &inventory.Table{
		Devices:     b.devices,
		Populated:   b.populated,
		Empty:       b.empty,
		CountsKnown: !b.countsUnknown,
		Comparable:  b.reason == "",
		Reason:      b.reason,
	}
	if !b.capacityUnknown {
		t.Capacity = uintPtr(b.total)
	}
	return t
}

// classifyArrays verifies every array's device count and requires at least one
// system-memory array. An array whose use is unknown withholds the table's
// totals: it may still hold system memory.
func classifyArrays(arrays map[uint16]*memoryArray, table *inventory.Table) error {
	systemArrays := 0
	for _, a := range arrays {
		if a.expected != a.seen {
			return fmt.Errorf("physical array device count disagrees with table")
		}
		switch a.use {
		case arrayUseSystemMemory:
			systemArrays++
		case 1, 4, 5, 6, 7:
			// Explicitly other, video, flash, nonvolatile or cache memory.
		default:
			// DSP0134 Table 73: Unknown (02h), reserved or future values do not
			// establish that this array is outside the host's system memory.
			table.Capacity = nil
			table.CountsKnown = false
			table.Comparable = false
			table.Reason = "Physical memory array use is unknown or unsupported"
		}
	}
	if systemArrays == 0 {
		return fmt.Errorf("no system memory array")
	}
	return nil
}

func uintPtr(v uint64) *uint64 { return &v }
