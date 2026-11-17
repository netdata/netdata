// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The integration page documents every chart the collector can create, with the volume manager chart shown for a
// plugin named "plugin".
func TestArtifacts(t *testing.T) {
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)

	volumeManager := newVolumeManagerChart("plugin")
	// the runtime title names the volume plugin; the page documents the common part
	volumeManager.Title = "Volume Manager State Of The World"

	charts, err := collecttest.V1ChartsByContext(slices.Concat(
		*charts.Copy(),
		Charts{volumeManager},
		*pvcSpaceChartsTmpl.Copy(),
		*pvcInodesChartsTmpl.Copy(),
	))
	require.NoError(t, err)

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, charts, nil))
}
