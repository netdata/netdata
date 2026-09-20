// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Embed only the core interface: this fixture deliberately has no YAML method.
type nativeModuleV2 struct {
	collectorapi.CollectorV2
	set   *chartengine.TemplateSet
	calls int
}

func (m *nativeModuleV2) ChartTemplateSet() *chartengine.TemplateSet { m.calls++; return m.set }

func runtimeTemplateSet(t *testing.T, id string) *chartengine.TemplateSet {
	t.Helper()
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateV2()))
	require.NoError(t, err)
	spec.Groups[0].Charts[0].ID = id
	set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Entries: []chartengine.TemplateEntry{{ID: "profile", Groups: spec.Groups}},
	})
	require.NoError(t, err)
	return set
}

func collectNativeFrame(t *testing.T, job *JobV2) jobV2PreparedEmission {
	t.Helper()
	job.refreshVnodeSnapshot()
	prepared, ok := job.collectAndEmit(0)
	require.True(t, ok)
	return prepared
}

func TestJobV2NativeTriggeringCollectionAndInvalidCandidates(t *testing.T) {
	for name := range map[string]bool{"nil": true, "zero": true, "global change": true, "collect error": true, "metric commit error": true} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			base := &mockModuleV2{
				store: store,
			}
			mod := &nativeModuleV2{
				CollectorV2: base,
				set:         runtimeTemplateSet(t, "initial"),
			}
			var out bytes.Buffer
			job := newTestJobV2(mod, &out)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			assert.Equal(t, 1, mod.calls)
			next := runtimeTemplateSet(t, "triggered")
			base.collectFunc = func(context.Context) error {
				store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
				mod.set = next
				return nil
			}
			prepared := collectNativeFrame(t, job)
			require.NoError(t, job.finishPreparedEmission(prepared))
			assert.Equal(t, 2, mod.calls, "capture once after Collect")
			assert.Contains(t, out.String(), "CHART 'module_job.triggered'")
			assert.Contains(t, out.String(), "SET 'busy' = 7")
			assert.NotContains(t, out.String(), "module_job.initial")
			out.Reset()
			before := store.Read(metrix.ReadRaw()).CollectMeta()
			invalidGlobal, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
				FallbackContextNamespace: "changed",
			})
			require.NoError(t, err)
			base.collectFunc = func(context.Context) error {
				store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(99)
				switch name {
				case "nil":
					mod.set = nil
				case "zero":
					mod.set = &chartengine.TemplateSet{}
				case "global change":
					mod.set = invalidGlobal
				case "collect error":
					mod.set = runtimeTemplateSet(t, "failed")
					return errors.New("collect failed")
				case "metric commit error":
					mod.set = runtimeTemplateSet(t, "failed")
					store.Write().SnapshotMeter("apache").Counter("workers_busy").ObserveTotal(100)
				}
				return nil
			}
			_, ok := job.collectAndEmit(0)
			assert.False(t, ok)
			assert.Empty(t, out.String())
			reader := store.Read(metrix.ReadRaw())
			value, exists := reader.Value("apache.workers_busy", nil)
			require.True(t, exists)
			assert.Equal(t, float64(7), value)
			assert.Equal(t, before.LastSuccessSeq, reader.CollectMeta().LastSuccessSeq)
			expectedCalls := 3
			if name == "collect error" {
				expectedCalls = 2
			}
			assert.Equal(t, expectedCalls, mod.calls)
			mod.set = next
			base.collectFunc = func(context.Context) error {
				store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(8)
				return nil
			}
			prepared = collectNativeFrame(t, job)
			require.NoError(t, job.finishPreparedEmission(prepared))
			assert.NotContains(t, out.String(), "CHART 'module_job.triggered'", "failed candidate must preserve chart lifecycle")
			assert.Contains(t, out.String(), "SET 'busy' = 8")
		})
	}
}

type selectiveNativeOutput struct {
	bytes.Buffer
	reject string
}

func (out *selectiveNativeOutput) CommitJobOutput(payload []byte, state OutputStateTransaction) error {
	if out.reject != "" && strings.Contains(string(payload), out.reject) {
		return errors.Join(errors.New("scope output rejected"), state.Abort())
	}
	if _, err := out.Write(payload); err != nil {
		return errors.Join(err, state.Abort())
	}
	return state.Commit()
}

