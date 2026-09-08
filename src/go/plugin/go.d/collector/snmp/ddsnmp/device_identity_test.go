// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/golang/mock/gomock"
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

func TestAcquireDeviceIdentity(t *testing.T) {
	tests := map[string]struct {
		sysInfo  snmputils.SysInfo
		metadata map[string]ddsnmp.MetaTag
		opts     ddsnmp.DeviceIdentityOptions
		want     *ddsnmp.DeviceIdentity
	}{
		"configured identity and label precedence": {
			sysInfo: snmputils.SysInfo{
				SysObjectID:  "1.3.6.1.4.1.9",
				Name:         "router",
				Descr:        "description",
				Contact:      "contact",
				Location:     "location",
				Vendor:       "system-vendor",
				Organization: "organization",
				Category:     "Router",
				Model:        "system-model",
			},
			metadata: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "profile-vendor", IsExactMatch: true},
				"model":  {Value: "generic-model"}, "serial_number": {Value: "serial"},
				"_node_stale_after_seconds": {Value: "90", IsExactMatch: true},
			},
			opts: ddsnmp.DeviceIdentityOptions{
				Address:    "192.0.2.1",
				GUID:       "configured-guid",
				Hostname:   "configured-host",
				BaseLabels: map[string]string{"_node_stale_after_seconds": "30", "site": "athens"},
				Labels:     map[string]string{"site": "london", "contact": "", "_vnode_type": "configured-type"},
			},
			want: &ddsnmp.DeviceIdentity{
				GUID:     "configured-guid",
				Hostname: "configured-host",
				Labels: map[string]string{
					"_vnode_type": "configured-type", "_net_default_iface_ip": "192.0.2.1", "address": "192.0.2.1",
					"sys_object_id": "1.3.6.1.4.1.9", "name": "router", "description": "description", "contact": "", "location": "location",
					"vendor": "profile-vendor", "model": "system-model", "type": "Router", "serial_number": "serial",
					"site": "london", "_node_stale_after_seconds": "90",
				},
			},
		},
		"system name and organization fallbacks without metadata source": {
			sysInfo: snmputils.SysInfo{
				Name:         "router",
				Organization: "organization",
			},
			opts: ddsnmp.DeviceIdentityOptions{
				Address: "Router.Example.",
			},
			want: &ddsnmp.DeviceIdentity{
				GUID:     "a2a0c16c-1b12-5ba4-8814-d81a16f862c6",
				Hostname: "router",
				Labels: map[string]string{
					"_vnode_type": "snmp", "_net_default_iface_ip": "Router.Example.", "address": "Router.Example.",
					"sys_object_id": "", "name": "router", "description": "", "contact": "", "location": "", "vendor": "organization",
				},
			},
		},
		"empty system fallback without metadata source": {
			opts: ddsnmp.DeviceIdentityOptions{
				Address: "Router.Example.",
			},
			want: &ddsnmp.DeviceIdentity{
				GUID:     "a2a0c16c-1b12-5ba4-8814-d81a16f862c6",
				Hostname: "snmp-device",
				Labels: map[string]string{
					"_vnode_type": "snmp", "_net_default_iface_ip": "Router.Example.", "address": "Router.Example.",
					"sys_object_id": "", "name": "", "description": "", "contact": "", "location": "",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			siBefore, metaBefore := tc.sysInfo, maps.Clone(tc.metadata)
			optsBefore := tc.opts
			optsBefore.BaseLabels = maps.Clone(tc.opts.BaseLabels)
			optsBefore.Labels = maps.Clone(tc.opts.Labels)
			calls, wantCalls := 0, 0
			var source ddsnmp.DeviceMetadataSource
			if tc.metadata != nil {
				wantCalls = 1
				source = metadataSourceFunc(func() (map[string]ddsnmp.MetaTag, error) {
					calls++
					return tc.metadata, nil
				})
			}

			identity, err := ddsnmp.AcquireDeviceIdentity(&tc.sysInfo, source, tc.opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, identity)
			assert.Equal(t, wantCalls, calls)

			for key := range identity.Labels {
				identity.Labels[key] = "changed"
			}
			assert.Equal(t, siBefore, tc.sysInfo)
			assert.Equal(t, metaBefore, tc.metadata)
			assert.Equal(t, optsBefore, tc.opts)
		})
	}
}

func TestAcquireDeviceIdentityFailure(t *testing.T) {
	metadataErr := errors.New("metadata acquisition failed")
	tests := map[string]struct {
		sysInfo   *snmputils.SysInfo
		wantErr   error
		wantCalls int
	}{
		"missing system identity skips metadata acquisition": {},
		"metadata failure returns no partial identity": {
			sysInfo: &snmputils.SysInfo{}, wantErr: metadataErr, wantCalls: 1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			calls := 0
			source := metadataSourceFunc(func() (map[string]ddsnmp.MetaTag, error) {
				calls++
				return map[string]ddsnmp.MetaTag{"vendor": {Value: "partial"}}, metadataErr
			})
			identity, err := ddsnmp.AcquireDeviceIdentity(tc.sysInfo, source, ddsnmp.DeviceIdentityOptions{})
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.Same(t, tc.wantErr, err)
			}
			assert.Nil(t, identity)
			assert.Equal(t, tc.wantCalls, calls)
		})
	}
}

func TestAcquireDeviceIdentityWithoutMetricsJob(t *testing.T) {
	const metadataOID = "1.3.6.1.4.1.9.999.0"
	client := snmpmock.NewMockHandler(gomock.NewController(t))
	client.EXPECT().MaxOids().Return(20).AnyTimes()
	client.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	gomock.InOrder(
		client.EXPECT().
			Get([]string{snmputils.OidSysDescr, snmputils.OidSysObject, snmputils.OidSysContact, snmputils.OidSysName, snmputils.OidSysLocation}).
			Return(
				&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Name: snmputils.OidSysObject, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.9"},
						{Name: snmputils.OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
					},
				}, nil),
		client.EXPECT().Get([]string{metadataOID}).Return(&gosnmp.SnmpPacket{
			Variables: []gosnmp.SnmpPDU{
				{Name: metadataOID, Type: gosnmp.OctetString, Value: []byte("serial-123")},
			},
		}, nil),
	)
	si, err := snmputils.GetSysInfo(client)
	require.NoError(t, err)
	source := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  client,
		Log:         logger.New(),
		SysObjectID: si.SysObjectID,
		Profiles: []*ddsnmp.Profile{{SourceFile: "identity.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {Fields: map[string]ddprofiledefinition.MetadataField{
					"serial_number": {Symbol: ddprofiledefinition.SymbolConfig{
						OID:  metadataOID,
						Name: "serial",
					}},
				}},
			},
		}}},
	})
	identity, err := ddsnmp.AcquireDeviceIdentity(si, source, ddsnmp.DeviceIdentityOptions{
		Address: "192.0.2.1",
	})
	require.NoError(t, err)
	want := &ddsnmp.DeviceIdentity{
		GUID:     "8ec4cad3-78e2-5ea1-ba26-cd6fdc51f121",
		Hostname: "router",
		Labels: map[string]string{
			"_vnode_type": "snmp", "_net_default_iface_ip": "192.0.2.1", "address": "192.0.2.1",
			"sys_object_id": "1.3.6.1.4.1.9", "name": "router", "description": "", "contact": "", "location": "",
			"vendor": "Cisco", "serial_number": "serial-123",
		},
	}
	assert.Equal(t, want, identity)
}
