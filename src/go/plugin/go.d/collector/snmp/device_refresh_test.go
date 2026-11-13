// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refreshTestTable = "1.3.6.1.4.1.99999.100"

// refreshTestDevice answers from source state independently of collector caches.
type refreshTestDevice struct {
	gosnmp.Handler
	values     map[string]gosnmp.SnmpPDU
	probes     int
	walks      int
	closes     int
	probeErr   error
	probePDUs  []gosnmp.SnmpPDU
	tableErr   error
	failOID    string
	omittedOID string
}

func (*refreshTestDevice) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (*refreshTestDevice) MaxOids() int                { return 20 }
func (d *refreshTestDevice) Close() error              { d.closes++; return nil }
func (d *refreshTestDevice) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if slices.Equal(oids, sysInfoOIDsForTest()) {
		d.probes++
		if d.probeErr != nil {
			return nil, d.probeErr
		}
		if d.probePDUs != nil {
			return &gosnmp.SnmpPacket{
				Variables: slices.Clone(d.probePDUs),
			}, nil
		}
	}
	if d.failOID != "" && slices.Contains(oids, d.failOID) {
		return nil, errors.New("synthetic request failure")
	}
	if d.tableErr != nil {
		for _, oid := range oids {
			if strings.HasPrefix(oid, refreshTestTable+".") {
				return nil, d.tableErr
			}
		}
	}
	packet := &gosnmp.SnmpPacket{}
	for _, oid := range oids {
		if oid == d.omittedOID {
			continue
		}
		pdu, ok := d.values[strings.TrimPrefix(oid, ".")]
		if !ok {
			pdu = gosnmp.SnmpPDU{
				Name: oid,
				Type: gosnmp.NoSuchInstance,
			}
		}
		packet.Variables = append(packet.Variables, pdu)
	}
	return packet, nil
}

func (d *refreshTestDevice) BulkWalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	d.walks++
	if root == refreshTestTable && d.tableErr != nil {
		return nil, d.tableErr
	}
	var pdus []gosnmp.SnmpPDU
	for oid, pdu := range d.values {
		if strings.HasPrefix(oid, root+".") && oid != d.omittedOID {
			pdus = append(pdus, pdu)
		}
	}
	slices.SortFunc(pdus, func(a, b gosnmp.SnmpPDU) int { return strings.Compare(a.Name, b.Name) })
	return pdus, nil
}

func (d *refreshTestDevice) setText(oid, text string) {
	d.values[oid] = gosnmp.SnmpPDU{
		Name:  oid,
		Type:  gosnmp.OctetString,
		Value: []byte(text),
	}
}

func refreshTestTableKey(name, index string) string {
	return tableMetricKey(ddsnmp.Metric{Name: name, Tags: map[string]string{"interface": index}})
}

func newRefreshTestCollector(t *testing.T) (*Collector, *refreshTestDevice, *time.Time) {
	t.Helper()
	return newRefreshTestCollectorWithObjectID(t, "1.3.6.1.4.1.99999.1")
}