func TestJobV2NativeScopesCanSkipFailedCandidate(t *testing.T) {
	store := metrix.NewCollectorStore()
	base := &mockModuleV2{
		store: store,
	}
	mod := &nativeModuleV2{
		CollectorV2: base,
		set:         runtimeTemplateSet(t, "initial"),
	}
	scopeA := metrix.HostScope{
		ScopeKey: "a",
		GUID:     "guid-a",
		Hostname: "host-a",
	}
	scopeB := metrix.HostScope{
		ScopeKey: "b",
		GUID:     "guid-b",
		Hostname: "host-b",
	}
	base.collectFunc = func(context.Context) error {
		meter := store.Write().SnapshotMeter("apache")
		meter.WithHostScope(scopeA).Gauge("workers_busy").Observe(1)
		meter.WithHostScope(scopeB).Gauge("workers_busy").Observe(2)
		return nil
	}
	out := &selectiveNativeOutput{}
	job := NewJobV2(JobV2Config{
		PluginName:  pluginName,
		Name:        jobName,
		ModuleName:  modName,
		FullName:    modName + "_" + jobName,
		Module:      mod,
		Out:         out,
		UpdateEvery: 1,
		Publication: hostoutput.New(),
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	prepared := collectNativeFrame(t, job)
	require.Len(t, prepared.scopes, 2)
	require.NoError(t, job.finishPreparedEmission(prepared))
	mod.set = runtimeTemplateSet(t, "intermediate")
	out.reject = "HOST 'guid-b'"
	prepared = collectNativeFrame(t, job)
	require.NoError(t, job.finishPreparedEmission(prepared), "successful peer scope remains published")
	assert.Equal(t, 3, mod.calls, "getter count does not depend on scope count")
	mod.set = runtimeTemplateSet(t, "latest")
	out.reject = ""
	out.Reset()
	prepared = collectNativeFrame(t, job)
	require.Len(t, prepared.scopes, 2)
	for _, scope := range prepared.scopes {
		removed := ""
		for _, action := range scope.plan.Actions {
			if a, ok := action.(chartengine.RemoveChartAction); ok {
				removed = a.ChartID
			}
		}
		if scope.scope.scopeKey == "a" {
			assert.Equal(t, "intermediate", removed)
		} else {
			assert.Equal(t, "initial", removed)
		}
	}
	require.NoError(t, job.finishPreparedEmission(prepared))
	assert.Equal(t, 4, mod.calls)
	assert.Equal(t, 2, strings.Count(out.String(), "CHART 'module_job.latest'"))
}

func TestJobV2RequiresProviderExceptFunctionOnly(t *testing.T) {
	base := &mockModuleV2{
		store: metrix.NewCollectorStore(),
	}
	missing := struct{ collectorapi.CollectorV2 }{base}
	for _, functionOnly := range []bool{false, true} {
		job := NewJobV2(JobV2Config{
			PluginName:   pluginName,
			Name:         jobName,
			ModuleName:   modName,
			FullName:     modName + "_" + jobName,
			Module:       missing,
			Out:          &bytes.Buffer{},
			FunctionOnly: functionOnly,
		})
		err := job.AutoDetectionManaged(context.Background())
		if functionOnly {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "exactly one")
		}
	}
}

func TestJobV2NativeHostSwitchAndTemplateChange(t *testing.T) {
	for name, tc := range map[string]struct {
		empty     bool
		retry     bool
		supersede bool
	}{
		"rejected switch cleanup":             {},
		"rejected switch retry":               {retry: true},
		"rejected switch newer snapshot":      {retry: true, supersede: true},
		"empty switch cleanup":                {empty: true},
		"empty switch then nonempty snapshot": {empty: true, retry: true, supersede: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			emitValue := true
			base := &mockModuleV2{
				store: store,
				collectFunc: func(context.Context) error {
					if emitValue {
						store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
					}
					return nil
				},
			}
			mod := &nativeModuleV2{
				CollectorV2: base,
				set:         runtimeTemplateSet(t, "initial"),
			}
			initial := vnodes.VirtualNode{
				Name:     "db",
				Hostname: "host-a",
				GUID:     "guid-a",
			}
			next := vnodes.VirtualNode{
				Name:     "db",
				Hostname: "host-b",
				GUID:     "guid-b",
			}
			out := &selectiveNativeOutput{}
			publisher := hostoutput.New()
			job := NewJobV2(JobV2Config{
				PluginName:  pluginName,
				Name:        jobName,
				ModuleName:  modName,
				FullName:    modName + "_" + jobName,
				Module:      mod,
				Out:         out,
				UpdateEvery: 1,
				Vnode:       initial,
				Publication: publisher,
			})
			currentVnode := bindJobV2VnodeLookup(job, "db", VnodeSnapshot{
				Vnode:            &initial,
				Revision:         1,
				MetadataRevision: 1,
			})
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			require.NoError(t, job.finishPreparedEmission(collectNativeFrame(t, job)))
			require.Contains(t, out.String(), "CHART 'module_job.initial'")
			require.Equal(t, 1, publisher.OwnerCount(initial.GUID))
			out.Reset()

			mod.set = runtimeTemplateSet(t, "intermediate")
			currentVnode.set(VnodeSnapshot{
				Vnode:            &next,
				Revision:         2,
				MetadataRevision: 2,
			})
			emitValue = !tc.empty
			out.reject = "HOST 'guid-b'"
			prepared := collectNativeFrame(t, job)
			require.Len(t, prepared.scopes, 1)
			assert.True(t, prepared.scopes[0].decision.needEngineReload)
			assert.NotContains(t, string(prepared.scopes[0].output), "module_job.initial")
			if tc.empty {
				assert.Empty(t, prepared.scopes[0].plan.Actions)
				require.NoError(t, job.finishPreparedEmission(prepared))
				assert.Equal(t, jobV2HostFromVnode(next), requireDefaultScopeState(t, job).host.engineHost)
			} else {
				require.ErrorContains(t, job.finishPreparedEmission(prepared), "scope output rejected")
				assert.Equal(t, jobV2HostFromVnode(initial), requireDefaultScopeState(t, job).host.engineHost)
			}
			assert.Empty(t, out.String())
			assert.Equal(t, jobV2HostFromVnode(initial), requireDefaultScopeState(t, job).host.cleanupOwner)
			assert.Equal(t, 1, publisher.OwnerCount(initial.GUID))
			assert.Zero(t, publisher.OwnerCount(next.GUID))

			wantHost, wantChart := initial.GUID, "initial"
			if tc.retry {
				wantHost, wantChart = next.GUID, "intermediate"
				if tc.supersede {
					wantChart = "latest"
					mod.set = runtimeTemplateSet(t, wantChart)
				}
				emitValue = true
				out.reject = ""
				prepared = collectNativeFrame(t, job)
				require.Len(t, prepared.scopes, 1)
				assert.Equal(t, !tc.empty, prepared.scopes[0].decision.needEngineReload)
				require.NoError(t, job.finishPreparedEmission(prepared))
				assert.Contains(t, out.String(), "HOST 'guid-b'")
				assert.Contains(t, out.String(), "CHART 'module_job."+wantChart+"'")
				assert.Contains(t, out.String(), "SET 'busy' = 7")
				assert.NotContains(t, out.String(), "module_job.initial")
				assert.NotContains(t, out.String(), "obsolete")
				assert.Zero(t, publisher.OwnerCount(initial.GUID))
				assert.Equal(t, 1, publisher.OwnerCount(next.GUID))
			}
			out.Reset()
			out.reject = ""
			job.Cleanup()
			assert.Contains(t, out.String(), "HOST '"+wantHost+"'")
			assert.Contains(t, out.String(), "CHART 'module_job."+wantChart+"'")
			assert.Equal(t, 1, strings.Count(out.String(), "'obsolete'"), "cleanup only charts emitted on its host")
			if tc.retry {
				assert.NotContains(t, out.String(), "module_job.initial")
			}
			assert.Zero(t, publisher.Len())
		})
	}
}
