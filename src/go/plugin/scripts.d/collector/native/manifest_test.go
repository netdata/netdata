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
		"valid": {manifest: fixtureManifest},
		"mode":  {manifest: fixtureManifest + "mode: push\n", wantErr: true},
		"version": {
			manifest: strings.Replace(fixtureManifest, "version: v1", "version: v2", 1),
			wantErr:  true,
		},
		"unknown field":                         {manifest: fixtureManifest + "typo: true\n", wantErr: true},
		"second document":                       {manifest: fixtureManifest + "---\nversion: v1\n", wantErr: true},
		"legacy metric declarations":            {manifest: fixtureManifest + "metrics: []\n", wantErr: true},
		"legacy check declarations":             {manifest: fixtureManifest + "checks: []\n", wantErr: true},
		"disabled collection without Functions": {manifest: fixtureManifest + "collect: false\n", wantErr: true},
		"disabled collection with charts": {
			manifest: fixtureManifest + "collect: false\nfunctions: [{id: items, name: Items, help: Show items.}]\n",
			wantErr:  true,
		},
		"empty command": {
			manifest: strings.Replace(fixtureManifest, "command: [./collect.sh]", "command: []", 1),
			wantErr:  true,
		},
		"no declaration": {manifest: "version: v1\ncommand: [./collect.sh]\n"},
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
			assert.Equal(t, !definition.functionOnly(), definition.templates != nil)
		})
	}
}
