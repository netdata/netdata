// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

func TestParseTable_SuppliedServerShape(t *testing.T) {
	e, d := fixture(t)

	table, err := parseTable(e, d)

	require.NoError(t, err)
	assert.Equal(t, &inventory.Table{
		Devices:     fixtureDevices(),
		Capacity:    uintPtr(1 << 40),
		Populated:   16,
		CountsKnown: true,
		Comparable:  true,
	}, table)
}

// tableSummary is a decoded table without its device list.
func tableSummary(table *inventory.Table) inventory.Table {
	summary := *table
	summary.Devices = nil
	return summary
}

func TestParseTable_DocumentedSizeEncodings(t *testing.T) {
	const fixtureTotal = 16 * (64 << 30)
	withCapacity := func(capacity uint64, population string) inventory.Device {
		d := fixtureDevice(0)
		d.Capacity, d.Population = uintPtr(capacity), population
		return d
	}
	known := func(capacity uint64, population string) inventory.Table {
		empty := 0
		if population == "empty" {
			empty = 1
		}
		return inventory.Table{
			Capacity:    uintPtr(fixtureTotal - 64<<30 + capacity),
			Populated:   16 - empty,
			Empty:       empty,
			CountsKnown: true,
			Comparable:  true,
		}
	}
	unknownDevice := fixtureDevice(0)
	unknownDevice.Capacity, unknownDevice.Population = nil, "unknown"
	unknownTable := inventory.Table{Populated: 15, Reason: "Unknown device capacity"}

	for name, tc := range map[string]struct {
		size     uint16
		extended uint32
		device   inventory.Device
		table    inventory.Table
	}{
		"empty socket": {
			size:   0,
			device: withCapacity(0, "empty"),
			table:  known(0, "empty"),
		},
		"megabytes": {
			size:   8192,
			device: withCapacity(8<<30, "populated"),
			table:  known(8<<30, "populated"),
		},
		"kilobytes": {
			size:   0x8001,
			device: withCapacity(1024, "populated"),
			table:  known(1024, "populated"),
		},
		"extended size": {
			size:     0x7fff,
			extended: 131072,
			device:   withCapacity(128<<30, "populated"),
			table:    known(128<<30, "populated"),
		},
		"unknown size":          {size: 0xffff, device: unknownDevice, table: unknownTable},
		"zero kilobytes":        {size: 0x8000, device: unknownDevice, table: unknownTable},
		"unknown extended size": {size: 0x7fff, extended: 0, device: unknownDevice, table: unknownTable},
	} {
		t.Run(name, func(t *testing.T) {
			_, d := fixture(t)
			records := fixtureRecords(t, d)
			device := records[1]
			binary.LittleEndian.PutUint16(device[12:14], tc.size)
			binary.LittleEndian.PutUint32(device[28:32], tc.extended)
			// Speeds use their extended fields the same way: 0xffff defers to the 32-bit value.
			binary.LittleEndian.PutUint16(device[21:23], 0xffff)
			binary.LittleEndian.PutUint32(device[84:88], 70000)
			binary.LittleEndian.PutUint16(device[32:34], 4800)
			d = joinRecords(records)
			tc.device.RatedSpeed, tc.device.ConfiguredSpeed = uintPtr(70000), uintPtr(4800)

			table, err := parseTable(entry3(d), d)

			require.NoError(t, err)
			require.NotEmpty(t, table.Devices)
			assert.Equal(t, tc.device, table.Devices[0])
			assert.Equal(t, tc.table, tableSummary(table))
		})
	}
}

