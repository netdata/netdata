// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// The integration page documents every chart the collector can create.
func TestArtifacts(t *testing.T) {
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)

	charts, err := collecttest.V1ChartsByContext(ifaceModuleEepromCharts)
	require.NoError(t, err)

	module, err := collecttest.DecodeMetadataModule(metadata, "")
	require.NoError(t, err)
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(module.Metrics, charts, nil))
}
