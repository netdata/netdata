// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nativeCoverageCollector struct {
	store metrix.CollectorStore
	set   *chartengine.TemplateSet
	calls int
}

func (c *nativeCoverageCollector) MetricStore() metrix.CollectorStore { return c.store }
func (c *nativeCoverageCollector) ChartTemplateSet() *chartengine.TemplateSet {
	c.calls++
	return c.set
}

func TestNativeChartCoverageUsesEntriesScopesAndGlobalPolicy(t *testing.T) {
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	store.Write().SnapshotMeter("").Gauge("alpha").Observe(1)
	store.Write().SnapshotMeter("").Gauge("beta").Observe(2)
	scope := metrix.HostScope{
		ScopeKey: "remote",
		GUID:     "remote-guid",
		Hostname: "remote-host",
	}
	store.Write().SnapshotMeter("").WithHostScope(scope).Gauge("beta").Observe(3)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
	var entries []chartengine.TemplateEntry
	for _, metric := range []string{"alpha", "beta"} {
		entries = append(entries, chartengine.TemplateEntry{
			ID:               metric,
			ContextNamespace: metric,
			Groups:           []charttpl.Group{{Family: "Test", Metrics: []string{metric}, Charts: []charttpl.Chart{{Title: metric, Context: "value", Units: "items", Dimensions: []charttpl.Dimension{{Selector: metric, Name: "value"}}}}}},
		})
	}
	set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Entries: entries,
	})
	require.NoError(t, err)
	collector := &nativeCoverageCollector{
		store: store,
		set:   set,
	}
	AssertChartCoverage(t, collector, ChartCoverageExpectation{})
	assert.Equal(t, 1, collector.calls)
	coverages, err := buildChartCoveragesFromStore(set, store, nil)
	require.NoError(t, err)
	require.Len(t, coverages, 2)
	foundRemote := false
	for _, scoped := range coverages {
		if scoped.ScopeKey == "remote" {
			foundRemote = true
			assert.Equal(t, map[string][]string{"beta.value": {"value"}}, scoped.Coverage.ExpectedByContext)
		}
	}
	assert.True(t, foundRemote, "remote scope must be checked")
	filtered, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Entries: entries,
		Policy: chartengine.EnginePolicy{
			Selector: &metrixselector.Expr{
				Deny: []string{"beta"},
			},
		},
	})
	require.NoError(t, err)
	collector.set = filtered
	AssertChartCoverage(t, collector, ChartCoverageExpectation{})
	coverages, err = buildChartCoveragesFromStore(filtered, store, nil)
	require.NoError(t, err)
	for _, scoped := range coverages {
		assert.NotContains(t, scoped.Coverage.ExpectedByContext, "beta.value")
		assert.NotContains(t, scoped.Coverage.ActualByContext, "beta.value")
	}
}

func TestChartCoverageRespectsRequiredInstanceLabels(t *testing.T) {
	for _, tc := range []struct {
		name      string
		instances charttpl.Instances
		labels    []metrix.Label
		wantChart bool
	}{
		{name: "missing required", instances: charttpl.Instances{ByLabels: []string{"node"}}},
		{name: "present required", instances: charttpl.Instances{ByLabels: []string{"node"}}, labels: []metrix.Label{{Key: "node", Value: "a"}}, wantChart: true},
		{name: "blank required", instances: charttpl.Instances{ByLabels: []string{"node"}}, labels: []metrix.Label{{Key: "node", Value: ""}}, wantChart: true},
		{name: "missing optional", instances: charttpl.Instances{OptionalByLabels: []string{"node"}}, wantChart: true},
		{name: "wildcard and exclusion", instances: charttpl.Instances{ByLabels: []string{"*", "node", "!node"}}, wantChart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := coverageEntry("profile", "value", "requests", "requests")
			entry.Groups[0].Charts[0].Instances = &tc.instances
			for _, provider := range []string{"native", "yaml"} {
				t.Run(provider, func(t *testing.T) {
					set := coverageSetForEntries(t, provider, entry)
					store := metrix.NewCollectorStore()
					managed, ok := metrix.AsCycleManagedStore(store)
					require.True(t, ok)
					managed.CycleController().BeginCycle()
					store.Write().SnapshotMeter("").WithLabels(tc.labels...).Gauge("requests").Observe(1)
					require.NoError(t, managed.CycleController().CommitCycleSuccess())
					coverage, err := buildChartCoverage(set, store.Read(metrix.ReadRaw(), metrix.ReadFlatten()), nil)
					require.NoError(t, err)
					if tc.wantChart {
						assert.Equal(t, map[string][]string{"value": {"requests"}}, coverage.ExpectedByContext)
						assert.Equal(t, map[string]map[string]struct{}{"value": {"requests": {}}}, coverage.ActualByContext)
					} else {
						assert.Empty(t, coverage.ExpectedByContext)
						assert.Empty(t, coverage.ActualByContext)
					}
				})
			}
		})
	}
}

