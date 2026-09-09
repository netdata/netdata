// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sharedGUID = "11111111-1111-1111-1111-111111111111"

type sharedFrameOutput struct{ owner *lifecycle.FrameOwner }

func (w sharedFrameOutput) Write(payload []byte) (int, error) {
	err := w.owner.CommitBorrowedProtocolFrame(payload)
	if err != nil {
		return 0, err
	}
	return len(payload), nil
}
func (w sharedFrameOutput) CommitBuiltJobOutput(build func() ([]byte, error), state OutputStateTransaction) error {
	return w.owner.CommitBuiltProtocolTransaction(build, state)
}

func newSharedContributor(
	t *testing.T,
	kind, name string,
	publisher *hostoutput.Publisher,
	writer sharedFrameOutput,
) (run, cleanup func()) {
	t.Helper()
	vnode := vnodes.VirtualNode{
		GUID:     sharedGUID,
		Hostname: name,
		Labels:   map[string]string{"vendor": "cisco"},
	}
	if kind == "v1" {
		charts := collectorapi.Charts{
			&collectorapi.Chart{
				ID:    "work",
				Title: "Work",
				Units: "units",
				Dims:  collectorapi.Dims{{ID: "value"}},
			},
		}
		module := &v1ModuleOwnedVnodeCollector{
			vnode: &vnode,
			MockCollectorV1: collectorapi.MockCollectorV1{
				ChartsFunc:  func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
			},
		}
		job := NewJob(
			JobConfig{
				PluginName:  "go.d",
				Name:        name,
				ModuleName:  "test",
				FullName:    "test_" + name,
				Module:      module,
				Publication: publisher,
				Out:         writer,
			},
		)
		require.NoError(t, job.AutoDetectionManaged(context.Background()))
		return job.runOnce, job.Cleanup
	}
	store := metrix.NewCollectorStore()
	module := &mockModuleV2{
		store:    store,
		template: chartTemplateV2(),
	}
	if kind == "v2" {
		module.vnode = &vnode
	}
	module.collectFunc = func(context.Context) error {
		meter := store.Write().SnapshotMeter("apache")
		if kind == "scope" {
			meter = meter.WithHostScope(
				metrix.HostScope{
					ScopeKey: "resource",
					GUID:     sharedGUID,
					Hostname: name,
					Labels:   vnode.Labels,
				},
			)
		}
		meter.Gauge("workers_busy").Observe(1)
		return nil
	}
	job := NewJobV2(
		JobV2Config{
			PluginName:  "go.d",
			Name:        name,
			ModuleName:  "test",
			FullName:    "test_" + name,
			Module:      module,
			Publication: publisher,
			Out:         writer,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	return job.runOnce, job.Cleanup
}

func TestQuietContributorAcrossConfiguredAuthority(t *testing.T) {
	for name, tc := range map[string]struct{ kind string }{
		"V1":          {kind: "v1"},
		"V2 default":  {kind: "v2"},
		"V2 explicit": {kind: "scope"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&out)
			require.NoError(t, err)
			publisher := hostoutput.New()
			writer := sharedFrameOutput{
				owner: frames,
			}
			runA, cleanupA := newSharedContributor(t, tc.kind, "generated", publisher, writer)
			runB, cleanupB := newSharedContributor(t, "v2", "peer", publisher, writer)
			runA()
			oracle := newHostProtocolOracle()
			oracle.apply(t, out.String())
			require.Equal(t, "cisco", oracle.labels[sharedGUID]["vendor"])
			configured, err := hostoutput.NewDefinition(
				netdataapi.HostInfo{
					GUID:     sharedGUID,
					Hostname: "configured",
					Labels:   map[string]string{"site": "london"},
				},
			)
			require.NoError(t, err)
			publisher.Bind(func(string) *hostoutput.Definition { return configured })
			out.Reset()
			runB()
			oracle.apply(t, out.String())
			require.Equal(t, map[string]string{"_hostname": "configured", "site": "london"}, oracle.labels[sharedGUID])
			cleanupB()
			publisher.Bind(nil)
			out.Reset()
			runA()
			oracle.apply(t, out.String())
			assert.Equal(t, map[string]string{"_hostname": "generated", "vendor": "cisco"}, oracle.labels[sharedGUID])
			out.Reset()
			runA()
			assert.NotContains(t, out.String(), "HOST_DEFINE")
			cleanupA()
			assert.Zero(t, publisher.Len())
		})
	}
}

