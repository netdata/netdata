// SPDX-License-Identifier: GPL-3.0-or-later

//go:build snmp_topology_fixtures

package parity

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAndResolveManifest(t *testing.T) {
	manifestPath := "../../../../testdata/snmp/enlinkd/nms8003/manifest.yaml"

	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	require.Equal(t, ManifestVersion, manifest.Version)
	require.Len(t, manifest.Scenarios, 1)

	scenario, ok := manifest.FindScenario("nms8003_lldp")
	require.True(t, ok)
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	for _, fixture := range resolved.Fixtures {
		st, err := os.Stat(fixture.WalkFile)
		require.NoError(t, err)
		require.False(t, st.IsDir())
	}

	_, err = os.Stat(resolved.GoldenYAML)
	require.NoError(t, err)
	_, err = os.Stat(resolved.GoldenJSON)
	require.NoError(t, err)
}
