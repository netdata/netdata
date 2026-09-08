// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type metadataSourceFunc func() (map[string]ddsnmp.MetaTag, error)

func (f metadataSourceFunc) CollectDeviceMetadata() (map[string]ddsnmp.MetaTag, error) {
	return f()
}

func TestAcquireDeviceIdentityPrecedenceAndOwnership(t *testing.T) {
	si := snmputils.SysInfo{
		SysObjectID: "1.3.6.1.4.1.9", Name: "router", Descr: "description", Contact: "contact", Location: "location",
		Vendor: "system-vendor", Organization: "organization", Category: "Router", Model: "system-model",
	}
	metadata := map[string]ddsnmp.MetaTag{
		"vendor": {Value: "profile-vendor", IsExactMatch: true},
		"model":  {Value: "generic-model"}, "serial_number": {Value: "serial"},
		"_node_stale_after_seconds": {Value: "90", IsExactMatch: true},
	}
	opts := ddsnmp.DeviceIdentityOptions{
		Address: "192.0.2.1", GUID: "configured-guid", Hostname: "configured-host",
		BaseLabels: map[string]string{"_node_stale_after_seconds": "30", "site": "athens"},
		Labels:     map[string]string{"site": "london", "contact": "", "_vnode_type": "configured-type"},
	}
	siBefore, metaBefore := si, maps.Clone(metadata)
	baseBefore, labelsBefore := maps.Clone(opts.BaseLabels), maps.Clone(opts.Labels)
	calls := 0
	identity, err := ddsnmp.AcquireDeviceIdentity(&si, metadataSourceFunc(func() (map[string]ddsnmp.MetaTag, error) {
		calls++
		return metadata, nil
	}), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, "configured-guid", identity.GUID)
	assert.Equal(t, "configured-host", identity.Hostname)
	assert.Equal(t, map[string]string{
		"_vnode_type": "configured-type", "_net_default_iface_ip": "192.0.2.1", "address": "192.0.2.1",
		"sys_object_id": si.SysObjectID, "name": "router", "description": "description", "contact": "", "location": "location",
		"vendor": "profile-vendor", "model": "system-model", "type": "Router", "serial_number": "serial",
		"site": "london", "_node_stale_after_seconds": "90",
	}, identity.Labels)
	identity.Labels["site"] = "changed"
	identity.Labels["serial_number"] = "changed"
	identity.Labels["_node_stale_after_seconds"] = "changed"
	assert.Equal(t, siBefore, si)
	assert.Equal(t, metaBefore, metadata)
	assert.Equal(t, baseBefore, opts.BaseLabels)
	assert.Equal(t, labelsBefore, opts.Labels)
}

func TestAcquireDeviceIdentityFallbacks(t *testing.T) {
	for _, tc := range []struct{ name, systemName, organization, hostname string }{
		{"system name", "router", "organization", "router"},
		{"empty system", "", "", "snmp-device"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, err := ddsnmp.AcquireDeviceIdentity(&snmputils.SysInfo{Name: tc.systemName, Organization: tc.organization}, nil,
				ddsnmp.DeviceIdentityOptions{Address: "Router.Example."})
			require.NoError(t, err)
			assert.Equal(t, uuid.NewSHA1(uuid.NameSpaceDNS, []byte("Router.Example.")).String(), identity.GUID)
			assert.Equal(t, tc.hostname, identity.Hostname)
			assert.Equal(t, "Router.Example.", identity.Labels["address"])
			assert.Equal(t, "snmp", identity.Labels["_vnode_type"])
			assert.Equal(t, tc.organization, identity.Labels["vendor"])
			assert.NotContains(t, identity.Labels, "_node_stale_after_seconds")
			assert.NotContains(t, identity.Labels, "model")
			assert.NotContains(t, identity.Labels, "type")
		})
	}
}

func TestAcquireDeviceIdentityFailure(t *testing.T) {
	wantErr := errors.New("metadata acquisition failed")
	calls := 0
	source := metadataSourceFunc(func() (map[string]ddsnmp.MetaTag, error) {
		calls++
		return map[string]ddsnmp.MetaTag{"vendor": {Value: "partial"}}, wantErr
	})
	identity, err := ddsnmp.AcquireDeviceIdentity(nil, source, ddsnmp.DeviceIdentityOptions{})
	require.Error(t, err)
	assert.Nil(t, identity)
	assert.Zero(t, calls)
	identity, err = ddsnmp.AcquireDeviceIdentity(&snmputils.SysInfo{}, source, ddsnmp.DeviceIdentityOptions{})
	assert.Same(t, wantErr, err)
	assert.Nil(t, identity)
	assert.Equal(t, 1, calls)
}

func TestAcquireDeviceIdentityWithoutMetricsJob(t *testing.T) {
	const metadataOID = "1.3.6.1.4.1.9.999.0"
	client := snmpmock.NewMockHandler(gomock.NewController(t))
	client.EXPECT().MaxOids().Return(20).AnyTimes()
	client.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	gomock.InOrder(
		client.EXPECT().Get([]string{snmputils.OidSysDescr, snmputils.OidSysObject, snmputils.OidSysContact, snmputils.OidSysName, snmputils.OidSysLocation}).Return(
			&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
				{Name: snmputils.OidSysObject, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.9"},
				{Name: snmputils.OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
			}}, nil),
		client.EXPECT().Get([]string{metadataOID}).Return(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
			{Name: metadataOID, Type: gosnmp.OctetString, Value: []byte("serial-123")},
		}}, nil),
	)
	si, err := snmputils.GetSysInfo(client)
	require.NoError(t, err)
	source := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient: client, Log: logger.New(), SysObjectID: si.SysObjectID,
		Profiles: []*ddsnmp.Profile{{SourceFile: "identity.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{"device": {Fields: map[string]ddprofiledefinition.MetadataField{
				"serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: metadataOID, Name: "serial"}},
			}}},
		}}},
	})
	identity, err := ddsnmp.AcquireDeviceIdentity(si, source, ddsnmp.DeviceIdentityOptions{Address: "192.0.2.1"})
	require.NoError(t, err)
	assert.Equal(t, "router", identity.Hostname)
	assert.Equal(t, "serial-123", identity.Labels["serial_number"])
	assert.Equal(t, si.SysObjectID, identity.Labels["sys_object_id"])
}