func TestBufferedScopeUsesCurrentAuthority(t *testing.T) {
	for name, tc := range map[string]struct{ explicit bool }{"default": {}, "explicit": {explicit: true}} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&out)
			require.NoError(t, err)
			publisher := hostoutput.New()
			writer := sharedFrameOutput{
				owner: frames,
			}
			store := metrix.NewCollectorStore()
			module := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
			}
			module.vnode = &vnodes.VirtualNode{
				GUID:     sharedGUID,
				Hostname: "old",
			}
			module.collectFunc = func(context.Context) error {
				meter := store.Write().SnapshotMeter("apache")
				if tc.explicit {
					meter = meter.WithHostScope(
						metrix.HostScope{
							ScopeKey: "resource",
							GUID:     sharedGUID,
							Hostname: "old",
						},
					)
				}
				meter.Gauge("workers_busy").Observe(1)
				return nil
			}
			job := NewJobV2(
				JobV2Config{
					PluginName:  "go.d",
					Name:        "buffered",
					ModuleName:  "test",
					FullName:    "test_buffered",
					Module:      module,
					Out:         writer,
					Publication: publisher,
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			defer job.Cleanup()
			prepared, ok := job.collectAndEmit(0)
			require.True(t, ok)
			configured, err := hostoutput.NewDefinition(
				netdataapi.HostInfo{
					GUID:     sharedGUID,
					Hostname: "new",
					Labels:   map[string]string{"site": "athens"},
				},
			)
			require.NoError(t, err)
			publisher.Bind(func(string) *hostoutput.Definition { return configured })
			require.NoError(t, job.finishPreparedEmission(prepared))
			oracle := newHostProtocolOracle()
			oracle.apply(t, out.String())
			assert.Equal(t, map[string]string{"_hostname": "new", "site": "athens"}, oracle.labels[sharedGUID])
		})
	}
}

func TestUncollectedV1CleanupDoesNotDefineOrSelectVnode(t *testing.T) {
	var out bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&out)
	require.NoError(t, err)
	publisher := hostoutput.New()
	_, cleanup := newSharedContributor(t, "v1", "unused", publisher, sharedFrameOutput{
		owner: frames,
	})
	cleanup()
	assert.NotContains(t, out.String(), "HOST_DEFINE")
	assert.NotContains(t, out.String(), "HOST '"+sharedGUID+"'")
	assert.Zero(t, publisher.Len())
}

// The oracle derives replacement labels and chart routing solely from protocol commands.
type hostProtocolOracle struct {
	labels             map[string]map[string]string
	charts             map[string]string
	selected, defining string
	pending            map[string]string
}

func newHostProtocolOracle() *hostProtocolOracle {
	return &hostProtocolOracle{
		labels: make(map[string]map[string]string),
		charts: make(map[string]string),
	}
}
func (o *hostProtocolOracle) apply(t *testing.T, wire string) {
	t.Helper()
	for _, line := range strings.Split(wire, "\n") {
		fields := strings.Split(line, "'")
		switch {
		case strings.HasPrefix(line, "HOST_DEFINE '"):
			require.GreaterOrEqual(t, len(fields), 5)
			o.defining = fields[1]
			o.pending = make(map[string]string)
		case strings.HasPrefix(line, "HOST_LABEL '"):
			require.GreaterOrEqual(t, len(fields), 5)
			o.pending[fields[1]] = fields[3]
		case line == "HOST_DEFINE_END":
			o.labels[o.defining] = o.pending
			o.defining = ""
		case strings.HasPrefix(line, "HOST '"):
			o.selected = fields[1]
			if o.selected != "" {
				require.Contains(t, o.labels, o.selected, "selected undefined host")
			}
		case strings.HasPrefix(line, "CHART '"):
			o.charts[o.selected+":"+fields[1]] = o.selected
		case strings.HasPrefix(line, "BEGIN '"):
			require.Contains(t, o.charts, o.selected+":"+fields[1], "sample lacks chart on selected host")
		}
	}
}

