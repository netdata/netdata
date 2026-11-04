// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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
	inputErr   error
	tableErr   error
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
	} else if d.inputErr != nil && (slices.Contains(oids, snmputils.OidSysName) || slices.Contains(oids, snmputils.OidSysLocation)) {
		return nil, d.inputErr
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

func TestCollectorRuntimeMetadataRetryKeepsPollingState(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	require.NotEmpty(t, c.Collect(context.Background()))
	dd := c.ddSnmpColl
	device.setText(snmputils.OidSysName, "after")
	device.inputErr = errors.New("synthetic metadata timeout")
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, "after", c.VirtualNode().Hostname, "usable system metadata can publish while profile inputs retry")
	assert.Equal(t, "before", c.ddSnmpColl.PreparedDeviceMetadata()["name"].Value)
	assert.NotNil(t, c.pendingSysInfo)
	assert.Same(t, dd, c.ddSnmpColl)
	device.inputErr = nil
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(
		t,
		"after",
		c.VirtualNode().Hostname,
		"unchanged subsequent sysinfo must retry pending derived metadata",
	)
	assert.Nil(t, c.pendingSysInfo)
	assert.Equal(t, "after", c.ddSnmpColl.PreparedDeviceMetadata()["name"].Value)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Zero(t, device.closes)
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

func newRefreshTestCollector(t *testing.T) (*Collector, *refreshTestDevice, *time.Time) {
	t.Helper()
	d := &refreshTestDevice{
		values: map[string]gosnmp.SnmpPDU{
			snmputils.OidSysObject: {
				Name:  snmputils.OidSysObject,
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.99999.1",
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
					Include: []string{"1.3.6.1.4.1.99999.1"},
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

func TestCollectorRuntimeMetadataRefresh(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	authored := c.Configuration()
	require.NotEmpty(t, c.Collect(context.Background()))
	guid := c.VirtualNode().GUID
	dd, client, profiles := c.ddSnmpColl, c.snmpClient, c.snmpProfiles
	iface, router := c.ifaceCache, c.funcRouter
	charts := c.Charts()
	walks := device.walks
	assert.Equal(t, authored, c.Configuration(), "acquired identity must not become an authored override")

	device.setText(snmputils.OidSysName, "after")
	device.setText(snmputils.OidSysContact, "after contact")
	device.setText(snmputils.OidSysDescr, "after description")
	device.setText(snmputils.OidSysLocation, "")
	*now = now.Add(59 * time.Second)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Equal(t, 1, device.probes)
	*now = now.Add(time.Second)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, 2, device.probes)
	assert.Equal(t, "after", c.VirtualNode().Hostname)
	assert.Equal(t, guid, c.VirtualNode().GUID)
	assert.Equal(t, "after", c.VirtualNode().Labels["name"])
	assert.Equal(t, "after contact", c.VirtualNode().Labels["contact"])
	assert.Equal(t, "after description", c.VirtualNode().Labels["description"])
	assert.Empty(t, c.VirtualNode().Labels["location"], "explicit empty metadata must clear the old value")
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Same(t, client, c.snmpClient)
	assert.Equal(t, profiles, c.snmpProfiles)
	assert.Same(t, iface, c.ifaceCache)
	assert.Same(t, router, c.funcRouter)
	assert.Same(t, charts, c.Charts())
	assert.Equal(t, walks, device.walks, "metadata changes do not rediscover tables")
	assert.Equal(t, authored, c.Configuration())
	for _, chart := range *c.Charts() {
		labels := make(map[string]string)
		for _, label := range chart.Labels {
			labels[label.Key] = label.Value
		}
		assert.Equal(t, "after", labels["sysName"], chart.ID)
		if strings.HasPrefix(chart.ID, "snmp_device_prof_ifTraffic") {
			assert.Equal(t, "after", labels["snmp_host"])
		}
	}
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "after", entries[0].Info.SysName)
	assert.Empty(t, entries[0].Info.SysLocation)

	*now = now.Add(24 * time.Hour)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Same(t, dd, c.ddSnmpColl, "elapsed time is never a full-rebuild trigger")
	assert.Equal(t, walks, device.walks)
}

func TestCollectorRuntimeProbeFailureRetainsCollection(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	require.NotEmpty(t, c.Collect(context.Background()))
	dd := c.ddSnmpColl
	device.probeErr = errors.New("synthetic probe timeout")
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()), "failed housekeeping must not stop usable measurement")
	assert.Equal(t, "before", c.VirtualNode().Hostname)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Equal(t, 2, device.probes)
	device.probeErr = nil
	device.setText(snmputils.OidSysName, "recovered")
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, "recovered", c.VirtualNode().Hostname)
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

func TestCollectorRuntimeMetadataPreservesAuthoredAndNamedVnodes(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "authored", true: "named"}[named], func(t *testing.T) {
			c, device, now := newRefreshTestCollector(t)
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
			*now = now.Add(time.Minute)
			require.NotEmpty(t, c.Collect(context.Background()))
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

func TestCollectorRuntimePartialMetadataRefresh(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	const tagOID = "1.3.6.1.4.1.99999.91.0"
	const metricOID = "1.3.6.1.4.1.99999.92.0"
	device.setText(tagOID, "before accessory")
	device.values[metricOID] = gosnmp.SnmpPDU{
		Name:  metricOID,
		Type:  gosnmp.Integer,
		Value: 7,
	}
	transport := &refreshSelectiveFailure{
		refreshTestDevice: device,
		failedOID:         tagOID,
	}
	c.snmpClient = transport
	c.snmpProfiles = append(c.snmpProfiles, &ddsnmp.Profile{
		SourceFile: "accessory.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag: "accessory",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID: tagOID,
					},
				},
			}},
			Metrics: []ddprofiledefinition.MetricsConfig{
				{Symbol: ddprofiledefinition.SymbolConfig{
					OID:  metricOID,
					Name: "accessoryMetric",
				}},
			},
		},
	})
	require.NotEmpty(t, c.Collect(context.Background()))
	dd, client := c.ddSnmpColl, c.snmpClient
	labels := func(id string) map[string]string {
		result := make(map[string]string)
		for _, label := range c.Charts().Get(id).Labels {
			result[label.Key] = label.Value
		}
		return result
	}
	device.setText(snmputils.OidSysName, "after")
	device.setText(tagOID, "after accessory")
	transport.fail = true
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, "after", c.VirtualNode().Hostname, "failed unrelated inputs cannot block healthy identity updates")
	assert.Equal(t, "after", labels("snmp_device_prof_ifTraffic_1")["snmp_host"])
	assert.Equal(t, "before accessory", labels(chartIDFromName("accessoryMetric"))["accessory"])
	require.NotNil(t, c.pendingSysInfo, "failed profile input refresh must remain retryable")
	entries := c.deviceStore.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "after", entries[0].Info.SysName)
	version := c.chartMetadataVersion
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(
		t,
		version,
		c.chartMetadataVersion,
		"identical usable snapshots do not redefine charts on a failed retry",
	)

	transport.fail = false
	*now = now.Add(time.Minute)
	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Nil(t, c.pendingSysInfo)
	assert.Equal(
		t,
		"after accessory",
		labels(chartIDFromName("accessoryMetric"))["accessory"],
		"recovered inputs refresh chart labels even with unchanged system metadata",
	)
	assert.Same(t, dd, c.ddSnmpColl)
	assert.Same(t, client, c.snmpClient)
}

func TestCollectorRuntimeUnchangedObservationReconcilesNegativeInput(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	// Check observed the name, but the subsequent profile input acquisition reports absence.
	device.values[snmputils.OidSysName] = gosnmp.SnmpPDU{
		Name: snmputils.OidSysName,
		Type: gosnmp.NoSuchInstance,
	}
	require.NotEmpty(t, c.Collect(context.Background()))
	require.Equal(t, "before", c.sysInfo.Name)
	require.Empty(t, c.ddSnmpColl.PreparedDeviceMetadata()["name"].Value)
	device.setText(snmputils.OidSysName, "before")
	*now = now.Add(time.Minute)

	require.NotEmpty(t, c.Collect(context.Background()))
	assert.Equal(t, 2, device.probes)
	assert.Equal(
		t,
		"before",
		c.ddSnmpColl.PreparedDeviceMetadata()["name"].Value,
		"a successful system observation contradicts prior unsupported evidence even when its value is unchanged",
	)
	chart := c.Charts().Get("snmp_device_prof_ifTraffic_1")
	require.NotNil(t, chart)
	assert.Contains(t, chart.Labels, collectorapi.Label{
		Key:   "snmp_host",
		Value: "before",
	})
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
