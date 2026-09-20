// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_Check(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(*testing.T, *fixtureHost)
		wantErr bool
	}{
		"readable source": {},
		"invalid source without state": {
			prepare: func(t *testing.T, h *fixtureHost) { h.write(t, []byte{17, 92}) },
			wantErr: true,
		},
		"existing state keeps source failures observable": {
			prepare: func(t *testing.T, h *fixtureHost) {
				c := h.collector(t)
				cycle(t, c)
				c.Cleanup(t.Context())
				h.write(t, []byte{17, 92})
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFixtureHost(t)
			if tc.prepare != nil {
				tc.prepare(t, h)
			}
			before, beforeErr := os.ReadFile(h.statePath)
			c := h.collector(t)
			if tc.wantErr {
				require.Error(t, c.Check(t.Context()))
			} else {
				require.NoError(t, c.Check(t.Context()))
			}
			after, afterErr := os.ReadFile(h.statePath)
			assert.Equal(t, before, after)
			assert.Equal(t, os.IsNotExist(beforeErr), os.IsNotExist(afterErr))
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	missing := healthyCollection()
	missing.ConfirmedLoss = "present"
	missing.Values = map[string]float64{
		"installed_capacity_bytes": 15 * (64 << 30), "populated_slots": 15, "empty_slots": 0,
		"missing_devices": 1, "capacity_deficit_bytes": 64 << 30,
	}
	missing.Loss = []discrepancy{{Locator: "DIMM_P0_A0", Missing: true, Deficit: 64 << 30}}
	offset := healthyCollection()
	offset.ConfirmedLoss = "present"
	offset.Values = map[string]float64{
		"installed_capacity_bytes": 1 << 40, "populated_slots": 15, "empty_slots": 1,
		"missing_devices": 1, "capacity_deficit_bytes": 64 << 30,
	}
	offset.Loss = missing.Loss
	growth := healthyCollection()
	growth.Values["installed_capacity_bytes"] += 64 << 30
	growth.Baseline["DIMM_P0_A0"] = 128 << 30
	reduced := healthyCollection()
	reduced.ConfirmedLoss = "present"
	reduced.Values["capacity_deficit_bytes"] = 64 << 30
	reduced.Baseline["DIMM_P0_A0"] = 128 << 30
	reduced.Loss = []discrepancy{{Locator: "DIMM_P0_A0", Deficit: 64 << 30}}
	reset := healthyCollection()
	reset.Values["installed_capacity_bytes"] = 15 * (64 << 30)
	reset.Values["populated_slots"] = 15
	delete(reset.Baseline, "DIMM_P0_A0")

	for name, tc := range map[string]struct {
		prepare       func(*testing.T, *fixtureHost, *Collector) *Collector
		want          collectionResult
		unchangedFile bool
	}{
		"unchanged inventory": {want: healthyCollection(), unchangedFile: true},
		"empty slot cannot be offset by growth elsewhere": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint16(r[1][12:14], 0)
				binary.LittleEndian.PutUint32(r[2][28:32], 131072)
				h.write(t, joinRecords(r))
				return c
			},
			want: offset,
		},
		"absent device with consistent array count": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				r = append(r[:1], r[2:]...)
				r[0][13] = 15
				h.write(t, joinRecords(r))
				return c
			},
			want: missing,
		},
		"pure growth becomes the accepted baseline": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint32(r[1][28:32], 131072)
				h.write(t, joinRecords(r))
				return c
			},
			want: growth,
		},
		"record order and handles do not change identity": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				r[1], r[2] = r[2], r[1]
				r[1][2] = 0x80
				h.write(t, joinRecords(r))
				return c
			},
			want: healthyCollection(), unchangedFile: true,
		},
		"duplicate locators do not confirm loss": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				r[1][16], r[2][16] = 2, 2
				h.write(t, joinRecords(r))
				return c
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "uncomparable",
				ConfirmedLoss: "unknown",
				Values:        map[string]float64{"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0},
				Baseline:      expectedBaseline(),
			},
			unchangedFile: true,
		},
		"capacity reduction after restart uses the accepted increase": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint32(r[1][28:32], 131072)
				h.write(t, joinRecords(r))
				cycle(t, c)
				c.Cleanup(t.Context())
				h.write(t, h.data)
				return h.collector(t)
			},
			want: reduced,
		},
		"restoration clears loss without accepting offsetting growth": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint16(r[1][12:14], 0)
				binary.LittleEndian.PutUint32(r[2][28:32], 131072)
				h.write(t, joinRecords(r))
				cycle(t, c)
				h.write(t, h.data)
				return c
			},
			want: healthyCollection(), unchangedFile: true,
		},
		"stopped collector reset learns the reduced inventory": {
			prepare: func(t *testing.T, h *fixtureHost, c *Collector) *Collector {
				r := fixtureRecords(t, h.data)
				r = append(r[:1], r[2:]...)
				r[0][13] = 15
				h.write(t, joinRecords(r))
				cycle(t, c)
				c.Cleanup(t.Context())
				require.NoError(t, os.Rename(h.statePath, h.statePath+".backup"))
				return h.collector(t)
			},
			want: reset,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFixtureHost(t)
			c := h.collector(t)
			cycle(t, c)
			before := diskState(t, h)
			if tc.prepare != nil {
				c = tc.prepare(t, h, c)
			}
			cycle(t, c)
			assert.Equal(t, tc.want, collectionOutput(t, c))
			if tc.unchangedFile {
				assert.Equal(t, before, diskState(t, h))
			}
		})
	}
}