func newRefreshTestCollectorWithObjectID(
	t *testing.T,
	sysObjectID string,
) (*Collector, *refreshTestDevice, *time.Time) {
	t.Helper()
	d := &refreshTestDevice{
		values: map[string]gosnmp.SnmpPDU{
			snmputils.OidSysObject: {
				Name:  snmputils.OidSysObject,
				Type:  gosnmp.ObjectIdentifier,
				Value: sysObjectID,
			},
			refreshTestTable + ".1.1": {Name: refreshTestTable + ".1.1", Type: gosnmp.Counter64, Value: uint64(42)},
		},
	}
	d.setText(snmputils.OidSysName, "before")
	d.setText(snmputils.OidSysDescr, "before description")
	d.setText(snmputils.OidSysContact, "before contact")
	d.setText(snmputils.OidSysLocation, "before location")
	c := New(ddsnmp.NewDeviceStore())
	c.Config = prepareV2Config()
	c.CreateVnode = true
	c.Ping.Enabled = false
	c.snmpClient = d
	c.snmpProfiles = []*ddsnmp.Profile{{
		SourceFile: "refresh.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Selector: ddprofiledefinition.SelectorSpec{
				{SysObjectID: ddprofiledefinition.SelectorIncludeExclude{
					Include: []string{sysObjectID},
				}},
			},
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
				{
					MetricTagConfig: ddprofiledefinition.MetricTagConfig{
						Tag: "snmp_host",
						Symbol: ddprofiledefinition.SymbolConfigCompat{
							OID: snmputils.OidSysName,
						},
					},
				},
			},
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {Fields: map[string]ddprofiledefinition.MetadataField{
					"name": {Symbol: ddprofiledefinition.SymbolConfig{
						OID: snmputils.OidSysName,
					}},
					"location": {Symbol: ddprofiledefinition.SymbolConfig{
						OID: snmputils.OidSysLocation,
					}},
				}},
			},
			Metrics: []ddprofiledefinition.MetricsConfig{{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  refreshTestTable,
					Name: "interfaces",
				},
				Symbols:    []ddprofiledefinition.SymbolConfig{{OID: refreshTestTable + ".1", Name: "ifTraffic"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{Tag: "interface", Index: 1}},
			}},
		},
	}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	require.NoError(t, c.Check(context.Background()))
	return c, d, &now
}

// systemChangeTestObjectID is resolved by the stock catalog, so a rebuild that
// keeps the classification still selects real profiles.
const systemChangeTestObjectID = "1.3.6.1.4.1.9.1.1"

// systemChangeTest drives rebuilds that keep the sysObjectID. Every rebuilt DD
// collector polls the initial test profiles through a fresh client.
type systemChangeTest struct {
	c          *Collector
	device     *refreshTestDevice
	now        *time.Time
	connectErr error
	clients    int
	closes     int
}

func newSystemChangeTest(t *testing.T) *systemChangeTest {
	t.Helper()
	c, device, now := newRefreshTestCollectorWithObjectID(t, systemChangeTestObjectID)
	st := &systemChangeTest{c: c, device: device, now: now}
	c.newSnmpClient = func() gosnmp.Handler {
		st.clients++
		client := snmpmock.NewMockHandler(gomock.NewController(t))
		// Register the concrete protocol before the shared helper's permissive getter.
		client.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
		setMockClientSetterExpect(client)
		client.EXPECT().Connect().Return(st.connectErr).AnyTimes()
		client.EXPECT().Close().DoAndReturn(func() error { st.closes++; return nil }).AnyTimes()
		client.EXPECT().MaxOids().Return(20).AnyTimes()
		client.EXPECT().Get(gomock.Any()).DoAndReturn(device.Get).AnyTimes()
		client.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(device.BulkWalkAll).AnyTimes()
		return client
	}
	var profiles []*ddsnmp.Profile
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		if profiles == nil {
			profiles = cfg.Profiles
		}
		cfg.Profiles = profiles
		return ddsnmpcollector.New(cfg)
	}
	return st
}

func (st *systemChangeTest) collectAfter(d time.Duration) map[string]int64 {
	*st.now = st.now.Add(d)
	return st.c.Collect(context.Background())
}

func chartLabelsForTest(chart *collectorapi.Chart) map[string]string {
	labels := make(map[string]string, len(chart.Labels))
	for _, label := range chart.Labels {
		labels[label.Key] = label.Value
	}
	return labels
}

