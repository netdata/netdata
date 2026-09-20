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
	for _, scoped := range coverages {
		if scoped.ScopeKey == "remote" {
			assert.Equal(t, map[string][]string{"beta.value": {"value"}}, scoped.Coverage.ExpectedByContext)
		}
	}
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
	}
}
