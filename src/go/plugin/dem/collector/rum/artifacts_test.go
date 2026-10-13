// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestShippedArtifacts(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, charts)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(
		t,
		metadata,
		charts,
		map[string][]string{"ingress_state": {"available", "unavailable"}},
	)
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
	health, err := os.ReadFile("../../../../../health/health.d/dem.conf")
	require.NoError(t, err)
	collecttest.AssertHealthAlertsTargetChartTemplateWith(t, health, charts, collecttest.HealthAlertsCheck{ContextPrefix: "rum."})
	collecttest.AssertMetadataAlertsMatchHealthConfigWith(t, metadata, health, collecttest.MetadataAlertsCheck{SharedHealthConfig: true})
}