func TestCollectorRuntimeSystemChangeRebuilds(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device := st.c, st.device
	authored := c.Configuration()
	require.NotEmpty(t, c.Collect(context.Background()))
	require.NotEmpty(t, c.Collect(context.Background()))
	require.NotNil(t, c.ifaceCache.interfaces["1"].rates.trafficIn)
	guid := c.VirtualNode().GUID
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	registrationID := entries[0].RegistrationID
	dd, router, iface := c.ddSnmpColl, c.funcRouter, c.ifaceCache
	assert.Equal(t, authored, c.Configuration(), "acquired identity must not become an authored override")

	device.setText(snmputils.OidSysName, "after")
	device.setText(snmputils.OidSysContact, "after contact")
	device.setText(snmputils.OidSysDescr, "after description")
	device.setText(snmputils.OidSysLocation, "")
	require.NotEmpty(t, st.collectAfter(59*time.Second))
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Equal(t, 1, device.probes)
	assert.Zero(t, st.clients)

	mx := st.collectAfter(time.Second)
	require.Contains(t, mx, metricIDFromKey(refreshTestTableKey("ifTraffic", "1")), "the rebuild cycle collects")
	assert.Equal(t, 2, device.probes)
	assert.Equal(t, 1, st.clients)
	assert.False(t, c.rebuildPending)
	assert.NotSame(t, dd, c.ddSnmpColl)
	assert.Equal(t, 1, device.closes, "retire the previous connection after the rebuild")
	assert.Zero(t, st.closes, "the installed connection must remain open")
	assert.Equal(t, "after", c.VirtualNode().Hostname)
	assert.Equal(t, guid, c.VirtualNode().GUID)
	assert.Equal(t, "after", c.VirtualNode().Labels["name"])
	assert.Equal(t, "after contact", c.VirtualNode().Labels["contact"])
	assert.Equal(t, "after description", c.VirtualNode().Labels["description"])
	assert.Empty(t, c.VirtualNode().Labels["location"], "explicit empty metadata must clear the old value")
	assert.Equal(t, systemChangeTestObjectID, c.sysInfo.SysObjectID)
	assert.Same(t, router, c.funcRouter)
	assert.Same(t, iface, c.ifaceCache)
	require.Contains(t, c.ifaceCache.interfaces, "1")
	assert.Nil(t, c.ifaceCache.interfaces["1"].rates.trafficIn, "rebuilt state starts new Function baselines")
	assert.Equal(t, authored, c.Configuration())
	require.NotEmpty(t, *c.Charts())
	for _, chart := range *c.Charts() {
		assert.False(t, chart.IsRemoved(), "%s is reported again, so it is redefined in place", chart.ID)
		labels := chartLabelsForTest(chart)
		assert.Equal(t, "after", labels["sysName"], chart.ID)
		if chart.ID == refreshTestTableKey("ifTraffic", "1") {
			assert.Equal(t, "after", labels["snmp_host"])
		}
	}
	entries = c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, registrationID, entries[0].RegistrationID)
	assert.Equal(t, "after", entries[0].Info.SysName)
	assert.Empty(t, entries[0].Info.SysLocation)
	assert.Equal(t, guid, entries[0].Info.VnodeGUID)

	require.NotEmpty(t, st.collectAfter(24*time.Hour))
	assert.Equal(t, 1, st.clients, "elapsed time is never a rebuild trigger")

	device.setText(snmputils.OidSysLocation, "second location")
	require.NotEmpty(t, st.collectAfter(time.Minute))
	assert.Equal(t, 2, st.clients)
	assert.Equal(t, 1, st.closes, "each rebuild retires the connection it replaces")
	assert.Equal(t, "second location", c.VirtualNode().Labels["location"])
	assert.Equal(t, "after", c.VirtualNode().Hostname)
	c.Cleanup(context.Background())
	assert.Equal(t, 2, st.closes)
}

