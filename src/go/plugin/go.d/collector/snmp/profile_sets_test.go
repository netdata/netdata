// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestSelectCollectionProfiles_RemovesTopologyPollWork(t *testing.T) {
	profiles := []*ddsnmp.Profile{newMixedTopologyProfile()}

	selected := selectCollectionProfiles(profiles)
	require.Len(t, selected, 1)

	prof := selected[0]
	require.NotNil(t, prof.Definition)
	require.Len(t, prof.Definition.Metrics, 1)
	assert.Equal(t, "upsBatteryStatus", prof.Definition.Metrics[0].Symbol.Name)
	require.Len(t, prof.Definition.VirtualMetrics, 1)
	assert.Equal(t, "upsBatteryStatusTotal", prof.Definition.VirtualMetrics[0].Name)
	require.Len(t, prof.Definition.MetricTags, 1)
	assert.Equal(t, "ups_model", prof.Definition.MetricTags[0].Tag)
	require.Len(t, prof.Definition.Metadata, 1)
	require.Len(t, prof.Definition.SysobjectIDMetadata, 1)
}

func TestSelectTopologyRefreshProfiles_KeepsOnlyTopologyData(t *testing.T) {
	profiles := []*ddsnmp.Profile{newMixedTopologyProfile()}

	selected := selectTopologyRefreshProfiles(profiles)
	require.Len(t, selected, 1)

	prof := selected[0]
	require.NotNil(t, prof.Definition)
	require.Len(t, prof.Definition.Metrics, 1)
	assert.Equal(t, metricLldpLocPortEntry, prof.Definition.Metrics[0].Symbol.Name)
	require.Len(t, prof.Definition.VirtualMetrics, 1)
	assert.Equal(t, "lldpLocalPortRows", prof.Definition.VirtualMetrics[0].Name)
	require.Len(t, prof.Definition.MetricTags, 1)
	assert.Equal(t, "lldp_loc_chassis_id", prof.Definition.MetricTags[0].Tag)
	assert.Empty(t, prof.Definition.Metadata)
	assert.Empty(t, prof.Definition.SysobjectIDMetadata)
}

func TestCollectorEnsureInitializedAllowsTopologyOnlyProfiles(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)
	mockSNMP.EXPECT().Close().Return(nil).AnyTimes()

	coll := New()
	coll.Config = prepareV2Config()
	coll.Ping.Enabled = false
	coll.CreateVnode = false
	coll.snmpClient = mockSNMP
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	coll.snmpProfiles = []*ddsnmp.Profile{}
	coll.topologyProfiles = []*ddsnmp.Profile{
		{
			Definition: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Symbol: ddprofiledefinition.SymbolConfig{Name: metricLldpLocPortEntry},
					},
				},
			},
		},
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector { return &mockDdSnmpCollector{} }

	require.NoError(t, coll.ensureInitialized())
	assert.Nil(t, coll.ddSnmpColl)
	assert.NotNil(t, coll.sysInfo)

	coll.stopTopologyScheduler()
	coll.Cleanup(context.Background())
}

func newMixedTopologyProfile() *ddsnmp.Profile {
	return &ddsnmp.Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"model": {
							Symbol: ddprofiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "deviceModel"},
						},
					},
				},
			},
			SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
				{
					SysobjectID: ".1.3.6.1.4.1.1",
					Metadata: map[string]ddprofiledefinition.MetadataField{
						"vendor": {Value: "test"},
					},
				},
			},
			MetricTags: []ddprofiledefinition.MetricTagConfig{
				{
					Tag: "lldp_loc_chassis_id",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						Name: "lldpLocChassisId",
					},
				},
				{
					Tag: "ups_model",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						Name: "upsModel",
					},
				},
			},
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.0.8802.1.1.2.1.3.7.1.2",
						Name: metricLldpLocPortEntry,
					},
				},
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.33.1.2.1.0",
						Name: "upsBatteryStatus",
					},
				},
			},
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{
					Name: "lldpLocalPortRows",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{Metric: metricLldpLocPortEntry, Table: "lldpLocPortTable"},
					},
				},
				{
					Name: "upsBatteryStatusTotal",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{Metric: "upsBatteryStatus", Table: "upsBatteryTable"},
					},
				},
			},
		},
	}
}
