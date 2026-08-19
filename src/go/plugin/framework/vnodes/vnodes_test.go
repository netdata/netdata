// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	nodes := Load("testdata")
	assert.NotNil(t, nodes)
	require.Contains(t, nodes, "first")
	require.Contains(t, nodes, "second")
	assert.Equal(t, "first", nodes["first"].Name)
	assert.Equal(t, "second", nodes["second"].Name)
	assert.NotNil(t, Load("not_exist"))
}

func TestIsStockConfig(t *testing.T) {
	assert.True(t, isStockConfig("/usr/lib/netdata/conf.d/vnodes/test.conf"))
	assert.False(t, isStockConfig("/etc/netdata/vnodes/test.conf"))
}

func TestLoad_IgnoresCustomNameInFileAndUsesHostnameIdentity(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "vnodes.yaml")
	cfg := `
- hostname: host-a
  name: custom-name
  guid: 11111111-2222-3333-4444-555555555555
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o644))

	nodes := Load(dir)
	require.Len(t, nodes, 1)
	require.Contains(t, nodes, "host-a")
	assert.Equal(t, "host-a", nodes["host-a"].Name)
	assert.Equal(t, "host-a", nodes["host-a"].Hostname)
}

func TestLoad_SkipsInvalidConfiguredVnodesIndividually(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "vnodes.yaml")
	cfg := `
- hostname: first
  guid: 11111111-2222-3333-4444-555555555555
- hostname: compact
  guid: 22222222333344445555666666666666
- hostname: bad=name
  guid: 33333333-4444-5555-6666-777777777777
- hostname: unsupported-guid-form
  guid: urn:uuid:44444444-5555-6666-7777-888888888888
- hostname: duplicate-guid
  guid: 11111111222233334444555555555555
- hostname: changed-label-value
  guid: 55555555-6666-7777-8888-999999999999
  labels:
    site: "operator's"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o644))

	nodes := Load(dir)

	require.Len(t, nodes, 2)
	require.Contains(t, nodes, "first")
	require.Contains(t, nodes, "compact")
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", nodes["first"].GUID)
	assert.Equal(t, "22222222333344445555666666666666", nodes["compact"].GUID)
}

func TestLoad_SkipsVNodeWhoseSourceCannotBePublished(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "operator's")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	cfgPath := filepath.Join(dir, "vnodes.yaml")
	cfg := `
- hostname: host-a
  guid: 11111111-2222-3333-4444-555555555555
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o644))

	require.Empty(t, Load(dir))
}

func TestValidateConfiguredRejectsLabelNormalization(t *testing.T) {
	tests := map[string]struct {
		labels  map[string]string
		wantErr string
	}{
		"trimmed key": {
			labels:  map[string]string{" region ": "east"},
			wantErr: "label key",
		},
		"normalization collision": {
			labels:  map[string]string{" region ": "east", "region": "west"},
			wantErr: "label key",
		},
		"single quote value": {
			labels:  map[string]string{"site": "operator's"},
			wantErr: "label value",
		},
		"line feed value": {
			labels:  map[string]string{"site": "east\nwest"},
			wantErr: "label value",
		},
		"carriage return value": {
			labels:  map[string]string{"site": "east\rwest"},
			wantErr: "label value",
		},
		"NUL value": {
			labels:  map[string]string{"site": "east\x00west"},
			wantErr: "label value",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			vnode := &VirtualNode{
				Name:       "host-a",
				Hostname:   "host-a",
				GUID:       "11111111-2222-3333-4444-555555555555",
				Labels:     test.labels,
				Source:     "file=/etc/netdata/vnodes/vnodes.yaml",
				SourceType: "user",
			}

			require.ErrorContains(t, ValidateConfigured(vnode), test.wantErr)
		})
	}
}