func TestCollectorRuntimeSystemChangeRebuildFailureKeepsCollecting(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device := st.c, st.device
	require.NotEmpty(t, c.Collect(context.Background()))
	dd := c.ddSnmpColl
	interfaceRows := func() any {
		params := funcapi.ResolvedParams{ifacesParamTypeGroup: {IDs: []string{"other"}}}
		return c.funcRouter.Handle(context.Background(), ifacesMethodID, params).Data
	}
	require.Len(t, interfaceRows(), 1)

	device.setText(snmputils.OidSysName, "after")
	st.connectErr = errors.New("synthetic connect failure")
	require.NotEmpty(t, st.collectAfter(time.Minute), "the incumbent is still compatible and keeps collecting")
	assert.Equal(t, 1, st.clients)
	assert.False(t, c.rebuildPending)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Zero(t, device.closes)
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Len(t, interfaceRows(), 1, "Function contents stay with the incumbent state")
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1, "the connection stays eligible for SNMP-family consumers")
	assert.Equal(t, "before", entries[0].Info.SysName)

	require.NotEmpty(t, st.collectAfter(time.Second))
	assert.Equal(t, 1, st.clients, "the rebuild retries on the next system check")

	// A classification change during the retries makes the incumbent incompatible.
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	assert.Empty(t, st.collectAfter(time.Minute))
	assert.True(t, c.rebuildPending)
	assert.Equal(t, 2, st.clients)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Empty(t, c.deviceStore.Entries())
	assert.Empty(t, interfaceRows())

	st.connectErr = nil
	require.NotEmpty(t, st.collectAfter(time.Minute))
	assert.Equal(t, 3, st.clients)
	assert.False(t, c.rebuildPending)
	assert.NotSame(t, dd, c.ddSnmpColl)
	assert.Equal(t, "after", c.VirtualNode().Hostname)
	entries = c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "after", entries[0].Info.SysName)
	assert.Equal(t, replacementTestObjectID, entries[0].Info.SysObjectID)
	c.Cleanup(context.Background())
}

func TestCollectorRuntimeSystemChangeWithoutProfilesKeepsCollecting(t *testing.T) {
	// The stock catalog has no profile for this sysObjectID.
	c, device, now := newRefreshTestCollectorWithObjectID(t, "1.3.6.1.2.1.999")
	c.newSnmpClient = func() gosnmp.Handler {
		t.Fatal("no candidate connection is needed without profiles")
		return nil
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	dd := c.ddSnmpColl
	device.setText(snmputils.OidSysName, "after")
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.False(t, c.rebuildPending)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Zero(t, device.closes)
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Equal(t, "before", c.sysInfo.Name, "the change stays pending for the next check")
}

func TestCollectorRuntimeSystemChangeFailedFirstPollKeepsPublication(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device := st.c, st.device
	const modelOID = "1.3.6.1.4.1.99999.95.0"
	device.setText(modelOID, "observed-model")
	c.snmpProfiles[0].Definition.Metadata["device"].Fields["model"] = ddprofiledefinition.MetadataField{
		Symbol: ddprofiledefinition.SymbolConfig{OID: modelOID},
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	require.Equal(t, "observed-model", c.VirtualNode().Labels["model"])
	charts := len(*c.Charts())
	require.NotZero(t, charts)

	// The rebuilt state cannot prepare its profile inputs on its first poll.
	device.setText(snmputils.OidSysName, "after")
	device.failOID = modelOID
	require.Empty(t, st.collectAfter(time.Minute))
	require.Equal(t, 1, st.clients)
	assert.Equal(t, "before", c.VirtualNode().Hostname, "the previous publication stays until inputs are prepared")
	assert.Equal(t, "observed-model", c.VirtualNode().Labels["model"])
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "before", entries[0].Info.SysName)
	assert.Equal(t, "observed-model", entries[0].Info.Model)
	require.Len(t, *c.Charts(), charts)
	for _, chart := range *c.Charts() {
		assert.False(t, chart.IsRemoved(), "%s: a failed poll leaves gaps, not retired charts", chart.ID)
	}

	device.failOID = ""
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, 1, st.clients)
	assert.Equal(t, "after", c.VirtualNode().Hostname)
	assert.Equal(t, "observed-model", c.VirtualNode().Labels["model"])
	entries = c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "after", entries[0].Info.SysName)
	assert.Equal(t, "observed-model", entries[0].Info.Model)
	require.Len(t, *c.Charts(), charts)
	for _, chart := range *c.Charts() {
		assert.False(t, chart.IsRemoved(), chart.ID)
		assert.Equal(t, "after", chartLabelsForTest(chart)["sysName"], chart.ID)
	}
	c.Cleanup(context.Background())
}

