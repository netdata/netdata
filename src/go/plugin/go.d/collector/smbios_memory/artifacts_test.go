// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestArtifacts(t *testing.T) {
	read := func(path string) []byte { data, err := os.ReadFile(path); require.NoError(t, err); return data }
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 60)
	require.NoError(t, err)
	metadata := read("metadata.yaml")
	health := read("../../../../../health/health.d/smbios_memory.conf")
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, map[string][]string{
		"inventory_status": inventoryStates, "comparison_status": comparisonStates, "confirmed_loss_status": lossStates,
	})
	collecttest.AssertHealthAlertsTargetChartTemplate(t, health, chartTemplateYAML)
	collecttest.AssertHealthAlertsMatchMetadata(t, health, metadata)
	collecttest.AssertMetadataAlertsMatchHealthConfig(t, metadata, health)
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml", collecttest.ConfigSchemaCheck{Defaults: true})
}

// The Live Data section mirrors the Function: ids, parameters and returned columns.
func TestMetadataDocumentsFunctions(t *testing.T) {
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	h := newFixtureHost(t)
	c := h.collector(t)
	cycle(t, c)
	collecttest.AssertMetadataDocumentsFunctions(t, metadata, collecttest.MetadataFunctionsCheck{
		Context: t.Context(),
		Module:  "smbios_memory",
		Methods: smbiosMethods(),
		Handler: c.funcRouter,
	})
}