func TestV2CleanupPanicReleasesPublicationOwners(t *testing.T) {
	for name, tc := range map[string]struct{ explicit bool }{"default": {}, "explicit": {explicit: true}} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			module := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				vnode: &vnodes.VirtualNode{
					GUID:     sharedGUID,
					Hostname: "device",
				},
				cleanupFunc: func(context.Context) { panic("cleanup failed") },
			}
			module.collectFunc = func(context.Context) error {
				meter := store.Write().SnapshotMeter("apache")
				if tc.explicit {
					meter = meter.WithHostScope(
						metrix.HostScope{
							ScopeKey: "device",
							GUID:     sharedGUID,
							Hostname: "device",
						},
					)
				}
				meter.Gauge("workers_busy").Observe(1)
				return nil
			}
			publisher := hostoutput.New()
			var out bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&out)
			require.NoError(t, err)
			job := NewJobV2(
				JobV2Config{
					PluginName:  "go.d",
					Name:        "device",
					ModuleName:  "test",
					FullName:    "test_device",
					Module:      module,
					Publication: publisher,
					Out: sharedFrameOutput{
						owner: frames,
					},
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()
			require.Equal(t, 1, publisher.Len())
			require.Panics(t, job.Cleanup)
			assert.Zero(t, publisher.Len())
		})
	}
}

func TestV1PanickingHostSwitchKeepsChartTarget(t *testing.T) {
	for name, tc := range map[string]struct{ cleanup bool }{"resume collection": {}, "cleanup after panic": {cleanup: true}} {
		t.Run(name, func(t *testing.T) {
			current := newSnapshotHolder(
				VnodeSnapshot{
					Vnode: &vnodes.VirtualNode{
						Name:     "device",
						Hostname: "old",
						GUID:     sharedGUID,
					},
					Revision:         1,
					MetadataRevision: 1,
				},
			)
			fail := false
			charts := collectorapi.Charts{
				&collectorapi.Chart{
					ID:    "work",
					Title: "Work",
					Units: "units",
					Dims:  collectorapi.Dims{{ID: "value"}},
				},
			}
			module := &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 {
					if fail {
						panic("collect failed")
					}
					return map[string]int64{"value": 1}
				},
			}
			var out bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&out)
			require.NoError(t, err)
			job := NewJob(
				JobConfig{
					PluginName: "go.d",
					Name:       "device",
					ModuleName: "test",
					FullName:   "test_device",
					Module:     module,
					Out: sharedFrameOutput{
						owner: frames,
					},
					Vnode:                 *current.snapshot().Vnode.Copy(),
					VnodeName:             "device",
					VnodeRevision:         1,
					VnodeMetadataRevision: 1,
					VnodeLookup:           current.lookup,
				},
			)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()
			oracle := newHostProtocolOracle()
			oracle.apply(t, out.String())
			out.Reset()
			const next = "22222222-2222-2222-2222-222222222222"
			current.set(
				VnodeSnapshot{
					Vnode: &vnodes.VirtualNode{
						Name:     "device",
						Hostname: "new",
						GUID:     next,
					},
					Revision:         2,
					MetadataRevision: 2,
				},
			)
			fail = true
			job.runOnce()
			require.Empty(t, out.String())
			if tc.cleanup {
				job.Cleanup()
				assert.Contains(t, out.String(), "HOST '"+sharedGUID+"'")
				assert.NotContains(t, out.String(), next)
			} else {
				fail = false
				job.runOnce()
				oracle.apply(t, out.String())
			}
		})
	}
}
