// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
