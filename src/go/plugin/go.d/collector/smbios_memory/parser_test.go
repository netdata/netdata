// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) ([]byte, []byte) {
	t.Helper()
	e, err := os.ReadFile("testdata/smbios3-entry.bin")
	require.NoError(t, err)
	d, err := os.ReadFile("testdata/system-memory.bin")
	require.NoError(t, err)
	return e, d
}

// This helper only frames the fixture's records; assertions come from the
// supplied server's decoded shape and DSP0134, not from parser output.
func fixtureRecords(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var records [][]byte
	for len(data) > 0 {
		require.GreaterOrEqual(t, len(data), 4)
		n := int(data[1])
		require.LessOrEqual(t, n, len(data))
		end := bytes.Index(data[n:], []byte{0, 0})
		require.GreaterOrEqual(t, end, 0)
		length := n + end + 2
		records = append(records, bytes.Clone(data[:length]))
		data = data[length:]
	}
	return records
}
func entry3(data []byte) []byte {
	e := make([]byte, 24)
	copy(e, "_SM3_")
	e[6] = 24
	e[7] = 3
	e[8] = 3
	e[10] = 1
	binary.LittleEndian.PutUint32(e[12:16], uint32(len(data)+128))
	checksum(e, 5)
	return e
}
func checksum(data []byte, offset int) {
	data[offset] = 0
	var sum byte
	for _, b := range data {
		sum += b
	}
	data[offset] = 0 - sum
}
func joinRecords(records [][]byte) []byte { return bytes.Join(records, nil) }

func TestParseSuppliedServerShape(t *testing.T) {
	e, d := fixture(t)
	table, err := parseTable(e, d)
	require.NoError(t, err)
	assert.True(t, table.Comparable)
	assert.Equal(t, uint64(1<<40), *table.Capacity)
	assert.Equal(t, 16, table.Populated)
	assert.Zero(t, table.Empty)
	require.Len(t, table.Devices, 16)
	first := table.Devices[0]
	assert.Equal(t, "DIMM_P0_A0", first.Locator)
	assert.Equal(t, "BANK 0", first.Bank)
	assert.Equal(t, "DDR4", first.MemoryType)
	assert.Equal(t, "DIMM", first.FormFactor)
	assert.Equal(t, uint64(64<<30), *first.Capacity)
	assert.Equal(t, uint64(2), *first.Ranks)
	assert.Equal(t, uint64(3200), *first.RatedSpeed)
	assert.Equal(t, uint64(3200), *first.ConfiguredSpeed)
}

func TestDocumentedDeviceEncodings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		size       uint16
		extended   uint32
		population string
		capacity   *uint64
	}{
		{"empty", 0, 0, "empty", uintPtr(0)}, {"megabytes", 8192, 0, "populated", uintPtr(8 << 30)},
		{"kilobytes", 0x8001, 0, "populated", uintPtr(1024)}, {"unknown", 0xffff, 0, "unknown", nil},
		{"extended", 0x7fff, 131072, "populated", uintPtr(128 << 30)}, {"unknown extended", 0x7fff, 0, "unknown", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, d := fixture(t)
			records := fixtureRecords(t, d)
			device := records[1]
			binary.LittleEndian.PutUint16(device[12:14], tc.size)
			binary.LittleEndian.PutUint32(device[28:32], tc.extended)
			binary.LittleEndian.PutUint16(device[21:23], 0xffff)
			binary.LittleEndian.PutUint32(device[84:88], 70000)
			binary.LittleEndian.PutUint16(device[32:34], 4800)
			d = joinRecords(records)
			table, err := parseTable(entry3(d), d)
			require.NoError(t, err)
			got := table.Devices[0]
			assert.Equal(t, tc.capacity, got.Capacity)
			assert.Equal(t, tc.population, got.Population)
			assert.Equal(t, uint64(70000), *got.RatedSpeed)
			assert.Equal(t, uint64(4800), *got.ConfiguredSpeed)
			if tc.capacity == nil {
				assert.False(t, table.Comparable)
				assert.Nil(t, table.Capacity)
				assert.False(t, table.CountsKnown)
			}
		})
	}
}

func TestParserClassificationAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		modify     func([][]byte)
		err        string
		devices    int
		comparable bool
	}{
		{"unknown locator", func(r [][]byte) { r[1][16] = 0 }, "", 16, false},
		{"duplicated locator", func(r [][]byte) { r[1][16] = 2; r[2][16] = 2 }, "", 16, false},
		{"bank duplicates allowed", func(r [][]byte) {}, "", 16, true},
		{"logical memory excluded", func(r [][]byte) { r[1][18] = 0x1f; r[1][12] = 0xff; r[1][13] = 0xff }, "", 15, true},
		{"not system memory", func(r [][]byte) { r[0][5] = 4 }, "no system memory", 0, false},
		{"array count mismatch", func(r [][]byte) { r[0][13] = 17 }, "device count", 0, false},
		{"array missing", func(r [][]byte) { r[1][4] = 0x99 }, "missing physical array", 0, false},
		{"duplicate handle", func(r [][]byte) { copy(r[2][2:4], r[1][2:4]) }, "duplicate DMI handle", 0, false},
		{"missing extended size", func(r [][]byte) { r[1] = append(bytes.Clone(r[1][:28]), r[1][92:]...); r[1][1] = 28 }, "extended memory size", 0, false},
		{"optional fields omitted", func(r [][]byte) {
			r[1][12] = 0
			r[1][13] = 0x20
			r[1] = append(bytes.Clone(r[1][:21]), r[1][92:]...)
			r[1][1] = 21
		}, "", 16, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, d := fixture(t)
			r := fixtureRecords(t, d)
			tc.modify(r)
			d = joinRecords(r)
			got, err := parseTable(entry3(d), d)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got.Devices, tc.devices)
			assert.Equal(t, tc.comparable, got.Comparable)
			if tc.name == "optional fields omitted" {
				assert.Nil(t, got.Devices[0].RatedSpeed)
				assert.Nil(t, got.Devices[0].ConfiguredSpeed)
				assert.Empty(t, got.Devices[0].Serial)
			}
		})
	}
}

func TestEntryPointsAndTruncation(t *testing.T) {
	e, d := fixture(t)
	for i := 0; i < len(e); i++ {
		_, err := parseTable(e[:i], d)
		require.Error(t, err)
	}
	for _, cut := range []int{0, 1, 3, 22, len(d) - 1, len(d) - 6} {
		_, err := parseTable(e, d[:cut])
		require.Error(t, err)
	}
	e[5]++
	_, err := parseTable(e, d)
	require.ErrorContains(t, err, "entry point")
	e = make([]byte, 31)
	copy(e, "_SM_")
	e[5] = 31
	e[6] = 2
	e[7] = 8
	copy(e[16:], "_DMI_")
	binary.LittleEndian.PutUint16(e[22:24], uint16(len(d)))
	binary.LittleEndian.PutUint16(e[28:30], 18)
	checksum(e[16:31], 5)
	checksum(e, 4)
	got, err := parseTable(e, d)
	require.NoError(t, err)
	assert.Equal(t, 16, got.Populated)
	e[28]++
	checksum(e[16:31], 5)
	checksum(e, 4)
	_, err = parseTable(e, d)
	require.ErrorContains(t, err, "structure count")
}

func FuzzParseTable(f *testing.F) {
	e, err := os.ReadFile("testdata/smbios3-entry.bin")
	require.NoError(f, err)
	d, err := os.ReadFile("testdata/system-memory.bin")
	require.NoError(f, err)
	f.Add(e, d)
	f.Fuzz(func(t *testing.T, entry, data []byte) { _, _ = parseTable(entry, data) })
}
