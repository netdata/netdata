// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The integration page documents every chart the collector can create in its default mode, with per-thread charts
// shown for a thread named "thread", and the stock alerts read documented dimensions.
func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/unbound.conf")

	cumulative := New().Cumulative
	charts, err := collecttest.V1ChartsByContext(slices.Concat(
		*charts(cumulative),
		*extendedCharts(cumulative),
		*threadCharts("thread", cumulative),
	))
	require.NoError(t, err)

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, charts, nil))
	collecttest.AssertHealthAlertsMatchMetadata(t, health, metadata)
}
