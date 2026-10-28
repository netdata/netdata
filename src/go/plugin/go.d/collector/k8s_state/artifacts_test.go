// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The Kubernetes Cluster State page documents every chart the collector can create (but the hidden discovery state
// chart), and the stock alerts read documented dimensions.
func TestArtifacts(t *testing.T) {
	read := func(path string) []byte {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return data
	}
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/k8sstate.conf")

	charts, err := collecttest.V1ChartsByContext(slices.Concat(
		nodeChartsTmpl,
		podChartsTmpl,
		containerChartsTmpl,
		deploymentChartsTmpl,
		cronJobChartsTmpl,
		pvcChartsTmpl,
	))
	require.NoError(t, err)

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, charts, nil))
	collecttest.AssertHealthAlertsMatchMetadata(t, health, metadata)
}
