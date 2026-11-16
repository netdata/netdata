// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const replacementTestObjectID = "1.3.6.1.4.1.9.1.1166"

func TestCollectorRuntimeDeviceReplacement(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	c.LocalVnode.GUID = "ba37a531-3724-4fa0-9787-166574eb6f6d"
	c.LocalVnode.Hostname = "authored-hostname"
	c.LocalVnode.Labels = map[string]string{"name": "authored name", "contact": ""}
	authored := c.Config
	authored.LocalVnode.Labels = maps.Clone(c.LocalVnode.Labels)
	device.values[refreshTestTable+".1.2"] = gosnmp.SnmpPDU{
		Name:  refreshTestTable + ".1.2",
		Type:  gosnmp.Counter64,
		Value: uint64(50),
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	require.NotEmpty(t, c.Collect(context.Background()))
	require.Len(t, c.ifaceCache.interfaces, 2)
	require.NotNil(t, c.ifaceCache.interfaces["1"].rates.trafficIn)
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	registrationID, guid := entries[0].RegistrationID, c.VirtualNode().GUID
	oldDD, router, interfaces := c.ddSnmpColl, c.funcRouter, c.ifaceCache
	oldTableChart := c.Charts().Get(refreshTestTableKey("ifTraffic", "2"))
	require.NotNil(t, oldTableChart)
	oldStatsChart := c.Charts().Get("snmp_device_prof_refresh_stats_timings")
	require.NotNil(t, oldStatsChart)

	client, clientCloses := newReplacementTestClient(t, device, nil)
	clientCreations := 0
	c.newSnmpClient = func() gosnmp.Handler { clientCreations++; return client }
	var replacement *mockDdSnmpCollector
	ddCreations := 0
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		ddCreations++
		replacement = newReplacementTestDD(t, cfg)
		return replacement
	}
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	device.setText(snmputils.OidSysName, "replacement")
	*now = now.Add(time.Minute)
	mx := c.Collect(context.Background())
	require.NotEmpty(t, mx)
	require.NotNil(t, replacement)
	assert.Equal(t, int64(7), mx[metricIDFromName("replacementLoad")])
	assert.NotContains(t, mx, refreshTestTableKey("ifTraffic", "2"))
	assert.NotSame(t, oldDD, c.ddSnmpColl)
	assert.Same(t, replacement, c.ddSnmpColl)
	assert.Equal(t, 1, replacement.collectCalls, "replacement's first frame must be collected only once")
	assert.Equal(t, 1, clientCreations)
	assert.Equal(t, 1, ddCreations)
	assert.Equal(t, 1, device.closes, "retire the previous connection after replacement")
	assert.Zero(t, *clientCloses, "the installed connection must remain open")
	assert.True(t, oldTableChart.Obsolete)
	assert.True(t, oldStatsChart.Obsolete)
	assert.Same(t, router, c.funcRouter)
	assert.Same(t, interfaces, c.ifaceCache)
	require.Len(t, c.ifaceCache.interfaces, 1)
	require.Contains(t, c.ifaceCache.interfaces, "1")
	assert.Nil(
		t,
		c.ifaceCache.interfaces["1"].rates.trafficIn,
		"a reused interface name cannot inherit the old device's counter baseline",
	)
	assert.Equal(t, guid, c.VirtualNode().GUID)
	assert.Equal(t, "authored-hostname", c.VirtualNode().Hostname)
	assert.Equal(t, "authored name", c.VirtualNode().Labels["name"])
	assert.Empty(t, c.VirtualNode().Labels["contact"])
	assert.Equal(t, authored, c.Configuration())
	entries = c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, registrationID, entries[0].RegistrationID)
	assert.Equal(t, replacementTestObjectID, entries[0].Info.SysObjectID)
	assert.Equal(t, "replacement", entries[0].Info.SysName)
	assert.Equal(t, guid, entries[0].Info.VnodeGUID)

	require.NotEmpty(t, c.Collect(context.Background()))
	assert.NotNil(t, c.ifaceCache.interfaces["1"].rates.trafficIn, "the next sample establishes the new device's rate")
	assert.Equal(t, 1, clientCreations)
	c.Cleanup(context.Background())
	assert.Equal(t, 1, device.closes)
	assert.Equal(t, 1, *clientCloses)
}