func TestBaselineFilePermissionsAndUnchangedPoll(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	before := diskState(t, h)
	info, err := os.Stat(h.statePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	cycle(t, c)
	after, err := os.Stat(h.statePath)
	require.NoError(t, err)
	assert.Equal(t, before, diskState(t, h))
	assert.Equal(t, info.ModTime(), after.ModTime())
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestPersistenceFailureRetainsLossAndRetries(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	dir := filepath.Dir(h.statePath)
	loss := []discrepancy{{Locator: "DIMM_P0_A0", Missing: true, Deficit: 64 << 30}}
	// These steps deliberately share state: persistence failures, retry and restart
	// must be exercised in order through the real collector and filesystem.
	for _, step := range []struct {
		name    string
		prepare func(*testing.T)
		want    collectionResult
	}{
		{
			name: "new loss survives a failed commit",
			prepare: func(t *testing.T) {
				require.NoError(t, os.Rename(dir, dir+".offline"))
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint16(r[1][12:14], 0)
				h.write(t, joinRecords(r))
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "state_error",
				ConfirmedLoss: "present",
				Values:        map[string]float64{"installed_capacity_bytes": 15 * (64 << 30), "populated_slots": 15, "empty_slots": 1},
				Baseline:      expectedBaseline(),
				Loss:          loss,
			},
		},
		{
			name:    "source failure does not discard uncommitted loss",
			prepare: func(t *testing.T) { h.write(t, []byte{17, 92}) },
			want: collectionResult{
				Inventory:     "unavailable",
				Comparison:    "state_error",
				ConfirmedLoss: "present",
				Baseline:      expectedBaseline(),
				Loss:          loss,
			},
		},
		{
			name:    "retry persists loss even while source remains unavailable",
			prepare: func(t *testing.T) { require.NoError(t, os.Rename(dir+".offline", dir)) },
			want: collectionResult{
				Inventory:     "unavailable",
				Comparison:    "unavailable",
				ConfirmedLoss: "present",
				Baseline:      expectedBaseline(),
				Loss:          loss,
			},
		},
		{
			name: "restart recovers committed loss",
			prepare: func(stepT *testing.T) {
				c.Cleanup(stepT.Context())
				// The replacement must remain alive for the following transition steps.
				c = h.collector(t)
			},
			want: collectionResult{
				Inventory:     "unavailable",
				Comparison:    "unavailable",
				ConfirmedLoss: "present",
				Baseline:      expectedBaseline(),
				Loss:          loss,
			},
		},
		{
			name: "failed restoration commit does not clear loss",
			prepare: func(t *testing.T) {
				require.NoError(t, os.Rename(dir, dir+".offline"))
				h.write(t, h.data)
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "state_error",
				ConfirmedLoss: "present",
				Values:        map[string]float64{"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0},
				Baseline:      expectedBaseline(),
				Loss:          loss,
			},
		},
		{
			name:    "successful restoration commit clears loss",
			prepare: func(t *testing.T) { require.NoError(t, os.Rename(dir+".offline", dir)) },
			want:    healthyCollection(),
		},
	} {
		t.Run(step.name, func(t *testing.T) {
			step.prepare(t)
			cycle(t, c)
			assert.Equal(t, step.want, collectionOutput(t, c))
		})
		if t.Failed() {
			return
		}
	}
}

func TestCollectorInvalidSourceAndState(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(*testing.T, *fixtureHost)
		want    collectionResult
	}{
		"corrupt state": {
			prepare: func(t *testing.T, h *fixtureHost) {
				require.NoError(t, os.WriteFile(h.statePath, []byte("broken"), 0600))
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "state_error",
				ConfirmedLoss: "unknown",
				Values:        map[string]float64{"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0},
			},
		},
		"different owner": {
			prepare: func(t *testing.T, h *fixtureHost) {
				c := h.collector(t)
				cycle(t, c)
				c.Cleanup(t.Context())
				s, err := readState(h.statePath, "fixture-agent")
				require.NoError(t, err)
				s.Owner = "different-agent"
				require.NoError(t, saveState(h.statePath, s))
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "state_error",
				ConfirmedLoss: "unknown",
				Values:        map[string]float64{"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0},
			},
		},
		"failed first commit": {
			prepare: func(t *testing.T, h *fixtureHost) {
				require.NoError(t, os.Rename(filepath.Dir(h.statePath), filepath.Dir(h.statePath)+".offline"))
			},
			want: collectionResult{
				Inventory:     "available",
				Comparison:    "state_error",
				ConfirmedLoss: "unknown",
				Values:        map[string]float64{"installed_capacity_bytes": 1 << 40, "populated_slots": 16, "empty_slots": 0},
			},
		},
		"invalid first table": {
			prepare: func(t *testing.T, h *fixtureHost) { h.write(t, []byte{17, 92}) },
			want: collectionResult{
				Inventory:     "unavailable",
				Comparison:    "unavailable",
				ConfirmedLoss: "unknown",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFixtureHost(t)
			tc.prepare(t, h)
			before, beforeErr := os.ReadFile(h.statePath)
			c := h.collector(t)
			cycle(t, c)
			assert.Equal(t, tc.want, collectionOutput(t, c))
			after, afterErr := os.ReadFile(h.statePath)
			assert.Equal(t, before, after)
			assert.Equal(t, os.IsNotExist(beforeErr), os.IsNotExist(afterErr))
		})
	}
}

func TestCollectorFunctionInventory(t *testing.T) {
	for name, tc := range map[string]struct {
		baselineOnly bool
		want         [][]any
	}{
		"current inventory": {want: expectedFunctionRows(false)},
		"retained loss after restart without readable DMI": {baselineOnly: true, want: expectedFunctionRows(true)},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFixtureHost(t)
			c := h.collector(t)
			cycle(t, c)
			if tc.baselineOnly {
				r := fixtureRecords(t, h.data)
				binary.LittleEndian.PutUint16(r[1][12:14], 0)
				h.write(t, joinRecords(r))
				cycle(t, c)
				before := diskState(t, h)
				c.Cleanup(t.Context())
				h.write(t, []byte{17, 92})
				c = h.collector(t)
				cycle(t, c)
				assert.Equal(t, before, diskState(t, h))
			}
			before := diskState(t, h)
			response := c.funcRouter.Handle(t.Context(), "inventory", funcapi.ResolvedParams{})
			require.Equal(t, 200, response.Status)
			assert.Equal(t, tc.want, response.Data)
			assert.Len(t, response.Columns, len(tc.want[0]))
			assert.Equal(t, before, diskState(t, h))
		})
	}
}

func TestCollectorFunctionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		method     string
		cancel     bool
		wantStatus int
	}{
		"waiting for first collection": {method: "inventory", wantStatus: 503},
		"canceled request":             {method: "inventory", cancel: true, wantStatus: 499},
		"unknown method":               {method: "unsupported", wantStatus: 404},
	} {
		t.Run(name, func(t *testing.T) {
			c := newFixtureHost(t).collector(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			assert.Equal(t, tc.wantStatus, c.funcRouter.Handle(ctx, tc.method, funcapi.ResolvedParams{}).Status)
		})
	}
}

func TestFunctionConcurrencyAndCancellation(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	before := diskState(t, h)
	want := expectedFunctionRows(false)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			response := c.funcRouter.Handle(t.Context(), "inventory", funcapi.ResolvedParams{})
			assert.Equal(t, want, response.Data)
			_, err := json.Marshal(response)
			assert.NoError(t, err)
		}
	}()
	for range 10 {
		cycle(t, c)
	}
	wg.Wait()
	assert.Equal(t, before, diskState(t, h))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	assert.ErrorIs(t, c.Collect(ctx), context.Canceled)
	managed.CycleController().AbortCycle()
	assert.Equal(t, want, c.funcRouter.Handle(t.Context(), "inventory", funcapi.ResolvedParams{}).Data)
	assert.Equal(t, before, diskState(t, h))
	c.Cleanup(t.Context())
	c.Cleanup(t.Context())
}