func TestCollectorRuntimeUnpreparedDeviceIsNotPublished(t *testing.T) {
	const modelOID = "1.3.6.1.4.1.99999.95.0"
	withModel := func(st *systemChangeTest) {
		st.device.setText(modelOID, "observed-model")
		st.c.snmpProfiles[0].Definition.Metadata["device"].Fields["model"] = ddprofiledefinition.MetadataField{
			Symbol: ddprofiledefinition.SymbolConfig{OID: modelOID},
		}
	}

	t.Run("new job", func(t *testing.T) {
		st := newSystemChangeTest(t)
		c, device := st.c, st.device
		withModel(st)
		device.failOID = modelOID
		require.Empty(t, c.Collect(context.Background()))
		assert.Nil(t, c.VirtualNode())
		assert.Empty(t, c.deviceStore.Entries())

		device.failOID = ""
		require.NotEmpty(t, c.Collect(context.Background()))
		assert.Equal(t, "observed-model", c.VirtualNode().Labels["model"])
		entries := c.deviceStore.Entries()
		require.Len(t, entries, 1)
		assert.Equal(t, "observed-model", entries[0].Info.Model)
	})

	t.Run("replacement", func(t *testing.T) {
		st := newSystemChangeTest(t)
		c, device := st.c, st.device
		withModel(st)
		require.NotEmpty(t, c.Collect(context.Background()))
		device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
			Name:  snmputils.OidSysObject,
			Type:  gosnmp.ObjectIdentifier,
			Value: replacementTestObjectID,
		}
		device.setText(snmputils.OidSysName, "replacement")
		device.failOID = modelOID
		require.Empty(t, st.collectAfter(time.Minute))
		require.Equal(t, 1, st.clients)
		assert.False(t, c.rebuildPending)
		assert.Empty(t, c.deviceStore.Entries(), "the withdrawn device returns with its profile metadata, not before")
		assert.Equal(t, "before", c.VirtualNode().Hostname)

		device.failOID = ""
		require.NotEmpty(t, c.Collect(context.Background()))
		assert.Equal(t, "replacement", c.VirtualNode().Hostname)
		entries := c.deviceStore.Entries()
		require.Len(t, entries, 1)
		assert.Equal(t, replacementTestObjectID, entries[0].Info.SysObjectID)
		assert.Equal(t, "replacement", entries[0].Info.SysName)
		assert.Equal(t, "observed-model", c.ddSnmpColl.PreparedDeviceMetadata()["model"].Value)
		c.Cleanup(context.Background())
	})
}

func TestCollectorRuntimeSystemChangeRelabelsPingCharts(t *testing.T) {
	st := newSystemChangeTest(t)
	c := st.c
	c.Ping.Enabled = true
	c.pingClient = &mockPingClient{sample: pingSuccessSample("192.0.2.1")}
	require.NotEmpty(t, c.Collect(context.Background()))
	require.NotNil(t, c.Charts().Get(pingRTTChart.ID))

	st.device.setText(snmputils.OidSysName, "after")
	require.NotEmpty(t, st.collectAfter(time.Minute))
	require.Equal(t, 1, st.clients)
	for _, id := range []string{pingRTTChart.ID, pingRTTStdDevChart.ID} {
		chart := c.Charts().Get(id)
		require.NotNil(t, chart, id)
		assert.False(t, chart.IsRemoved(), id)
		assert.Equal(t, "after", chartLabelsForTest(chart)["sysName"], id)
	}
	c.Cleanup(context.Background())
}

func TestCollectorRuntimeUnansweredFieldsAreNotChanges(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device := st.c, st.device
	require.NotEmpty(t, c.Collect(context.Background()))
	device.probePDUs = []gosnmp.SnmpPDU{
		device.values[snmputils.OidSysObject],
		{Name: snmputils.OidSysName, Type: gosnmp.NoSuchInstance},
	}
	require.NotEmpty(t, st.collectAfter(time.Minute))
	assert.Equal(t, 2, device.probes)
	assert.Zero(t, st.clients)
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Equal(t, "before location", c.VirtualNode().Labels["location"])

	device.probePDUs = nil
	device.setText(snmputils.OidSysLocation, "after location")
	require.NotEmpty(t, st.collectAfter(time.Minute))
	assert.Equal(t, 1, st.clients)
	assert.Equal(t, "after location", c.VirtualNode().Labels["location"])
	c.Cleanup(context.Background())
}

