// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The integration page mirrors the shipped artifacts: option rows equal the DynCfg form, metric rows
// equal the chart definitions, and alert rows equal the health configuration.
// (The health configuration's own consistency with the charts is not asserted here: its alert units
// use `%` where the charts say `percentage`.)
func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/mssql.conf")

	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml", collecttest.ConfigSchemaCheck{Defaults: true})
	collecttest.AssertMetadataAlertsMatchHealthConfig(t, metadata, health)

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, everyChartByContext(t), nil))
}

// everyChartByContext instantiates one chart from every template the collector can create, on a
// SQL Server version new enough to pass every version gate, and returns them keyed by context.
func everyChartByContext(t *testing.T) map[string]charttpl.Chart {
	t.Helper()

	c := New()
	c.setServerProperties("16.0.4175.1", 0)

	c.addDatabaseCharts("db")
	c.addDatabaseLogCharts("db")
	c.addWaitTypeCharts("PAGEIOLATCH_SH", "io")
	c.addLockResourceCharts("OBJECT")
	c.addLockStatsCharts("OBJECT")
	job := sqlAgentJob{id: "job-1", name: "nightly", chartID: "nightly", enabled: true}
	c.ensureJobStatusChart(job)
	c.ensureJobExecutionCharts(job)
	c.addReplicationCharts("db", "pub")
	c.addAGCharts("ag")
	c.addAGReplicaCharts("ag", "node1", "SYNCHRONOUS_COMMIT", "AUTOMATIC")
	c.addAGDatabaseReplicaCharts("ag", "node1", "db")
	require.NoError(t, c.Charts().Add(agClusterQuorumStateChart.Copy()))
	c.addAGClusterMemberCharts("node1")
	c.addAGPageRepairCharts("db")

	charts := make(map[string]charttpl.Chart)
	for _, chart := range *c.Charts() {
		require.NotContainsf(t, charts, chart.Ctx, "context %q instantiated twice", chart.Ctx)
		charts[chart.Ctx] = chartTemplate(chart)
	}
	return charts
}

func chartTemplate(chart *collectorapi.Chart) charttpl.Chart {
	typ := string(chart.Type)
	if typ == "" {
		typ = string(collectorapi.Line)
	}
	out := charttpl.Chart{Context: chart.Ctx, Title: chart.Title, Units: chart.Units, Type: typ}
	for _, dim := range chart.Dims {
		out.Dimensions = append(out.Dimensions, charttpl.Dimension{Name: dim.Name})
	}
	return out
}