func TestCollectorRuntimeDeviceReplacementFailureAndRecovery(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	require.NotEmpty(t, c.Collect(context.Background()))
	interfaceParams := funcapi.ResolvedParams{
		ifacesParamTypeGroup: {IDs: []string{"other"}},
	}
	interfaceRows := func() any {
		return c.funcRouter.Handle(context.Background(), ifacesMethodID, interfaceParams).Data
	}
	require.Len(t, interfaceRows(), 1, "normal source collection must populate the Function before replacement")
	oldDD, guid := c.ddSnmpColl, c.VirtualNode().GUID
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	registrationID := entries[0].RegistrationID
	clientCreations := 0
	failedClient, failedCloses := newReplacementTestClient(t, device, errors.New("synthetic connect failure"))
	recoveredClient, recoveredCloses := newReplacementTestClient(t, device, nil)
	c.newSnmpClient = func() gosnmp.Handler {
		clientCreations++
		if clientCreations == 1 {
			return failedClient
		}
		return recoveredClient
	}
	ddCreations := 0
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		ddCreations++
		return newReplacementTestDD(t, cfg)
	}
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: "1.3.6.1.2.1.999",
	}
	*now = now.Add(time.Minute)
	assert.Empty(
		t,
		c.Collect(context.Background()),
		"an unknown new classification must not emit the incumbent's metrics",
	)
	assert.True(t, c.rebuildPending)
	assert.Same(t, oldDD, c.ddSnmpColl)
	assert.Zero(t, clientCreations, "no profiles means no candidate connection is needed")
	assert.Zero(t, device.closes)
	assert.Empty(
		t,
		c.deviceStore.Entries(),
		"known incompatible connection state must not remain eligible for SNMP-family consumers",
	)
	assert.Empty(t, c.deviceStore.DevicesByHostname(c.Hostname))
	lifecycle := c.deviceStore.LifecycleCut().Entries
	require.Len(t, lifecycle, 1)
	assert.Equal(t, registrationID, lifecycle[0].RegistrationID)
	assert.False(t, lifecycle[0].TopologyReady)
	assert.Empty(
		t,
		interfaceRows(),
		"a confirmed classification change must withdraw prior-device Function rows immediately",
	)

	device.probePDUs = []gosnmp.SnmpPDU{
		{Name: snmputils.OidSysName, Type: gosnmp.OctetString, Value: []byte("incomplete")},
	}
	*now = now.Add(time.Minute)
	assert.Empty(t, c.Collect(context.Background()), "a later unanswered object ID cannot resume incompatible profiles")
	assert.True(t, c.rebuildPending)
	assert.Zero(t, clientCreations)
	assert.Empty(t, interfaceRows(), "an incomplete retry cannot restore prior-device Function rows")
	device.probePDUs = nil
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	*now = now.Add(time.Minute)
	assert.Empty(t, c.Collect(context.Background()), "failed candidate connection must keep old profiles suspended")
	assert.True(t, c.rebuildPending)
	assert.Same(t, oldDD, c.ddSnmpColl)
	assert.Equal(t, 1, clientCreations)
	assert.Zero(t, *failedCloses, "a handler whose Connect failed does not guarantee a closable connection")
	assert.Zero(t, device.closes, "failed replacement must not close the incumbent used for retry probes")
	assert.Zero(t, ddCreations)
	assert.Equal(t, guid, c.VirtualNode().GUID)
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Empty(t, interfaceRows(), "failed connection setup must keep prior-device Function rows withdrawn")

	*now = now.Add(time.Second)
	assert.Empty(
		t,
		c.Collect(context.Background()),
		"pending replacement must also suppress old samples between checks",
	)
	assert.Equal(t, 1, clientCreations)
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.False(t, c.rebuildPending)
	assert.Equal(t, replacementTestObjectID, c.sysInfo.SysObjectID)
	assert.Equal(t, 2, clientCreations)
	assert.Equal(t, 1, ddCreations)
	assert.Equal(t, 1, device.closes)
	assert.Equal(t, guid, c.VirtualNode().GUID)
	entries = c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, registrationID, entries[0].RegistrationID)
	assert.Equal(t, replacementTestObjectID, entries[0].Info.SysObjectID)
	assert.Len(t, interfaceRows(), 1, "compatible collection repopulates the existing Function owner")
	c.Cleanup(context.Background())
	assert.Zero(t, *failedCloses)
	assert.Equal(t, 1, *recoveredCloses)
}

