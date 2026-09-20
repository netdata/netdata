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

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

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
		statePath: filepath.Join(stateDir, "smbios-memory.json"),
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
	c.statePath = h.statePath
	c.owner = "fixture-agent"
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
func status(t *testing.T, c *Collector, name, want string) {
	t.Helper()
	p, ok := c.store.Read().StateSet(name, nil)
	require.True(t, ok, name)
	assert.True(t, p.States[want], "%s: %v", name, p.States)
}
func gauge(t *testing.T, c *Collector, name string) float64 {
	t.Helper()
	p, ok := c.store.Read().Value(name, nil)
	require.True(t, ok, name)
	return p
}
func diskState(t *testing.T, h *fixtureHost) []byte {
	t.Helper()
	data, err := os.ReadFile(h.statePath)
	require.NoError(t, err)
	return data
}

func TestBaselineLossRestartRestoreAndReset(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	require.NoError(t, c.Check(t.Context()))
	_, err := os.Stat(h.statePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "absent")
	assert.Equal(t, float64(1<<40), gauge(t, c, "installed_capacity_bytes"))
	initial := diskState(t, h)
	info, err := os.Stat(h.statePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	cycle(t, c)
	assert.Equal(t, initial, diskState(t, h))
	info2, err := os.Stat(h.statePath)
	require.NoError(t, err)
	assert.Equal(t, info.ModTime(), info2.ModTime())
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	// Empty slot A, while B doubles: aggregate capacity stays 1 TiB but A is lost.
	r := fixtureRecords(t, h.data)
	binary.LittleEndian.PutUint16(r[1][12:14], 0)
	binary.LittleEndian.PutUint32(r[2][28:32], 131072)
	h.write(t, joinRecords(r))
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "present")
	assert.Equal(t, 1.0, gauge(t, c, "missing_devices"))
	assert.Equal(t, float64(64<<30), gauge(t, c, "capacity_deficit_bytes"))
	assert.Equal(t, float64(1<<40), gauge(t, c, "installed_capacity_bytes"))
	loss := diskState(t, h)
	assert.NotEqual(t, initial, loss)
	c.Cleanup(t.Context())
	h.write(t, []byte{17, 92})
	c = h.collector(t)
	require.NoError(t, c.Check(t.Context()))
	cycle(t, c)
	status(t, c, "inventory_status", "unavailable")
	status(t, c, "confirmed_loss_status", "present")
	_, ok := c.store.Read().Value("installed_capacity_bytes", nil)
	assert.False(t, ok)
	assert.Equal(t, loss, diskState(t, h))
	response := c.router.Handle(t.Context(), "inventory", funcapi.ResolvedParams{})
	require.Equal(t, 200, response.Status)
	assert.Contains(t, response.Help, "Retained loss")
	require.Len(t, response.Data.([][]any), 16)
	assert.Nil(t, response.Data.([][]any)[0][4])

	h.write(t, h.data)
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "absent")
	// The original A is restored; B's temporary increase during loss was not accepted.
	assert.Equal(t, initial, diskState(t, h))
	// Missing Type 17 with a consistent array count is a comparable disappearance.
	r = fixtureRecords(t, h.data)
	r = append(r[:1], r[2:]...)
	r[0][13] = 15
	h.write(t, joinRecords(r))
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "present")
	assert.Equal(t, 1.0, gauge(t, c, "missing_devices"))
	c.Cleanup(t.Context())
	require.NoError(t, os.Rename(h.statePath, h.statePath+".backup"))
	c = h.collector(t)
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "absent")
	assert.Len(t, c.state.Baseline, 15)
}

func TestBaselineAdditionsIdentityAndFailures(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	r := fixtureRecords(t, h.data)
	binary.LittleEndian.PutUint32(r[1][28:32], 131072)
	h.write(t, joinRecords(r))
	cycle(t, c)
	assert.Equal(t, uint64(128<<30), *c.state.Baseline[0].Capacity)
	increased := diskState(t, h)
	// Reordering records and changing table-local handles must not change the baseline.
	r[1], r[2] = r[2], r[1]
	r[1][2] = 0x80
	h.write(t, joinRecords(r))
	cycle(t, c)
	assert.Equal(t, increased, diskState(t, h))
	// Duplicate locators make comparison unavailable, not a memory loss.
	r[1][16] = 2
	r[2][16] = 2
	h.write(t, joinRecords(r))
	cycle(t, c)
	status(t, c, "inventory_status", "available")
	status(t, c, "comparison_status", "uncomparable")
	status(t, c, "confirmed_loss_status", "unknown")
	assert.Equal(t, increased, diskState(t, h))
	// A complete snapshot with original sizes proves a reduction, even after restart.
	c.Cleanup(t.Context())
	h.write(t, h.data)
	c = h.collector(t)
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "present")
	assert.Zero(t, gauge(t, c, "missing_devices"))
	assert.Equal(t, float64(64<<30), gauge(t, c, "capacity_deficit_bytes"))
}