func TestCollector_Init(t *testing.T) {
	for name, tc := range map[string]struct {
		configure func(*Collector)
		wantErr   bool
	}{
		"defaults":                      {},
		"vnode rejected":                {configure: func(c *Collector) { c.Vnode = "another-host" }, wantErr: true},
		"nonpositive interval rejected": {configure: func(c *Collector) { c.UpdateEvery = 0 }, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			if tc.configure != nil {
				tc.configure(c)
			}
			if tc.wantErr {
				require.Error(t, c.Init(t.Context()))
			} else {
				require.NoError(t, c.Init(t.Context()))
			}
		})
	}
}

func TestCollectorRegistration(t *testing.T) {
	creator := collectorapi.DefaultRegistry["smbios_memory"]
	type registration struct {
		Policy      collectorapi.InstancePolicy
		Methods     map[string]string // method id -> public Function name
		UpdateEvery int
	}
	got := registration{
		Policy:      creator.InstancePolicy,
		Methods:     make(map[string]string),
		UpdateEvery: New().UpdateEvery,
	}
	for _, method := range creator.SharedFunctions() {
		got.Methods[method.ID] = funcapi.FunctionName("smbios_memory", method)
	}
	assert.Equal(
		t,
		registration{
			Policy:      collectorapi.InstancePolicySingle,
			Methods:     map[string]string{"inventory": "smbios-memory-inventory"},
			UpdateEvery: 60,
		},
		got,
	)
}

