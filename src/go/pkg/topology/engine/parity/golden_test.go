// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGoldenCache(t *testing.T) {
	manifest, err := LoadManifest("../testdata/enlinkd/nms8003/manifest.yaml")
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms8003_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario("../testdata/enlinkd/nms8003/manifest.yaml", scenario)
	require.NoError(t, err)

	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS8000(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms8000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{"nms8000_cdp", "nms8000_lldp"}
	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS13637(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13637/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13637_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS10205B(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms10205b/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms10205b_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS17216(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms17216/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{"nms17216_lldp", "nms17216_cdp"}
	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS0123(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0123/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0123_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS0002(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0002/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{"nms0002_cisco_juniper_lldp", "nms0002_cisco_alcatel_lldp"}
	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS0000(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{
		"nms0000_network_all_lldp",
		"nms0000_network_two_connected_lldp",
		"nms0000_network_three_connected_lldp",
		"nms0000_microsense_lldp",
		"nms0000_ms16_lldp",
		"nms0000_planet_lldp",
	}

	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS7467(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7467/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7467_cdp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS7563(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7563/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{"nms7563_cisco01", "nms7563_homeserver_lldp", "nms7563_switch02_cdp"}
	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS4930(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms4930/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{
		"nms4930_dlink1_bridge_fdb",
		"nms4930_dlink2_bridge_fdb",
	}

	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS7777DW(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7777dw/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7777dw_lldp_no_links")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS13923(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13923/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13923_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS13593(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13593/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13593_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
}

func TestValidateGoldenCache_NMS7918(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{
		"nms7918_asw01_bridge_fdb",
		"nms7918_stcasw01_bridge_fdb",
		"nms7918_samasw01_bridge_fdb",
		"nms7918_ospwl01_arp",
		"nms7918_ospess01_arp",
		"nms7918_pe01_arp",
	}

	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestValidateGoldenCache_NMS18541(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenarios := []string{
		"nms18541_network_all_lldp",
		"nms18541_topoqfx_sw01_sw02_sw03_lldp",
		"nms18541_topoqfx_sw01_lldp",
		"nms18541_topoqfx_sw02_lldp",
		"nms18541_topoqfx_sw03_lldp",
		"nms18541_topoqfx_sw04_lldp",
		"nms18541_topoqfx_sw08_lldp",
		"nms18541_topoqfx_sw09_lldp",
		"nms18541_qfx_lldp",
		"nms18541_microsens_sw01_lldp",
		"nms18541_microsens_sw02_lldp",
		"nms18541_microsens_sw03_lldp",
		"nms18541_microsens_sw04_lldp",
		"nms18541_microsens_sw08_lldp",
		"nms18541_microsens_sw09_lldp",
	}

	for _, scenarioID := range scenarios {
		scenario, ok := manifest.FindScenario(scenarioID)
		require.True(t, ok)

		resolved, resolveErr := ResolveScenario(manifestPath, scenario)
		require.NoError(t, resolveErr)
		require.NoError(t, ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON))
	}
}

func TestGoldenYAMLValidation(t *testing.T) {
	doc, err := LoadGoldenYAML("../testdata/enlinkd/nms8003/golden/nms8003_lldp.yaml")
	require.NoError(t, err)

	require.Equal(t, GoldenVersion, doc.Version)
	require.Equal(t, "nms8003_lldp", doc.ScenarioID)
	require.Len(t, doc.Devices, 5)
	require.Len(t, doc.Adjacencies, 12)
	require.Equal(t, 6, doc.Expectations.BidirectionalPairs)

	payload, err := doc.CanonicalJSON()
	require.NoError(t, err)
	require.Contains(t, string(payload), "\"scenario_id\": \"nms8003_lldp\"")
}