func TestCollectorRuntimeProbeFailureRetainsCollection(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device := st.c, st.device
	require.NotEmpty(t, c.Collect(context.Background()))
	dd := c.ddSnmpColl
	device.probeErr = errors.New("synthetic probe timeout")
	require.NotEmpty(t, st.collectAfter(time.Minute), "failed housekeeping must not stop usable measurement")
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Equal(t, 2, device.probes)
	device.probeErr = nil
	device.setText(snmputils.OidSysName, "recovered")
	require.NotEmpty(t, st.collectAfter(time.Minute))
	assert.Equal(t, "recovered", c.VirtualNode().Hostname)
	c.Cleanup(context.Background())
}

func TestCollectorRuntimeUnusableObjectIDDoesNotRebuild(t *testing.T) {
	for name, pdu := range map[string]gosnmp.SnmpPDU{
		"missing": {Name: snmputils.OidSysObject, Type: gosnmp.NoSuchObject},
		"invalid": {Name: snmputils.OidSysObject, Type: gosnmp.OctetString, Value: []byte("not-an-oid")},
	} {
		t.Run(name, func(t *testing.T) {
			c, device, now := newRefreshTestCollector(t)
			require.NotEmpty(t, c.Collect(context.Background()))
			dd := c.ddSnmpColl
			device.values[snmputils.OidSysObject] = pdu
			*now = now.Add(time.Minute)
			require.NotEmpty(t, c.Collect(context.Background()))
			assert.Same(t, dd, c.ddSnmpColl)
			assert.Equal(t, "1.3.6.1.4.1.99999.1", c.sysInfo.SysObjectID)
		})
	}
}

func TestCollectorRuntimeSystemChangePreservesAuthoredAndNamedVnodes(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "authored", true: "named"}[named], func(t *testing.T) {
			st := newSystemChangeTest(t)
			c, device := st.c, st.device
			c.LocalVnode.Hostname = "authored"
			c.LocalVnode.Labels = map[string]string{"contact": "", "site": "configured"}
			c.snmpProfiles[0].Definition.Metadata["device"].Fields["vendor"] = ddprofiledefinition.MetadataField{
				Value: "profile-vendor",
			}
			if named {
				c.Vnode = "configured-vnode"
				c.SetConfiguredVnode(
					vnodes.VirtualNode{
						GUID:     "11111111-1111-1111-1111-111111111111",
						Hostname: "centrally-managed",
					},
				)
			}
			authored := c.Configuration()
			require.NotEmpty(t, c.Collect(context.Background()))
			device.setText(snmputils.OidSysName, "changed")
			device.setText(snmputils.OidSysContact, "changed contact")
			require.NotEmpty(t, st.collectAfter(time.Minute))
			require.Equal(t, 1, st.clients)
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			assert.Equal(t, authored, c.Configuration())
			if named {
				assert.Nil(t, c.VirtualNode())
				assert.Equal(t, "centrally-managed", c.deviceStore.Entries()[0].Info.VnodeHostname)
				c.SetConfiguredVnode(
					vnodes.VirtualNode{
						GUID:     "11111111-1111-1111-1111-111111111111",
						Hostname: "updated-centrally",
					},
				)
				assert.Equal(
					t,
					"profile-vendor",
					c.deviceStore.Entries()[0].Info.Vendor,
					"configured metadata updates retain acquired profile identity",
				)
			} else {
				assert.Equal(t, "authored", c.VirtualNode().Hostname)
				assert.Equal(t, "changed", c.VirtualNode().Labels["name"])
				assert.Empty(t, c.VirtualNode().Labels["contact"])
				assert.Equal(t, "configured", c.VirtualNode().Labels["site"])
			}
		})
	}
}

func TestCollectorRuntimePingOnlyKeepsSetupOnlySNMP(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	c.PingOnly, c.CreateVnode = true, false
	c.pingClient = &mockPingClient{
		sample: pingSuccessSample("192.0.2.1"),
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	device.setText(snmputils.OidSysName, "changed")
	*now = now.Add(24 * time.Hour)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, 1, device.probes)
	assert.Zero(t, device.walks)
}