func newReplacementTestClient(t *testing.T, device *refreshTestDevice, connectErr error) (*snmpmock.MockHandler, *int) {
	t.Helper()
	client := snmpmock.NewMockHandler(gomock.NewController(t))
	// Register the concrete protocol before the shared helper's permissive getter.
	client.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	setMockClientSetterExpect(client)
	client.EXPECT().Connect().Return(connectErr).Times(1)
	closes := new(int)
	closeCalls := 0
	if connectErr == nil {
		closeCalls = 1
	}
	client.EXPECT().Close().DoAndReturn(func() error { *closes++; return nil }).Times(closeCalls)
	if connectErr == nil {
		client.EXPECT().MaxOids().Return(20).AnyTimes()
		client.EXPECT().Get(gomock.Any()).DoAndReturn(device.Get).AnyTimes()
		client.EXPECT().BulkWalkAll(snmputils.RootOidMibSystem).DoAndReturn(device.BulkWalkAll).Times(2)
	}
	return client, closes
}

func newReplacementTestDD(t *testing.T, cfg ddsnmpcollector.Config) *mockDdSnmpCollector {
	t.Helper()
	require.Equal(t, replacementTestObjectID, cfg.SysObjectID)
	require.NotEmpty(t, cfg.Profiles, "replacement must resolve actual metric profiles")
	var sources []string
	for _, profile := range cfg.Profiles {
		sources = append(sources, profile.SourceFile)
		assert.NotEqual(t, "refresh.yaml", profile.SourceFile, "replacement cannot reuse the incumbent profile")
	}
	require.Contains(t, strings.Join(sources, ","), "cisco")
	return &mockDdSnmpCollector{
		pms: []*ddsnmp.ProfileMetrics{{
			Source: cfg.Profiles[0].SourceFile,
			Metrics: []ddsnmp.Metric{
				{Name: "replacementLoad", Value: 7},
				{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{"interface": "1"},
					MultiValue: map[string]int64{"in": 10, "out": 20},
				},
			},
		}},
	}
}

func TestCollectorRuntimeReplacementDisablesUnselectedBGP(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	profile := normalTestProfile()
	require.NoError(t, ddsnmp.CompileTransforms(profile))
	c.snmpProfiles = append(c.snmpProfiles, profile)
	for oid, value := range map[string]int{
		normalTestRoot + ".1.0": 7, normalTestRoot + ".2.0": 3, normalTestRoot + ".3.0": 1,
		normalTestRoot + ".4.0": 3600, normalTestRoot + ".5.0": 6,
	} {
		device.values[oid] = gosnmp.SnmpPDU{
			Name:  oid,
			Type:  gosnmp.Integer,
			Value: value,
		}
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	require.NotNil(t, c.bgp)
	require.True(t, c.normal.latest.document.BGP.Enabled)
	require.Equal(t, 200, c.funcRouter.Handle(context.Background(), bgpPeersMethodID, nil).Status)
	client, _ := newReplacementTestClient(t, device, nil)
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	c.newSnmpClient = func() gosnmp.Handler { return client }
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		require.False(t, profilesHaveBGP(cfg.Profiles), "the current catalog selection must not declare BGP")
		return &mockDdSnmpCollector{
			pms: []*ddsnmp.ProfileMetrics{
				{Source: cfg.Profiles[0].SourceFile, Metrics: []ddsnmp.Metric{{Name: "replacementLoad", Value: 7}}},
			},
		}
	}
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: "1.3.6.1.4.1.9.1.1",
	}
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Nil(t, c.bgp)
	assert.False(t, c.normal.latest.document.BGP.Enabled)
	params, err := c.funcRouter.MethodParams(context.Background(), bgpPeersMethodID)
	require.NoError(t, err)
	assert.NotEmpty(t, params, "BGP remains a registered method returning unavailable")
	assert.Equal(t, 503, c.funcRouter.Handle(context.Background(), bgpPeersMethodID, nil).Status)
}

func TestCollectorRuntimeReplacementWithBlankAcquiredName(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	require.NotEmpty(t, c.Collect(context.Background()))
	guid := c.VirtualNode().GUID
	client, _ := newReplacementTestClient(t, device, nil)
	creations := 0
	c.newSnmpClient = func() gosnmp.Handler { creations++; return client }
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector { return newReplacementTestDD(t, cfg) }
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	device.setText(snmputils.OidSysName, " \t\n ")
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	require.Equal(t, "snmp-device", c.VirtualNode().Hostname)
	require.Equal(t, guid, c.VirtualNode().GUID)
	require.False(t, c.rebuildPending)
	require.Len(t, c.deviceStore.Entries(), 1)
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	require.Equal(t, 1, creations, "blank names must not require repeated client rebuilding")
}
