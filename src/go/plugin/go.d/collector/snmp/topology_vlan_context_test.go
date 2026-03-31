// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestWithTopologyVLANContextTags_AddsContextFields(t *testing.T) {
	tags := withTopologyVLANContextTags(map[string]string{"key": "value"}, "200", "servers")
	require.Equal(t, "value", tags["key"])
	require.Equal(t, "200", tags[tagTopologyContextVLANID])
	require.Equal(t, "servers", tags[tagTopologyContextVLANName])
}

func TestIsTopologyVLANContextMetric_FiltersSupportedMetrics(t *testing.T) {
	require.True(t, isTopologyVLANContextMetric(metricFdbEntry))
	require.True(t, isTopologyVLANContextMetric(metricStpPortEntry))
	require.False(t, isTopologyVLANContextMetric(metricCdpCacheEntry))
}

func TestLoadTopologyVLANContextProfiles_LoadsFDBAndSTPProfiles(t *testing.T) {
	profiles, err := loadTopologyVLANContextProfiles()
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	require.True(t, profilesHaveExtension(profiles, fdbArpProfileName))
	require.True(t, profilesHaveExtension(profiles, stpProfileName))
}

func TestCollectorInitTopologyVLANClient_V2UsesCommunitySuffix(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	mockSNMP.EXPECT().SetTarget(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetPort(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetRetries(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetLogger(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetVersion(gosnmp.Version2c).Times(1)
	mockSNMP.EXPECT().SetCommunity("public").Times(1)
	mockSNMP.EXPECT().SetCommunity("public@200").Times(1)
	mockSNMP.EXPECT().SetMaxRepetitions(uint32(25)).Times(1)
	mockSNMP.EXPECT().SetMaxRepetitions(uint32(17)).Times(1)
	mockSNMP.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockSNMP.EXPECT().Community().Return("public").AnyTimes()
	mockSNMP.EXPECT().Connect().Return(nil).Times(1)
	mockSNMP.EXPECT().Close().Return(nil).Times(1)

	coll := New()
	coll.Config = prepareV2Config()
	coll.adjMaxRepetitions = 17
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

	client, err := coll.initTopologyVLANClient("200")
	require.NoError(t, err)
	require.Same(t, mockSNMP, client)
	require.NoError(t, client.Close())
}

func TestCollectorInitTopologyVLANClient_V3UsesContextName(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	mockSNMP.EXPECT().SetTarget(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetPort(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetRetries(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetLogger(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetMaxRepetitions(uint32(25)).Times(1)
	mockSNMP.EXPECT().SetVersion(gosnmp.Version3).Times(1)
	mockSNMP.EXPECT().SetSecurityModel(gomock.Any()).Times(1)
	mockSNMP.EXPECT().SetMsgFlags(gomock.Any()).Times(1)
	mockSNMP.EXPECT().SetSecurityParameters(gomock.Any()).Times(1)
	mockSNMP.EXPECT().Version().Return(gosnmp.Version3).AnyTimes()
	mockSNMP.EXPECT().SetContextName("vlan-200").Times(1)
	mockSNMP.EXPECT().Connect().Return(nil).Times(1)
	mockSNMP.EXPECT().Close().Return(nil).Times(1)

	coll := New()
	coll.Config = prepareV3Config()
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

	client, err := coll.initTopologyVLANClient("200")
	require.NoError(t, err)
	require.Same(t, mockSNMP, client)
	require.NoError(t, client.Close())
}

func TestCollectorCollectTopologyVLANContextRejectsInvalidVLANID(t *testing.T) {
	coll := New()
	coll.Config = prepareV2Config()

	profiles := []*ddsnmp.Profile{{}}

	_, err := coll.collectTopologyVLANContext("", profiles)
	require.EqualError(t, err, "empty vlan id")

	_, err = coll.collectTopologyVLANContext("abc", profiles)
	require.EqualError(t, err, "invalid vlan id 'abc': strconv.Atoi: parsing \"abc\": invalid syntax")
}

func TestCollectorCollectTopologyVLANContextBuildsDedicatedCollector(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	mockSNMP.EXPECT().SetTarget(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetPort(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetRetries(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetLogger(gomock.Any()).AnyTimes()
	mockSNMP.EXPECT().SetVersion(gosnmp.Version2c).Times(1)
	mockSNMP.EXPECT().SetCommunity("public").Times(1)
	mockSNMP.EXPECT().SetCommunity("public@300").Times(1)
	mockSNMP.EXPECT().SetMaxRepetitions(uint32(25)).Times(1)
	mockSNMP.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockSNMP.EXPECT().Community().Return("public").AnyTimes()
	mockSNMP.EXPECT().Connect().Return(nil).Times(1)
	mockSNMP.EXPECT().Close().Return(nil).Times(1)

	coll := New()
	coll.Config = prepareV2Config()
	coll.sysInfo = &snmputils.SysInfo{SysObjectID: "1.2.3.4.5"}
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

	profiles := []*ddsnmp.Profile{
		{
			SourceFile: fdbArpProfileName,
			Definition: &ddprofiledefinition.ProfileDefinition{},
		},
	}

	expected := []*ddsnmp.ProfileMetrics{{}}
	captured := ddsnmpcollector.Config{}
	coll.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		captured = cfg
		return &mockDdSnmpCollector{pms: expected}
	}

	pms, err := coll.collectTopologyVLANContext("300", profiles)
	require.NoError(t, err)
	require.Equal(t, expected, pms)
	require.Same(t, mockSNMP, captured.SnmpClient)
	require.Equal(t, "1.2.3.4.5", captured.SysObjectID)
	require.True(t, profilesHaveExtension(captured.Profiles, fdbArpProfileName))
}

func TestCollectorCollectTopologyVTPVLANContextsIngestsPerVLANMetrics(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	setMockClientSetterExpect(mockSNMP)
	mockSNMP.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockSNMP.EXPECT().Community().Return("public").AnyTimes()
	mockSNMP.EXPECT().Connect().Return(nil).Times(1)
	mockSNMP.EXPECT().Close().Return(nil).Times(1)

	coll := New()
	coll.Config = prepareV2Config()
	coll.sysInfo = &snmputils.SysInfo{Name: "switch-a", SysObjectID: "1.2.3.4.5"}
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	coll.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		require.True(t, profilesHaveExtension(cfg.Profiles, fdbArpProfileName))
		require.True(t, profilesHaveExtension(cfg.Profiles, stpProfileName))
		return &mockDdSnmpCollector{
			pms: []*ddsnmp.ProfileMetrics{
				{
					Metrics: []ddsnmp.Metric{
						{
							Name: metricFdbEntry,
							Tags: map[string]string{
								tagFdbMac:        "00:11:22:33:44:55",
								tagFdbBridgePort: "7",
							},
						},
					},
				},
			},
		}
	}

	coll.topologyCache.mu.Lock()
	coll.topologyCache.vlanIDToName["200"] = "servers"
	coll.topologyCache.mu.Unlock()

	coll.collectTopologyVTPVLANContexts()

	coll.topologyCache.mu.RLock()
	defer coll.topologyCache.mu.RUnlock()

	require.Len(t, coll.topologyCache.fdbEntries, 1)
	for _, entry := range coll.topologyCache.fdbEntries {
		require.Equal(t, "200", entry.vlanID)
		require.Equal(t, "servers", entry.vlanName)
		require.Equal(t, "7", entry.bridgePort)
		require.Equal(t, "00:11:22:33:44:55", entry.mac)
	}
}

func TestCollectorCollectTopologyVTPVLANContextsSkipsFailedVLANCollections(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	setMockClientSetterExpect(mockSNMP)
	mockSNMP.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockSNMP.EXPECT().Community().Return("public").AnyTimes()
	mockSNMP.EXPECT().Connect().Return(nil).Times(2)
	mockSNMP.EXPECT().Close().Return(nil).Times(2)

	coll := New()
	coll.Config = prepareV2Config()
	coll.sysInfo = &snmputils.SysInfo{Name: "switch-a", SysObjectID: "1.2.3.4.5"}
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

	collectCalls := 0
	coll.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		collectCalls++
		if collectCalls == 1 {
			return &mockDdSnmpCollector{
				err: fmt.Errorf("walk failed"),
			}
		}
		return &mockDdSnmpCollector{
			pms: []*ddsnmp.ProfileMetrics{
				{
					Metrics: []ddsnmp.Metric{
						{
							Name: metricStpPortEntry,
							Tags: map[string]string{
								tagStpPort:                 "9",
								tagStpPortState:            "forwarding",
								tagStpPortDesignatedBridge: "800066778899aabb",
								tagStpPortDesignatedPort:   "0009",
							},
						},
					},
				},
			},
		}
	}

	coll.topologyCache.mu.Lock()
	coll.topologyCache.vlanIDToName["100"] = "bad"
	coll.topologyCache.vlanIDToName["200"] = "good"
	coll.topologyCache.mu.Unlock()

	coll.collectTopologyVTPVLANContexts()

	coll.topologyCache.mu.RLock()
	defer coll.topologyCache.mu.RUnlock()

	require.Len(t, coll.topologyCache.stpPorts, 1)
	entry := coll.topologyCache.stpPorts["9|vlan:200"]
	require.NotNil(t, entry)
	require.Equal(t, "200", entry.vlanID)
	require.Equal(t, "good", entry.vlanName)
}