func TestPersistenceFailureRetainsLossAndRetries(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	dir := filepath.Dir(h.statePath)
	require.NoError(t, os.Rename(dir, dir+".offline"))
	r := fixtureRecords(t, h.data)
	binary.LittleEndian.PutUint16(r[1][12:14], 0)
	h.write(t, joinRecords(r))
	cycle(t, c)
	status(t, c, "comparison_status", "state_error")
	status(t, c, "confirmed_loss_status", "present")
	h.write(t, []byte{17, 92})
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "present")
	require.NoError(t, os.Rename(dir+".offline", dir))
	cycle(t, c)
	c.Cleanup(t.Context())
	c = h.collector(t)
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "present")
	// A restoration whose commit fails does not clear the retained loss.
	require.NoError(t, os.Rename(dir, dir+".offline"))
	h.write(t, h.data)
	cycle(t, c)
	status(t, c, "comparison_status", "state_error")
	status(t, c, "confirmed_loss_status", "present")
	require.NoError(t, os.Rename(dir+".offline", dir))
	cycle(t, c)
	status(t, c, "confirmed_loss_status", "absent")
}

func TestUnbaselinedAndInvalidState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *fixtureHost)
	}{
		{"corrupt state", func(t *testing.T, h *fixtureHost) {
			require.NoError(t, os.WriteFile(h.statePath, []byte("broken"), 0600))
		}},
		{"different owner", func(t *testing.T, h *fixtureHost) {
			c := h.collector(t)
			cycle(t, c)
			c.Cleanup(t.Context())
			s, err := readState(h.statePath, "fixture-agent")
			require.NoError(t, err)
			s.Owner = "different-agent"
			require.NoError(t, saveState(h.statePath, s))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFixtureHost(t)
			tc.prepare(t, h)
			before := diskState(t, h)
			c := h.collector(t)
			cycle(t, c)
			status(t, c, "comparison_status", "state_error")
			status(t, c, "confirmed_loss_status", "unknown")
			assert.Equal(t, before, diskState(t, h))
		})
	}
	t.Run("failed first commit", func(t *testing.T) {
		h := newFixtureHost(t)
		require.NoError(t, os.Rename(filepath.Dir(h.statePath), filepath.Dir(h.statePath)+".offline"))
		c := h.collector(t)
		cycle(t, c)
		assert.Nil(t, c.state)
		status(t, c, "confirmed_loss_status", "unknown")
		status(t, c, "comparison_status", "state_error")
	})
	t.Run("invalid first table", func(t *testing.T) {
		h := newFixtureHost(t)
		h.write(t, []byte{17, 92})
		c := h.collector(t)
		require.Error(t, c.Check(t.Context()))
		cycle(t, c)
		assert.Nil(t, c.state)
		status(t, c, "confirmed_loss_status", "unknown")
		_, err := os.Stat(h.statePath)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestFunctionConcurrencyAndCancellation(t *testing.T) {
	h := newFixtureHost(t)
	c := h.collector(t)
	assert.Equal(t, 503, c.router.Handle(t.Context(), "inventory", funcapi.ResolvedParams{}).Status)
	cycle(t, c)
	before := diskState(t, h)
	response := c.router.Handle(t.Context(), "inventory", funcapi.ResolvedParams{})
	for _, row := range response.Data.([][]any) {
		require.Len(t, row, len(response.Columns))
	}
	assert.Equal(t, c.snapshot.Load().ReadAt.UnixMilli(), response.Data.([][]any)[0][17])
	assert.Equal(t, "available", response.Data.([][]any)[0][18])
	assert.Nil(t, response.Data.([][]any)[0][19])
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			r := c.router.Handle(t.Context(), "inventory", funcapi.ResolvedParams{})
			if r.Status != 200 {
				t.Errorf("unexpected Function status %d", r.Status)
			}
			_, err := json.Marshal(r)
			if err != nil {
				t.Error(err)
			}
		}
	}()
	for range 10 {
		cycle(t, c)
	}
	wg.Wait()
	assert.Equal(t, before, diskState(t, h))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.Equal(t, 499, c.router.Handle(ctx, "inventory", funcapi.ResolvedParams{}).Status)
	managed, _ := metrix.AsCycleManagedStore(c.store)
	managed.CycleController().BeginCycle()
	assert.ErrorIs(t, c.Collect(ctx), context.Canceled)
	managed.CycleController().AbortCycle()
	assert.Equal(t, 404, c.router.Handle(t.Context(), "unsupported", funcapi.ResolvedParams{}).Status)
	c.Cleanup(t.Context())
	c.Cleanup(t.Context())
}

func TestConfigAndRegistration(t *testing.T) {
	c := New()
	data, err := json.Marshal(c.Configuration())
	require.NoError(t, err)
	var jsonConfig, yamlConfig Config
	require.NoError(t, json.Unmarshal(data, &jsonConfig))
	data, err = yaml.Marshal(c.Configuration())
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &yamlConfig))
	assert.Equal(t, jsonConfig, yamlConfig)
	assert.Equal(t, 60, jsonConfig.UpdateEvery)
	c.Vnode = "another-host"
	require.ErrorContains(t, c.Init(t.Context()), "does not support vnode")
	c.Vnode = ""
	c.UpdateEvery = 0
	require.Error(t, c.Init(t.Context()))
	creator := collectorapi.DefaultRegistry["smbios_memory"]
	assert.Equal(t, collectorapi.InstancePolicySingle, creator.InstancePolicy)
	methods := creator.SharedFunctions()
	require.Len(t, methods, 1)
	assert.Equal(t, "inventory", methods[0].ID)
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
	status(t, candidate, "confirmed_loss_status", "present")
	assert.Contains(t, candidate.snapshot.Load().Rows[0].Comparison, "retained loss")
}
