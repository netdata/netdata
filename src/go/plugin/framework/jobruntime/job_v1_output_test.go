// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV1WriteFailurePreservesCommittedEmission(t *testing.T) {
	for name, tc := range map[string]struct{ warm, switchHost bool }{"first write": {}, "redefinition": {warm: true}, "host switch": {warm: true, switchHost: true}} {
		t.Run(name, func(t *testing.T) {
			reject := false
			var wire bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(writeFunc(func(p []byte) (int, error) {
				if reject {
					return len(p) - 1, nil
				}
				return wire.Write(p)
			}))
			require.NoError(t, err)
			chart := &collectorapi.Chart{
				ID:    "work",
				Title: "old",
				Units: "units",
				Dims:  collectorapi.Dims{{ID: "old"}, {ID: "value"}},
			}
			charts := collectorapi.Charts{chart}
			mod := &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 {
					if reject {
						chart.Title = "desired"
						_ = chart.MarkDimRemove("old", false)
						chart.MarkNotCreated()
					}
					return map[string]int64{"old": 1, "value": 1}
				},
			}
			current := newSnapshotHolder(
				VnodeSnapshot{
					Vnode: &vnodes.VirtualNode{
						Name:     "device",
						Hostname: "device",
						GUID:     sharedGUID,
					},
					Revision:         1,
					MetadataRevision: 1,
				},
			)
			publisher := hostoutput.New()
			job := NewJob(
				JobConfig{
					PluginName: "go.d",
					Name:       "device",
					ModuleName: "test",
					FullName:   "test_device",
					Module:     mod,
					Out: sharedFrameOutput{
						owner: frames,
					},
					Publication:           publisher,
					Vnode:                 *current.snapshot().Vnode,
					VnodeName:             "device",
					VnodeRevision:         1,
					VnodeMetadataRevision: 1,
					VnodeLookup:           current.lookup,
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			if tc.warm {
				job.runOnce()
			}
			type committed struct {
				guid         string
				owner        *hostoutput.Owner
				info         netdataapi.HostInfo
				charts, self jobV1ChartInventory
				prev         time.Time
				priority     int
			}
			snapshot := func() committed {
				return committed{
					job.hostGUID,
					job.hostOwner,
					job.hostDefinition.Info(),
					maps.Clone(job.hostCharts),
					maps.Clone(job.selfCharts),
					job.prevRun,
					job.priority,
				}
			}
			before := snapshot()
			if tc.switchHost {
				current.set(
					VnodeSnapshot{
						Vnode: &vnodes.VirtualNode{
							Name:     "device",
							Hostname: "next",
							GUID:     "22222222-2222-2222-2222-222222222222",
						},
						Revision:         2,
						MetadataRevision: 2,
					},
				)
			}
			reject = true
			job.runOnce()
			assert.Equal(t, before, snapshot())
			assert.False(t, chart.IsCreated(), "collector's pending redefinition must survive abort")
			assert.Len(t, chart.Dims, 2)
			assert.True(t, chart.Dims[0].IsRemoved())
			assert.False(t, job.panicked.Load())
			assert.True(t, frames.Census().Poisoned)
			if tc.warm {
				assert.Equal(t, 1, publisher.Len())
			} else {
				assert.Zero(t, publisher.Len())
			}
			job.Cleanup()
			assert.Zero(t, publisher.Len())
		})
	}
}

