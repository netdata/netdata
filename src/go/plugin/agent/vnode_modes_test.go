// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

type unusedVnodeAcquirer struct{}

func (unusedVnodeAcquirer) Acquire(context.Context, vnodes.SNMPConfig) (*vnodes.Metadata, error) {
	panic("loading must not acquire")
}
func TestSharedVnodeFilesDoNotBreakPluginsWithoutSNMPAcquisition(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "vnodes")
	require.NoError(t, os.Mkdir(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "devices.yaml"), []byte(`
- hostname: static-node
  guid: 11111111-2222-3333-4444-555555555555
- name: snmp-device
  mode: snmp
  mode_snmp:
    address: device
    credentials:
      community: fixture
`), 0600))
	for _, name := range []string{"scripts.d", "ibm.d", "go.d"} {
		t.Run(name, func(t *testing.T) {
			config := Config{Name: name, PluginConfigDir: []string{root}}
			if name == "go.d" {
				config.SNMPVnodeAcquirer = unusedVnodeAcquirer{}
			}
			a := New(config)
			loaded := a.setupVnodeRegistry()
			require.Contains(t, loaded, "static-node")
			if name == "go.d" {
				require.Contains(t, loaded, "snmp-device")
			} else {
				require.NotContains(t, loaded, "snmp-device", "shared SNMP entries must not reach unsupported plugin startup")
			}
		})
	}
}