func TestParseTable_Classification(t *testing.T) {
	healthy := inventory.Table{Capacity: uintPtr(1 << 40), Populated: 16, CountsKnown: true, Comparable: true}
	uncomparable := healthy
	uncomparable.Comparable, uncomparable.Reason = false, "Slot locators are missing or duplicated within a bank"
	unknownType := fixtureDevice(0)
	unknownType.MemoryType = ""
	unnamedSlot := fixtureDevice(0)
	unnamedSlot.Locator = ""
	minimal := fixtureDevice(0)
	minimal.Capacity = uintPtr(8 << 30)
	minimal.Manufacturer, minimal.Part, minimal.Serial = "", "", ""
	minimal.Ranks, minimal.RatedSpeed, minimal.ConfiguredSpeed = nil, nil, nil

	sharedLocator := fixtureDevice(0)
	sharedLocator.Bank, sharedLocator.Locator = "P0 CHANNEL A", "DIMM 0"

	for name, tc := range map[string]struct {
		modify  func([][]byte)
		devices int
		first   inventory.Device
		table   inventory.Table
	}{
		"bank duplicates allowed": {
			modify: func([][]byte) {}, devices: 16, first: fixtureDevice(0), table: healthy,
		},
		"unknown locator": {
			modify: func(r [][]byte) { r[1][16] = 0 }, devices: 16, first: unnamedSlot, table: uncomparable,
		},
		"duplicated locator": {
			modify: func(r [][]byte) { r[1][16], r[2][16] = 2, 2 }, devices: 16, table: uncomparable,
			first: func() inventory.Device { d := fixtureDevice(0); d.Locator = "BANK 0"; return d }(),
		},
		"locator repeated across banks": {
			modify:  func(r [][]byte) { copy(r, sharedLocatorRecords(t, joinRecords(r))) },
			devices: 16, first: sharedLocator, table: healthy,
		},
		"locator repeated within a bank": {
			modify: func(r [][]byte) {
				copy(r, sharedLocatorRecords(t, joinRecords(r)))
				r[2] = relabel(t, r[2], "P0 CHANNEL A", "DIMM 0")
			},
			devices: 16, first: sharedLocator, table: uncomparable,
		},
		"locator without bank repeated": {
			modify: func(r [][]byte) {
				copy(r, sharedLocatorRecords(t, joinRecords(r)))
				r[1][17], r[3][17] = 0, 0
			},
			devices: 16, table: uncomparable,
			first: func() inventory.Device { d := sharedLocator; d.Bank = ""; return d }(),
		},
		"unknown technology still physical": {
			modify: func(r [][]byte) { r[1][18] = 2 }, devices: 16, first: unknownType, table: healthy,
		},
		"logical memory excluded": {
			modify: func(r [][]byte) {
				r[1][18] = 0x1f
				binary.LittleEndian.PutUint16(r[1][12:14], 0xffff)
			},
			devices: 15,
			first:   fixtureDevice(1),
			table:   inventory.Table{Capacity: uintPtr(15 * (64 << 30)), Populated: 15, CountsKnown: true, Comparable: true},
		},
		"optional fields omitted": {
			modify: func(r [][]byte) {
				binary.LittleEndian.PutUint16(r[1][12:14], 0x2000)
				r[1] = append(bytes.Clone(r[1][:21]), r[1][92:]...)
				r[1][1] = 21
			},
			devices: 16,
			first:   minimal,
			table:   inventory.Table{Capacity: uintPtr(15*(64<<30) + 8<<30), Populated: 16, CountsKnown: true, Comparable: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, d := fixture(t)
			r := fixtureRecords(t, d)
			tc.modify(r)
			d = joinRecords(r)

			table, err := parseTable(entry3(d), d)

			require.NoError(t, err)
			assert.Len(t, table.Devices, tc.devices)
			assert.Equal(t, tc.first, table.Devices[0])
			assert.Equal(t, tc.table, tableSummary(table))
		})
	}
}

func TestParseTable_MalformedTables(t *testing.T) {
	for name, tc := range map[string]struct {
		modify func([][]byte)
		err    string
	}{
		"not system memory":     {func(r [][]byte) { r[0][5] = 4 }, "no system memory"},
		"array count mismatch":  {func(r [][]byte) { r[0][13] = 17 }, "device count"},
		"array missing":         {func(r [][]byte) { r[1][4] = 0x99 }, "missing physical array"},
		"duplicate handle":      {func(r [][]byte) { copy(r[2][2:4], r[1][2:4]) }, "duplicate DMI handle"},
		"missing extended size": {func(r [][]byte) { r[1] = append(bytes.Clone(r[1][:28]), r[1][92:]...); r[1][1] = 28 }, "extended memory size"},
	} {
		t.Run(name, func(t *testing.T) {
			_, d := fixture(t)
			r := fixtureRecords(t, d)
			tc.modify(r)
			d = joinRecords(r)

			_, err := parseTable(entry3(d), d)

			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestParseTable_EntryPoints(t *testing.T) {
	for name, tc := range map[string]struct {
		entry func(data []byte) []byte
		err   string
	}{
		"smbios 3":                 {entry: entry3},
		"smbios 2":                 {entry: func(d []byte) []byte { return entry2(d, 18) }},
		"smbios 2 structure count": {entry: func(d []byte) []byte { return entry2(d, 19) }, err: "structure count"},
		"corrupt checksum": {
			entry: func(d []byte) []byte { e := entry3(d); e[5]++; return e },
			err:   "entry point",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, d := fixture(t)

			table, err := parseTable(tc.entry(d), d)

			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 16, table.Populated)
		})
	}
}

func TestParseTable_Truncation(t *testing.T) {
	e, d := fixture(t)
	for i := range len(e) {
		_, err := parseTable(e[:i], d)
		assert.Error(t, err, "entry point truncated to %d bytes", i)
	}
	for _, cut := range []int{0, 1, 3, 22, len(d) - 1, len(d) - 6} {
		_, err := parseTable(e, d[:cut])
		assert.Error(t, err, "table truncated to %d bytes", cut)
	}
}

func FuzzParseTable(f *testing.F) {
	e, err := os.ReadFile("testdata/smbios3-entry.bin")
	require.NoError(f, err)
	d, err := os.ReadFile("testdata/system-memory.bin")
	require.NoError(f, err)
	f.Add(e, d)
	f.Fuzz(func(t *testing.T, entry, data []byte) { _, _ = parseTable(entry, data) })
}
