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
