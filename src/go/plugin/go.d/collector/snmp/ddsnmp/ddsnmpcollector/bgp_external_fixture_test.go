// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_GenericBGP4Rows_WithMatchedStandardProfile(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.40310", "nvidia-cumulus-linux-switch.yaml")
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.Metrics = nil
	profile.Definition.VirtualMetrics = nil

	// LibreNMS does not currently ship a Cumulus peer-table capture with BGP rows.
	// Use a real generic BGP4-MIB fixture to verify the standard BGP mixin after
	// production profile matching.
	pdus := loadSnmprecPDUs(t, "librenms/pfsense_frr_bgp_peer_table.snmprec")
	pdus = append(pdus,
		createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.1", gosnmp.IPAddress, "169.254.1.1"),
		createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.9", gosnmp.IPAddress, "169.254.1.9"),
	)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.15.3", pdus)

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.40310",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Empty(t, pm.Metrics)
	require.Len(t, pm.BGPRows, 2)

	rows := bgpRowsByNeighbor(pm.BGPRows)
	tests := map[string]struct {
		neighbor string
		validate func(t *testing.T, row ddsnmp.BGPRow)
	}{
		"established FRR peer": {
			neighbor: "169.254.1.1",
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, "_std-bgp4-mib.yaml", row.OriginProfileID)
				assert.Equal(t, "4200000000", row.Identity.RemoteAS)
				assert.True(t, row.Admin.Enabled.Value)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
				assert.EqualValues(t, 6, row.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 14, row.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 4, row.LastError.Code.Value)
				assert.EqualValues(t, 0, row.LastError.Subcode.Value)
				assert.EqualValues(t, 96951, row.Connection.EstablishedUptime.Value)
			},
		},
		"second FRR peer": {
			neighbor: "169.254.1.9",
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, "4200000004", row.Identity.RemoteAS)
				assert.EqualValues(t, 2, row.LastError.Code.Value)
				assert.EqualValues(t, 2, row.LastError.Subcode.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := rows[tc.neighbor]
			require.NotZero(t, row)
			tc.validate(t, row)
		})
	}
}