func TestV1CommittedChartInventory(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate     func(*collectorapi.Charts)
		wantTitles map[string]string
		wantDims   int
		wantCharts int
	}{
		"redefinition": {mutate: func(cs *collectorapi.Charts) {
			c := (*cs)[0]
			c.Title = "new"
			c.MarkNotCreated()
		}, wantTitles: map[string]string{"test_device.work": "new"}, wantDims: 2, wantCharts: 1},
		"dimension removal": {mutate: func(cs *collectorapi.Charts) {
			c := (*cs)[0]
			_ = c.MarkDimRemove("old", false)
			c.MarkNotCreated()
		}, wantTitles: map[string]string{"test_device.work": "old"}, wantDims: 1, wantCharts: 1},
		"obsolete definition": {mutate: func(cs *collectorapi.Charts) {
			c := (*cs)[0]
			c.MarkRemove()
			c.MarkNotCreated()
		}, wantTitles: map[string]string{}, wantCharts: 0},
		"removal intent without obsolete output": {mutate: func(cs *collectorapi.Charts) { (*cs)[0].MarkRemove() }, wantTitles: map[string]string{"test_device.work": "old"}, wantCharts: 0},
		"same ID replacement": {mutate: func(cs *collectorapi.Charts) {
			c := (*cs)[0]
			c.MarkRemove()
			c.MarkNotCreated()
			_ = cs.Add(&collectorapi.Chart{
				ID:    "work",
				Title: "replacement",
				Units: "units",
				Dims:  collectorapi.Dims{{ID: "value"}},
			})
		}, wantTitles: map[string]string{"test_device.work": "replacement"}, wantDims: 1, wantCharts: 1},
	} {
		t.Run(name, func(t *testing.T) {
			var wire bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&wire)
			require.NoError(t, err)
			chart := &collectorapi.Chart{
				ID:    "work",
				Title: "old",
				Units: "units",
				Dims:  collectorapi.Dims{{ID: "old"}, {ID: "value"}},
			}
			charts := collectorapi.Charts{chart}
			change := false
			mod := &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 {
					if change {
						tc.mutate(&charts)
					}
					return map[string]int64{"old": 1, "value": 1}
				},
			}
			job := NewJob(
				JobConfig{
					PluginName: "go.d",
					Name:       "device",
					ModuleName: "test",
					FullName:   "test_device",
					Module:     mod,
					Out: sharedFrameOutput{
						owner: frames,
					},
					Vnode: vnodes.VirtualNode{
						GUID:     sharedGUID,
						Hostname: "device",
					},
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()
			change = true
			wire.Reset()
			job.runOnce()
			titles := map[string]string{}
			for key, opts := range job.hostCharts {
				titles[key] = opts.Title
			}
			assert.Equal(t, tc.wantTitles, titles)
			require.Len(t, charts, tc.wantCharts)
			if len(charts) > 0 {
				assert.Len(t, charts[0].Dims, tc.wantDims)
				assert.True(t, charts[0].IsCreated())
				if name != "same ID replacement" {
					assert.Same(t, chart, charts[0])
				}
			}
			wire.Reset()
			job.Cleanup()
			assert.Equal(t, len(tc.wantTitles)+2, strings.Count(wire.String(), "CHART '"))
		})
	}
}

func TestV1IgnoredDefinitionHasNoPublishedHost(t *testing.T) {
	for name, tc := range map[string]struct{ id string }{
		"oversized ID":           {id: strings.Repeat("x", NetdataChartIDMaxLength)},
		"oversized qualified ID": {id: strings.Repeat("x", NetdataChartIDMaxLength-len("test_device"))},
	} {
		t.Run(name, func(t *testing.T) {
			var wire bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&wire)
			require.NoError(t, err)
			charts := collectorapi.Charts{
				&collectorapi.Chart{
					ID:    tc.id,
					Title: "ignored",
					Units: "units",
					Dims:  collectorapi.Dims{{ID: "value"}},
				},
			}
			mod := &collectorapi.MockCollectorV1{
				ChartsFunc:  func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
			}
			publisher := hostoutput.New()
			job := NewJob(
				JobConfig{
					PluginName: "go.d",
					Name:       "device",
					ModuleName: "test",
					FullName:   "test_device",
					Module:     mod,
					Out: sharedFrameOutput{
						owner: frames,
					},
					Publication: publisher,
					Vnode: vnodes.VirtualNode{
						GUID:     sharedGUID,
						Hostname: "device",
					},
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()
			assert.True(t, charts[0].IsIgnored())
			assert.Empty(t, job.hostCharts)
			assert.Empty(t, job.hostGUID)
			assert.Zero(t, publisher.Len())
			wire.Reset()
			job.Cleanup()
			assert.NotContains(t, wire.String(), sharedGUID)
			assert.NotContains(t, wire.String(), "ignored")
			assert.Equal(t, 2, strings.Count(wire.String(), "CHART '"))
		})
	}
}
