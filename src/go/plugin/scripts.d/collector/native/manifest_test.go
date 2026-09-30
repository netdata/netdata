// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadManifest(t *testing.T) {
	tests := map[string]struct {
		manifest string
		wantErr  bool
	}{
		"valid":           {manifest: fixtureManifest},
		"mode":            {manifest: fixtureManifest + "mode: push\n", wantErr: true},
		"version":         {manifest: strings.Replace(fixtureManifest, "version: v1", "version: v2", 1), wantErr: true},
		"unknown field":   {manifest: fixtureManifest + "typo: true\n", wantErr: true},
		"second document": {manifest: fixtureManifest + "---\nversion: v1\n", wantErr: true},
		"duplicate metric": {
			manifest: strings.Replace(
				fixtureManifest,
				"name: processed_total, type: counter",
				"name: depth, type: gauge",
				1,
			),
			wantErr: true,
		},
		"reserved metric": {
			manifest: strings.Replace(fixtureManifest, "name: depth", "name: native.check.test", 1),
			wantErr:  true,
		},
		"invalid type": {manifest: strings.Replace(fixtureManifest, "type: gauge", "type: typo", 1), wantErr: true},
		"missing unit": {manifest: strings.Replace(fixtureManifest, "unit: jobs}", "unit: ' '}", 1), wantErr: true},
		"duplicate identity label": {
			manifest: strings.Replace(fixtureManifest, "by_labels: [queue]", "by_labels: [queue, queue]", 1),
			wantErr:  true,
		},
		"empty command": {
			manifest: strings.Replace(fixtureManifest, "command: [./collect.sh]", "command: []", 1),
			wantErr:  true,
		},
		"no declaration": {manifest: "version: v1\ncommand: [./collect.sh]\n", wantErr: true},
		"reserved chart ID": {
			manifest: strings.Replace(fixtureManifest, "charts: charts.yaml", "charts: reserved.yaml", 1),
			wantErr:  true,
		},
		"missing config schema": {manifest: fixtureManifest + "config_schema: missing.json\n", wantErr: true},
		"secret file":           {manifest: "SYNTHETIC_SECRET\n", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "collect.sh"), []byte("#!/bin/sh\n"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "charts.yaml"), []byte(fixtureCharts), 0644))
			reserved := strings.Replace(fixtureCharts, "id: depth", "id: native_check_depth", 1)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "reserved.yaml"), []byte(reserved), 0644))
			path := filepath.Join(dir, "manifest.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.manifest), 0644))

			definition, err := loadManifest(path, statExecutable)
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET")
				return
			}
			require.NoError(t, err)
			assert.Equal(
				t,
				[]string{filepath.Join(dir, "collect.sh")},
				definition.command,
				"command resolves against the manifest",
			)
			assert.Equal(t, modeOneshot, definition.Mode)
			assert.NotNil(t, definition.templates)
		})
	}
}
