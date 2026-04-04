// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestCollector_Collect_CumulusBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.40310", "nvidia-cumulus-linux-switch.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"bgpPeerTable": {
			"bgpPeerAdminStatus",
			"bgpPeerState",
			"bgpPeerInUpdates",
			"bgpPeerOutUpdates",
			"bgpPeerLastErrorCode",
			"bgpPeerLastErrorSubcode",
			"bgpPeerFsmEstablishedTime",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/pfsense_frr_bgp_peer_table.snmprec")
	pdus = append(pdus,
		createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.1", gosnmp.IPAddress, "169.254.1.1"),
		createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.9", gosnmp.IPAddress, "169.254.1.9"),
	)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"bgpPeerTable": pdus,
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.40310",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"neighbor":  "169.254.1.1",
		"remote_as": "4200000000",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"neighbor":  "169.254.1.1",
		"remote_as": "4200000000",
	})
	assert.Equal(t, map[string]int64{"received": 6, "sent": 14}, updates.MultiValue)

	lastErrorCode := requireMetricWithTags(t, metrics, "bgpPeerLastErrorCode", map[string]string{
		"neighbor":  "169.254.1.1",
		"remote_as": "4200000000",
	})
	assert.EqualValues(t, 4, lastErrorCode.Value)

	lastErrorSubcode := requireMetricWithTags(t, metrics, "bgpPeerLastErrorSubcode", map[string]string{
		"neighbor":  "169.254.1.9",
		"remote_as": "4200000004",
	})
	assert.EqualValues(t, 2, lastErrorSubcode.Value)

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"neighbor":  "169.254.1.1",
		"remote_as": "4200000000",
	})
	assert.EqualValues(t, 96951, establishedTime.Value)
}