func TestChartCoverageExcludesOnlyLosingCollisionRoutes(t *testing.T) {
	for _, provider := range []string{"native", "yaml"} {
		t.Run(provider, func(t *testing.T) {
			first := coverageEntry("first", "value", `requests{node="shared"}`, "first")
			second := coverageEntry("second", "value", "requests", "")
			for _, entry := range []*chartengine.TemplateEntry{&first, &second} {
				entry.Groups[0].Charts[0].ID = "shared"
				entry.Groups[0].Charts[0].Instances = &charttpl.Instances{ByLabels: []string{"node"}}
			}
			second.Groups[0].Charts[0].Dimensions[0].NameFromLabel = "state"
			// Exercise nested chart provenance and one losing/one winning instance
			// of the same dimension. Dropping an entire template would hide "kept".
			second.Groups = []charttpl.Group{{Family: "Outer", Groups: second.Groups}}
			set := coverageSetForEntries(t, provider, first, second)
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			require.True(t, ok)
			managed.CycleController().BeginCycle()
			meter := store.Write().SnapshotMeter("")
			meter.WithLabels(metrix.Label{Key: "node", Value: "shared"}, metrix.Label{Key: "state", Value: "rejected"}).Gauge("requests").Observe(1)
			meter.WithLabels(metrix.Label{Key: "node", Value: "other"}, metrix.Label{Key: "state", Value: "kept"}).Gauge("requests").Observe(2)
			require.NoError(t, managed.CycleController().CommitCycleSuccess())
			coverage, err := buildChartCoverage(set, store.Read(metrix.ReadRaw(), metrix.ReadFlatten()), nil)
			require.NoError(t, err)
			assert.Equal(t, map[string][]string{"value": {"first", "kept"}}, coverage.ExpectedByContext)
			assert.Equal(t, map[string]map[string]struct{}{"value": {"first": {}, "kept": {}}}, coverage.ActualByContext)
		})
	}
}

func coverageEntry(id, context, selector, name string) chartengine.TemplateEntry {
	return chartengine.TemplateEntry{
		ID: id,
		Groups: []charttpl.Group{{
			Family:  "Test",
			Metrics: []string{"requests"},
			Charts: []charttpl.Chart{{
				Title: "Requests", Context: context, Units: "requests",
				Dimensions: []charttpl.Dimension{{Selector: selector, Name: name}},
			}},
		}},
	}
}

func coverageSetForEntries(t *testing.T, provider string, entries ...chartengine.TemplateEntry) *chartengine.TemplateSet {
	t.Helper()
	if provider == "native" {
		set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{Entries: entries})
		require.NoError(t, err)
		return set
	}
	spec := charttpl.Spec{Version: charttpl.VersionV1}
	for _, entry := range entries {
		spec.Groups = append(spec.Groups, entry.Groups...)
	}
	data, err := spec.MarshalTemplate()
	require.NoError(t, err)
	set, err := chartengine.NewTemplateSetYAML([]byte(data))
	require.NoError(t, err)
	return set
}
