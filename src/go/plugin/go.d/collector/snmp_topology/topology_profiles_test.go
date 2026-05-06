// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProfiles_UsesDeclarativeTopologyExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sysObjectID string
		sysDescr    string
		extensions  []string
	}{
		{
			name:        "Cisco",
			sysObjectID: "1.3.6.1.4.1.9.1.1",
			extensions:  []string{topologyLldpProfileName, cdpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName, vtpProfileName},
		},
		{
			name:        "Cisco Small Business",
			sysObjectID: "1.3.6.1.4.1.9.6.1.94.24.5",
			extensions:  []string{topologyLldpProfileName, cdpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "Aruba",
			sysObjectID: "1.3.6.1.4.1.47196.4.1.1.1.50",
			sysDescr:    "Aruba JL635A 8325 GL.10.04.2000",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "Arista",
			sysObjectID: "1.3.6.1.4.1.30065.1.3011.7050.1958.128",
			sysDescr:    "Arista Networks EOS version 4.15.3F running on an Arista Networks DCS-7050TX-128",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "Juniper",
			sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.39",
			sysDescr:    "Juniper SRX240B gsm-fw",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "MikroTik",
			sysObjectID: "1.3.6.1.4.1.14988.1",
			sysDescr:    "RouterOS CRS326-24G-2S+",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "Zyxel",
			sysObjectID: "1.3.6.1.4.1.890.1.15",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
		{
			name:        "D-Link",
			sysObjectID: "1.3.6.1.4.1.171.10.137.1.1",
			extensions:  []string{topologyLldpProfileName, fdbArpProfileName, qBridgeProfileName, stpProfileName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			profiles := ddsnmp.FindProfiles(tt.sysObjectID, tt.sysDescr, nil)
			require.NotEmpty(t, profiles)

			var found bool
			for _, prof := range profiles {
				if prof == nil || !prof.HasExtension(topologyLldpProfileName) {
					continue
				}
				for _, ext := range tt.extensions {
					assert.Truef(t, prof.HasExtension(ext), "expected extension %q for %s", ext, tt.name)
				}
				found = true
			}

			assert.Truef(t, found, "no topology-enabled profile matched %s", tt.name)
		})
	}
}
