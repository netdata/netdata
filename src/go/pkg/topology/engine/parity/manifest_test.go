// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAndResolveManifest(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms8003/manifest.yaml"

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

func TestLoadManifest_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	badManifest := `version: v1
scenarios:
  - id: duplicate
    protocols: {lldp: true}
    fixtures:
      - device_id: d1
        walk_file: fixture1.txt
    golden_yaml: golden.yaml
    golden_json: golden.json
  - id: duplicate
    protocols: {lldp: true}
    fixtures:
      - device_id: d2
        walk_file: fixture2.txt
    golden_yaml: golden2.yaml
    golden_json: golden2.json
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(badManifest), 0o644))

	_, err := LoadManifest(manifestPath)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate scenario id")
}
