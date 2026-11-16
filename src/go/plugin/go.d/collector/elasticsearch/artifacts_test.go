// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The Elasticsearch and OpenSearch pages document every chart the collector can create, and the stock alerts read
// documented dimensions.
func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/elasticsearch.conf")

	charts, err := collecttest.V1ChartsByContext(slices.Concat(
		nodeChartsTmpl,
		clusterHealthChartsTmpl,
		clusterStatsChartsTmpl,
		nodeIndexChartsTmpl,
	))
	require.NoError(t, err)

	for _, id := range []string{"collector-go.d.plugin-elasticsearch", "collector-go.d.plugin-opensearch"} {
		t.Run(id, func(t *testing.T) {
			module, err := collecttest.DecodeMetadataModule(metadata, id)
			require.NoError(t, err)
			require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, charts, nil))
			collecttest.AssertHealthAlertsMatchMetadataWith(t, health, metadata, collecttest.HealthAlertsCheck{ModuleID: id})
		})
	}
}