func TestCandidateLoadsStateOnlyAfterActivation(t *testing.T) {
	h := newFixtureHost(t)
	active := h.collector(t)
	cycle(t, active)
	// Job replacement probes a candidate while the previous job may still run.
	candidate := h.collector(t)
	require.NoError(t, candidate.Check(t.Context()))
	r := fixtureRecords(t, h.data)
	binary.LittleEndian.PutUint16(r[1][12:14], 0)
	h.write(t, joinRecords(r))
	cycle(t, active)
	active.Cleanup(t.Context())
	h.write(t, []byte{17, 92})
	cycle(t, candidate)
	want := collectionResult{
		Inventory:     "unavailable",
		Comparison:    "unavailable",
		ConfirmedLoss: "present",
		Baseline:      expectedBaseline(),
		Loss:          []discrepancy{{Locator: "DIMM_P0_A0", Missing: true, Deficit: 64 << 30}},
	}
	assert.Equal(t, want, collectionOutput(t, candidate))
	assert.Equal(
		t,
		expectedFunctionRows(true),
		candidate.funcRouter.Handle(t.Context(), "inventory", funcapi.ResolvedParams{}).Data,
	)
}

// A debug run from a terminal compares like the Agent's job but never writes
// the baseline file that job owns.
func TestTerminalSessionKeepsStateReadOnly(t *testing.T) {
	loss := healthyCollection()
	loss.ConfirmedLoss = "present"
	loss.Values = map[string]float64{
		"installed_capacity_bytes": 15 * (64 << 30), "populated_slots": 15, "empty_slots": 1,
		"missing_devices": 1, "capacity_deficit_bytes": 64 << 30,
	}
	loss.Loss = []discrepancy{{Locator: "DIMM_P0_A0", Missing: true, Deficit: 64 << 30}}

	for name, tc := range map[string]struct {
		saved bool
	}{
		"without a saved baseline":        {},
		"with the Agent's saved baseline": {saved: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFixtureHost(t)
			if tc.saved {
				cycle(t, h.collector(t))
			}
			before, beforeErr := os.ReadFile(h.statePath)
			c := h.collector(t)
			c.isTerminal = func() bool { return true }
			require.NoError(t, c.Init(t.Context()))

			cycle(t, c)
			assert.Equal(t, healthyCollection(), collectionOutput(t, c))
			r := fixtureRecords(t, h.data)
			binary.LittleEndian.PutUint16(r[1][12:14], 0)
			h.write(t, joinRecords(r))
			cycle(t, c)
			assert.Equal(t, loss, collectionOutput(t, c))

			after, afterErr := os.ReadFile(h.statePath)
			assert.Equal(t, before, after)
			assert.Equal(t, os.IsNotExist(beforeErr), os.IsNotExist(afterErr))
		})
	}
}

func TestUnknownArrayUseDoesNotConfirmLoss(t *testing.T) {
	h := newFixtureHost(t)
	r := fixtureRecords(t, h.data)
	r[0][13] = 8
	secondArray := append([]byte(nil), r[0]...)
	binary.LittleEndian.PutUint16(secondArray[2:4], 0x200)
	for _, d := range r[9:17] {
		binary.LittleEndian.PutUint16(d[4:6], 0x200)
	}
	r = append([][]byte{r[0], secondArray}, r[1:]...)
	h.write(t, joinRecords(r))
	c := h.collector(t)
	cycle(t, c)
	before := diskState(t, h)
	// DSP0134 Use=02h withholds classification, not proof of removed devices.
	r[1][5] = 2
	h.write(t, joinRecords(r))
	cycle(t, c)
	want := collectionResult{
		Inventory:     "available",
		Comparison:    "uncomparable",
		ConfirmedLoss: "unknown",
		Baseline:      expectedBaseline(),
	}
	assert.Equal(t, want, collectionOutput(t, c))
	assert.Equal(t, before, diskState(t, h))
}
