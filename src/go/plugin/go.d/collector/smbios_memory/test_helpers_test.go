// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

// A fixed clock makes snapshot timestamps exact.
var fixtureTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// The synthetic fixture (testdata/README.md): sixteen populated 64 GiB DDR4 DIMMs
// with two ranks at 3200 MT/s. These literals are the oracle for parser and
// Function rows; they are documented fixture values, not parser output.
func fixtureDevice(i int) inventory.Device {
	return inventory.Device{
		Handle:          uint16(0x110 + i),
		Locator:         fmt.Sprintf("DIMM_P%d_%c0", i/8, 'A'+i),
		Bank:            "BANK 0",
		Population:      "populated",
		Capacity:        uintPtr(64 << 30),
		MemoryType:      "DDR4",
		FormFactor:      "DIMM",
		Manufacturer:    "Example Memory",
		Part:            "EXAMPLE-64G",
		Serial:          fmt.Sprintf("SYNTHETIC-%02d", i),
		Ranks:           uintPtr(2),
		RatedSpeed:      uintPtr(3200),
		ConfiguredSpeed: uintPtr(3200),
	}
}

func fixtureDevices() []inventory.Device {
	devices := make([]inventory.Device, 16)
	for i := range devices {
		devices[i] = fixtureDevice(i)
	}
	return devices
}

func fixture(t *testing.T) (entry, data []byte) {
	t.Helper()
	entry, err := os.ReadFile("testdata/smbios3-entry.bin")
	require.NoError(t, err)
	data, err = os.ReadFile("testdata/system-memory.bin")
	require.NoError(t, err)
	return entry, data
}

// fixtureRecords only frames the fixture's records so tests can mutate them;
// assertions come from the documented fixture shape and DSP0134.
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

func joinRecords(records [][]byte) []byte { return bytes.Join(records, nil) }

// entry3 builds a checksummed SMBIOS 3 entry point whose maximum size exceeds the table.
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

// entry2 builds a checksummed SMBIOS 2 entry point with an exact length and structure count.
func entry2(data []byte, count uint16) []byte {
	e := make([]byte, 31)
	copy(e, "_SM_")
	e[5] = 31
	e[6] = 2
	e[7] = 8
	copy(e[16:], "_DMI_")
	binary.LittleEndian.PutUint16(e[22:24], uint16(len(data)))
	binary.LittleEndian.PutUint16(e[28:30], count)
	checksum(e[16:31], 5)
	checksum(e, 4)
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

// fixtureHost is a private host prefix with DMI tables and a varlib directory.
type fixtureHost struct {
	root, tables, statePath string
	data                    []byte
}

func newFixtureHost(t *testing.T) *fixtureHost {
	t.Helper()
	_, data := fixture(t)
	root := t.TempDir()
	tables := filepath.Join(root, "sys/firmware/dmi/tables")
	require.NoError(t, os.MkdirAll(tables, 0700))
	stateDir := filepath.Join(root, "varlib")
	require.NoError(t, os.Mkdir(stateDir, 0700))
	h := &fixtureHost{
		root:      root,
		tables:    tables,
		statePath: filepath.Join(stateDir, stateFileName),
		data:      data,
	}
	t.Setenv("NETDATA_HOST_PREFIX", root)
	h.write(t, data)
	return h
}

func (h *fixtureHost) write(t *testing.T, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(h.tables, "smbios_entry_point"), entry3(data), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(h.tables, "DMI"), data, 0600))
}

func (h *fixtureHost) collector(t *testing.T) *Collector {
	t.Helper()
	c := New()
	c.baseline.path = h.statePath
	c.baseline.owner = "fixture-agent"
	c.now = func() time.Time { return fixtureTime }
	c.isTerminal = func() bool { return false }
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c
}

func cycle(t *testing.T, c *Collector) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cc := managed.CycleController()
	cc.BeginCycle()
	if err := c.Collect(t.Context()); err != nil {
		cc.AbortCycle()
		require.NoError(t, err)
	}
	require.NoError(t, cc.CommitCycleSuccess())
}

func diskState(t *testing.T, h *fixtureHost) []byte {
	t.Helper()
	data, err := os.ReadFile(h.statePath)
	require.NoError(t, err)
	return data
}

// collectionResult is the complete measurement and slot-comparison outcome of a
// cycle. Descriptive inventory fields are checked through the Function's rows.
type collectionResult struct {
	Inventory     string
	Comparison    string
	ConfirmedLoss string
	Values        map[string]float64
	Baseline      map[string]uint64
	Loss          []discrepancy
}

func collectionOutput(t *testing.T, c *Collector) collectionResult {
	t.Helper()
	reader := c.store.Read()
	activeStates := func(name string) string {
		point, ok := reader.StateSet(name, nil)
		require.True(t, ok, name)
		var active []string
		for state, enabled := range point.States {
			if enabled {
				active = append(active, state)
			}
		}
		sort.Strings(active)
		return strings.Join(active, ",")
	}
	got := collectionResult{
		Inventory:     activeStates("inventory_status"),
		Comparison:    activeStates("comparison_status"),
		ConfirmedLoss: activeStates("confirmed_loss_status"),
	}
	reader.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
		require.Zero(t, labels.Len(), "host inventory metrics have no device labels")
		if got.Values == nil {
			got.Values = make(map[string]float64)
		}
		got.Values[name] = value
	})
	if state := c.baseline.current; state != nil {
		got.Baseline = make(map[string]uint64)
		for _, device := range state.Baseline {
			got.Baseline[device.Locator] = *device.Capacity
		}
		got.Loss = state.Loss
	}
	return got
}

func expectedBaseline() map[string]uint64 {
	baseline := make(map[string]uint64)
	for _, d := range fixtureDevices() {
		baseline[d.Locator] = *d.Capacity
	}
	return baseline
}

func healthyCollection() collectionResult {
	return collectionResult{
		Inventory:     "available",
		Comparison:    "comparable",
		ConfirmedLoss: "absent",
		Values: map[string]float64{
			"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0,
			"missing_devices": 0, "capacity_deficit_bytes": 0,
		},
		Baseline: expectedBaseline(),
	}
}

// expectedFunctionRows renders the fixture devices as the Function reports them.
func expectedFunctionRows(baselineOnly bool) [][]any {
	var rows [][]any
	for i, d := range fixtureDevices() {
		row := []any{
			fmt.Sprintf("%d:%04x", i, d.Handle), d.Locator, d.Bank,
			d.Population, *d.Capacity, *d.Capacity, "unchanged", "current firmware table",
			d.MemoryType, d.FormFactor, d.Manufacturer, d.Part, d.Serial,
			*d.Ranks, *d.RatedSpeed, *d.ConfiguredSpeed, "unavailable", fixtureTime.UnixMilli(), "available", nil,
		}
		if baselineOnly {
			row[3], row[4] = "unknown", nil
			row[6], row[7] = "unavailable", "baseline only; current data unavailable"
			row[13], row[14], row[15] = nil, nil, nil
			row[18], row[19] = "unavailable", fixtureTime.UnixMilli()
			if i == 0 {
				row[6] = "retained loss; unavailable"
			}
		}
		rows = append(rows, row)
	}
	return rows
}