// Only this profile's scalar input fails; normal system and metric polling remain usable.
type refreshSelectiveFailure struct {
	*refreshTestDevice
	failedOID string
	fail      bool
}

func (d *refreshSelectiveFailure) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if d.fail && slices.Contains(oids, d.failedOID) {
		return nil, errors.New("synthetic profile input timeout")
	}
	return d.refreshTestDevice.Get(oids)
}

func TestCollectorRuntimeInitialProfileMetadataRecovery(t *testing.T) {
	for _, field := range []string{"vendor", "model"} {
		t.Run(field, func(t *testing.T) {
			c, device, now := newRefreshTestCollector(t)
			const inputOID = "1.3.6.1.4.1.99999.80.0"
			const metricOID = "1.3.6.1.4.1.99999.81.0"
			device.setText(inputOID, "recovered-"+field)
			device.values[metricOID] = gosnmp.SnmpPDU{
				Name:  metricOID,
				Type:  gosnmp.Integer,
				Value: 7,
			}
			transport := &refreshSelectiveFailure{
				refreshTestDevice: device,
				failedOID:         inputOID,
				fail:              true,
			}
			c.snmpClient = transport
			c.snmpProfiles = append(c.snmpProfiles, &ddsnmp.Profile{
				SourceFile: "recovering.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Selector: ddprofiledefinition.SelectorSpec{
						{
							SysObjectID: ddprofiledefinition.SelectorIncludeExclude{
								Include: []string{"1.3.6.1.4.1.99999.1"},
							},
						},
					},
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								field: {Symbol: ddprofiledefinition.SymbolConfig{
									OID: inputOID,
								}},
							},
						},
					},
					Metrics: []ddprofiledefinition.MetricsConfig{
						{Symbol: ddprofiledefinition.SymbolConfig{
							OID:  metricOID,
							Name: "recoveredMetric",
						}},
					},
				},
			})
			require.NotEmpty(
				t,
				c.Collect(context.Background()),
				"healthy profiles remain available during preparation failure",
			)
			dd, client := c.ddSnmpColl, c.snmpClient
			require.Empty(t, dd.PreparedDeviceMetadata()[field].Value)
			transport.fail = false
			require.Equal(t, int64(7), c.Collect(context.Background())[metricIDFromName("recoveredMetric")])
			assert.Equal(
				t,
				"recovered-"+field,
				c.VirtualNode().Labels[field],
				"normal lazy preparation must publish newly usable metadata",
			)
			entries := c.deviceStore.Entries()
			require.Len(t, entries, 1)
			if field == "vendor" {
				assert.Equal(t, "recovered-vendor", entries[0].Info.Vendor)
			} else {
				assert.Equal(t, "recovered-model", entries[0].Info.Model)
			}
			*now = now.Add(time.Minute)
			require.NotEmpty(t, c.Collect(context.Background()))
			assert.Equal(t, "recovered-"+field, c.VirtualNode().Labels[field])
			assert.Same(t, dd, c.ddSnmpColl)
			assert.Same(t, client, c.snmpClient)
		})
	}
}

func TestCollectorPublishesPreparedMetadataWhenMetricsFail(t *testing.T) {
	c, device, _ := newRefreshTestCollector(t)
	const modelOID = "1.3.6.1.4.1.99999.95.0"
	device.setText(modelOID, "observed-model")
	c.snmpProfiles[0].Definition.Metadata["device"].Fields["model"] = ddprofiledefinition.MetadataField{
		Symbol: ddprofiledefinition.SymbolConfig{OID: modelOID},
	}
	device.tableErr = errors.New("synthetic table failure")
	require.Empty(t, c.Collect(context.Background()))
	require.Equal(t, "observed-model", c.ddSnmpColl.PreparedDeviceMetadata()["model"].Value)
	assert.Equal(t, "observed-model", c.VirtualNode().Labels["model"], "usable metadata is independent of metric-frame success")
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "observed-model", entries[0].Info.Model)
}
