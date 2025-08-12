// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP func() *Collector
		wantFail    bool
	}{
		"fail with default config": {
			wantFail: true,
			prepareSNMP: func() *Collector {
				return New()
			},
		},
		"fail when using SNMPv3 but 'user.name' not set": {
			wantFail: true,
			prepareSNMP: func() *Collector {
				collr := New()
				collr.Config = prepareV3Config()
				collr.User.Name = ""
				return collr
			},
		},
		"success when using SNMPv1 with valid config": {
			wantFail: false,
			prepareSNMP: func() *Collector {
				collr := New()
				collr.Config = prepareV1Config()
				return collr
			},
		},
		"success when using SNMPv2 with valid config": {
			wantFail: false,
			prepareSNMP: func() *Collector {
				collr := New()
				collr.Config = prepareV2Config()
				return collr
			},
		},
		"success when using SNMPv3 with valid config": {
			wantFail: false,
			prepareSNMP: func() *Collector {
				collr := New()
				collr.Config = prepareV3Config()
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepareSNMP()

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP func(t *testing.T, m *snmpmock.MockHandler) *Collector
	}{
		"cleanup call if snmpClient initialized": {
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareV2Config()
				collr.newSnmpClient = func() gosnmp.Handler { return m }
				setMockClientInitExpect(m)

				require.NoError(t, collr.Init(context.Background()))

				m.EXPECT().Close().Times(1)

				return collr
			},
		},
		"cleanup call does not panic if snmpClient not initialized": {
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareV2Config()
				collr.newSnmpClient = func() gosnmp.Handler { return m }
				setMockClientInitExpect(m)

				require.NoError(t, collr.Init(context.Background()))

				collr.snmpClient = nil

				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			collr := test.prepareSNMP(t, mockSNMP)

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP   func(t *testing.T, m *snmpmock.MockHandler) *Collector
		wantNumCharts int
		doCollect     bool
	}{
		"if-mib, no custom": {
			doCollect:     true,
			wantNumCharts: len(netIfaceChartsTmpl)*4 + 1,
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareV2Config()
				if collr.EnableProfiles {
					setMockClientSysObjectidExpect(m)
				}
				setMockClientSysinfoAndUptimeExpect(m)
				setMockClientIfMibExpect(m)

				return collr
			},
		},
		"custom, no if-mib": {
			wantNumCharts: 10,
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 9)
				collr.collectIfMib = false

				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			setMockClientInitExpect(mockSNMP)

			collr := test.prepareSNMP(t, mockSNMP)
			collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

			require.NoError(t, collr.Init(context.Background()))

			if test.doCollect {
				_ = collr.Check(context.Background())
				_ = collr.Collect(context.Background())
			}

			assert.Equal(t, test.wantNumCharts, len(*collr.Charts()))
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail    bool
		prepareSNMP func(m *snmpmock.MockHandler) *Collector
	}{
		"success when sysinfo collected": {
			wantFail: false,
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareV2Config()

				setMockClientSysInfoExpect(m)

				return collr
			},
		},
		"fail when snmp client Get fails": {
			wantFail: true,
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 3)
				collr.collectIfMib = false
				m.EXPECT().WalkAll(snmpsd.RootOidMibSystem).Return(nil, errors.New("mock Get() error")).Times(1)

				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			setMockClientInitExpect(mockSNMP)

			collr := test.prepareSNMP(mockSNMP)
			collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP   func(m *snmpmock.MockHandler) *Collector
		wantCollected map[string]int64
	}{
		"success only IF-MIB": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareV2Config()

				if collr.EnableProfiles {
					setMockClientSysObjectidExpect(m)
				}
				setMockClientIfMibExpect(m)

				return collr
			},
			wantCollected: map[string]int64{
				"net_iface_ether1_admin_status_down":                0,
				"net_iface_ether1_admin_status_testing":             0,
				"net_iface_ether1_admin_status_up":                  1,
				"net_iface_ether1_bcast_in":                         0,
				"net_iface_ether1_bcast_out":                        0,
				"net_iface_ether1_discards_in":                      0,
				"net_iface_ether1_discards_out":                     0,
				"net_iface_ether1_errors_in":                        0,
				"net_iface_ether1_errors_out":                       0,
				"net_iface_ether1_mcast_in":                         0,
				"net_iface_ether1_mcast_out":                        0,
				"net_iface_ether1_oper_status_dormant":              0,
				"net_iface_ether1_oper_status_down":                 1,
				"net_iface_ether1_oper_status_lowerLayerDown":       0,
				"net_iface_ether1_oper_status_notPresent":           0,
				"net_iface_ether1_oper_status_testing":              0,
				"net_iface_ether1_oper_status_unknown":              0,
				"net_iface_ether1_oper_status_up":                   0,
				"net_iface_ether1_traffic_in":                       0,
				"net_iface_ether1_traffic_out":                      0,
				"net_iface_ether1_ucast_in":                         0,
				"net_iface_ether1_ucast_out":                        0,
				"net_iface_ether2_admin_status_down":                0,
				"net_iface_ether2_admin_status_testing":             0,
				"net_iface_ether2_admin_status_up":                  1,
				"net_iface_ether2_bcast_in":                         0,
				"net_iface_ether2_bcast_out":                        7386,
				"net_iface_ether2_discards_in":                      0,
				"net_iface_ether2_discards_out":                     0,
				"net_iface_ether2_errors_in":                        0,
				"net_iface_ether2_errors_out":                       0,
				"net_iface_ether2_mcast_in":                         1891,
				"net_iface_ether2_mcast_out":                        28844,
				"net_iface_ether2_oper_status_dormant":              0,
				"net_iface_ether2_oper_status_down":                 0,
				"net_iface_ether2_oper_status_lowerLayerDown":       0,
				"net_iface_ether2_oper_status_notPresent":           0,
				"net_iface_ether2_oper_status_testing":              0,
				"net_iface_ether2_oper_status_unknown":              0,
				"net_iface_ether2_oper_status_up":                   1,
				"net_iface_ether2_traffic_in":                       615057509,
				"net_iface_ether2_traffic_out":                      159677206,
				"net_iface_ether2_ucast_in":                         71080332,
				"net_iface_ether2_ucast_out":                        39509661,
				"net_iface_sfp-sfpplus1_admin_status_down":          0,
				"net_iface_sfp-sfpplus1_admin_status_testing":       0,
				"net_iface_sfp-sfpplus1_admin_status_up":            1,
				"net_iface_sfp-sfpplus1_bcast_in":                   0,
				"net_iface_sfp-sfpplus1_bcast_out":                  0,
				"net_iface_sfp-sfpplus1_discards_in":                0,
				"net_iface_sfp-sfpplus1_discards_out":               0,
				"net_iface_sfp-sfpplus1_errors_in":                  0,
				"net_iface_sfp-sfpplus1_errors_out":                 0,
				"net_iface_sfp-sfpplus1_mcast_in":                   0,
				"net_iface_sfp-sfpplus1_mcast_out":                  0,
				"net_iface_sfp-sfpplus1_oper_status_dormant":        0,
				"net_iface_sfp-sfpplus1_oper_status_down":           0,
				"net_iface_sfp-sfpplus1_oper_status_lowerLayerDown": 0,
				"net_iface_sfp-sfpplus1_oper_status_notPresent":     1,
				"net_iface_sfp-sfpplus1_oper_status_testing":        0,
				"net_iface_sfp-sfpplus1_oper_status_unknown":        0,
				"net_iface_sfp-sfpplus1_oper_status_up":             0,
				"net_iface_sfp-sfpplus1_traffic_in":                 0,
				"net_iface_sfp-sfpplus1_traffic_out":                0,
				"net_iface_sfp-sfpplus1_ucast_in":                   0,
				"net_iface_sfp-sfpplus1_ucast_out":                  0,
				"net_iface_sfp-sfpplus2_admin_status_down":          0,
				"net_iface_sfp-sfpplus2_admin_status_testing":       0,
				"net_iface_sfp-sfpplus2_admin_status_up":            1,
				"net_iface_sfp-sfpplus2_bcast_in":                   0,
				"net_iface_sfp-sfpplus2_bcast_out":                  0,
				"net_iface_sfp-sfpplus2_discards_in":                0,
				"net_iface_sfp-sfpplus2_discards_out":               0,
				"net_iface_sfp-sfpplus2_errors_in":                  0,
				"net_iface_sfp-sfpplus2_errors_out":                 0,
				"net_iface_sfp-sfpplus2_mcast_in":                   0,
				"net_iface_sfp-sfpplus2_mcast_out":                  0,
				"net_iface_sfp-sfpplus2_oper_status_dormant":        0,
				"net_iface_sfp-sfpplus2_oper_status_down":           0,
				"net_iface_sfp-sfpplus2_oper_status_lowerLayerDown": 0,
				"net_iface_sfp-sfpplus2_oper_status_notPresent":     1,
				"net_iface_sfp-sfpplus2_oper_status_testing":        0,
				"net_iface_sfp-sfpplus2_oper_status_unknown":        0,
				"net_iface_sfp-sfpplus2_oper_status_up":             0,
				"net_iface_sfp-sfpplus2_traffic_in":                 0,
				"net_iface_sfp-sfpplus2_traffic_out":                0,
				"net_iface_sfp-sfpplus2_ucast_in":                   0,
				"net_iface_sfp-sfpplus2_ucast_out":                  0,
				"uptime":                                            60,
			},
		},
		"success only custom OIDs supported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 3)
				collr.collectIfMib = false

				if collr.EnableProfiles {
					setMockClientSysObjectidExpect(m)
				}

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: 10, Type: gosnmp.Counter32},
						{Value: 20, Type: gosnmp.Counter64},
						{Value: 30, Type: gosnmp.Gauge32},
						{Value: 1, Type: gosnmp.Boolean},
						{Value: 40, Type: gosnmp.Gauge32},
						{Value: 50, Type: gosnmp.TimeTicks},
						{Value: 60, Type: gosnmp.Uinteger32},
						{Value: 70, Type: gosnmp.Integer},
					},
				}, nil).Times(1)

				return collr
			},
			wantCollected: map[string]int64{
				//"TestMetric":             1,
				"1.3.6.1.2.1.2.2.1.10.0": 10,
				"1.3.6.1.2.1.2.2.1.16.0": 20,
				"1.3.6.1.2.1.2.2.1.10.1": 30,
				"1.3.6.1.2.1.2.2.1.16.1": 1,
				"1.3.6.1.2.1.2.2.1.10.2": 40,
				"1.3.6.1.2.1.2.2.1.16.2": 50,
				"1.3.6.1.2.1.2.2.1.10.3": 60,
				"1.3.6.1.2.1.2.2.1.16.3": 70,
				"uptime":                 60,
			},
		},
		"success only custom OIDs supported and unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 2)
				collr.collectIfMib = false

				if collr.EnableProfiles {
					setMockClientSysObjectidExpect(m)
				}

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: 10, Type: gosnmp.Counter32},
						{Value: 20, Type: gosnmp.Counter64},
						{Value: 30, Type: gosnmp.Gauge32},
						{Value: nil, Type: gosnmp.NoSuchInstance},
						{Value: nil, Type: gosnmp.NoSuchInstance},
						{Value: nil, Type: gosnmp.NoSuchInstance},
					},
				}, nil).Times(1)

				return collr
			},
			wantCollected: map[string]int64{
				//"TestMetric":             1,
				"1.3.6.1.2.1.2.2.1.10.0": 10,
				"1.3.6.1.2.1.2.2.1.16.0": 20,
				"1.3.6.1.2.1.2.2.1.10.1": 30,
				"uptime":                 60,
			},
		},
		"success only custom OIDs unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 2)
				collr.collectIfMib = false

				if collr.EnableProfiles {
					setMockClientSysObjectidExpect(m)
				}

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: nil, Type: gosnmp.NoSuchInstance},
						{Value: nil, Type: gosnmp.NoSuchInstance},
						{Value: nil, Type: gosnmp.NoSuchObject},
						{Value: "192.0.2.0", Type: gosnmp.NsapAddress},
						{Value: []uint8{118, 101, 116}, Type: gosnmp.OctetString},
						{Value: ".1.3.6.1.2.1.4.32.1.5.2.1.4.10.19.0.0.16", Type: gosnmp.ObjectIdentifier},
					},
				}, nil).Times(1)

				return collr
			},
			wantCollected: map[string]int64{
				//"TestMetric": 1,
				"uptime": 60,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			setMockClientInitExpect(mockSNMP)
			setMockClientSysinfoAndUptimeExpect(mockSNMP)

			collr := test.prepareSNMP(mockSNMP)
			collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

			require.NoError(t, collr.Init(context.Background()))

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

			if collr.EnableProfiles {
				mx["TestMetric"] = 1
			}

			assert.Equal(t, test.wantCollected, mx)
		})
	}
}

func mockInit(t *testing.T) (*snmpmock.MockHandler, func()) {
	mockCtl := gomock.NewController(t)
	cleanup := func() { mockCtl.Finish() }
	mockSNMP := snmpmock.NewMockHandler(mockCtl)

	return mockSNMP, cleanup
}

func prepareV3Config() Config {
	cfg := prepareV2Config()
	cfg.Options.Version = gosnmp.Version3.String()
	cfg.User = User{
		Name:          "name",
		SecurityLevel: "authPriv",
		AuthProto:     strings.ToLower(gosnmp.MD5.String()),
		AuthKey:       "auth_key",
		PrivProto:     strings.ToLower(gosnmp.AES.String()),
		PrivKey:       "priv_key",
	}
	return cfg
}

func prepareV2Config() Config {
	cfg := prepareV1Config()
	cfg.Options.Version = gosnmp.Version2c.String()
	return cfg
}

func prepareV1Config() Config {
	return Config{
		UpdateEvery: 1,
		Hostname:    "192.0.2.1",
		Community:   "public",
		Options: Options{
			Port:           161,
			Retries:        1,
			Timeout:        5,
			Version:        gosnmp.Version1.String(),
			MaxOIDs:        60,
			MaxRepetitions: 25,
		},
	}
}

func prepareConfigWithUserCharts(cfg Config, start, end int) Config {
	if start > end || start < 0 || end < 1 {
		panic(fmt.Sprintf("invalid index range ('%d'-'%d')", start, end))
	}
	cfg.ChartsInput = []ChartConfig{
		{
			ID:       "test_chart1",
			Title:    "This is Test Chart1",
			Units:    "kilobits/s",
			Family:   "family",
			Type:     module.Area.String(),
			Priority: module.Priority,
			Dimensions: []DimensionConfig{
				{
					OID:        "1.3.6.1.2.1.2.2.1.10",
					Name:       "in",
					Algorithm:  module.Incremental.String(),
					Multiplier: 8,
					Divisor:    1000,
				},
				{
					OID:        "1.3.6.1.2.1.2.2.1.16",
					Name:       "out",
					Algorithm:  module.Incremental.String(),
					Multiplier: 8,
					Divisor:    1000,
				},
			},
		},
	}

	for i := range cfg.ChartsInput {
		cfg.ChartsInput[i].IndexRange = []int{start, end}
	}

	return cfg
}

func setMockClientInitExpect(m *snmpmock.MockHandler) {
	m.EXPECT().Target().AnyTimes()
	m.EXPECT().Port().AnyTimes()
	m.EXPECT().Version().AnyTimes()
	m.EXPECT().Community().AnyTimes()
	m.EXPECT().SetTarget(gomock.Any()).AnyTimes()
	m.EXPECT().SetPort(gomock.Any()).AnyTimes()
	m.EXPECT().SetRetries(gomock.Any()).AnyTimes()
	m.EXPECT().SetMaxRepetitions(gomock.Any()).AnyTimes()
	m.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
	m.EXPECT().SetLogger(gomock.Any()).AnyTimes()
	m.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
	m.EXPECT().SetCommunity(gomock.Any()).AnyTimes()
	m.EXPECT().SetVersion(gomock.Any()).AnyTimes()
	m.EXPECT().SetSecurityModel(gomock.Any()).AnyTimes()
	m.EXPECT().SetMsgFlags(gomock.Any()).AnyTimes()
	m.EXPECT().SetSecurityParameters(gomock.Any()).AnyTimes()
	m.EXPECT().Connect().Return(nil).AnyTimes()
}

func setMockClientSysObjectidExpect(m *snmpmock.MockHandler) {
	m.EXPECT().Get([]string{snmpsd.OidSysObject}).Return(&gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Value: ".1.1.1",
				Name: ".1.3.6.1.2.1.1.2.0",
				Type: gosnmp.ObjectIdentifier},
		},
	}, nil).MinTimes(1)
	m.EXPECT().Get([]string{"1.1.1.0"}).Return(&gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Name: "1.1.1.0", Value: 1, Type: gosnmp.Integer},
		},
	}, nil).MinTimes(1)

}

func setMockClientSysInfoExpect(m *snmpmock.MockHandler) {
	m.EXPECT().WalkAll(snmpsd.RootOidMibSystem).Return([]gosnmp.SnmpPDU{
		{Name: snmpsd.OidSysDescr, Value: []uint8("mock sysDescr"), Type: gosnmp.OctetString},
		{Name: snmpsd.OidSysObject, Value: ".1.3.6.1.4.1.14988.1", Type: gosnmp.ObjectIdentifier},
		{Name: snmpsd.OidSysContact, Value: []uint8("mock sysContact"), Type: gosnmp.OctetString},
		{Name: snmpsd.OidSysName, Value: []uint8("mock sysName"), Type: gosnmp.OctetString},
		{Name: snmpsd.OidSysLocation, Value: []uint8("mock sysLocation"), Type: gosnmp.OctetString},
	}, nil).MinTimes(1)
}

func setMockClientSysinfoAndUptimeExpect(m *snmpmock.MockHandler) {
	setMockClientSysInfoExpect(m)

	m.EXPECT().Get([]string{snmpsd.OidSysUptime}).Return(&gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Value: uint32(6048), Type: gosnmp.TimeTicks},
		},
	}, nil).MinTimes(1)
}

func setMockClientIfMibExpect(m *snmpmock.MockHandler) {
	m.EXPECT().WalkAll(rootOidIfMibIfTable).Return([]gosnmp.SnmpPDU{
		{Name: oidIfIndex + ".1", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfIndex + ".2", Value: 2, Type: gosnmp.Integer},
		{Name: oidIfIndex + ".17", Value: 17, Type: gosnmp.Integer},
		{Name: oidIfIndex + ".18", Value: 18, Type: gosnmp.Integer},
		{Name: oidIfDescr + ".1", Value: []uint8("ether1"), Type: gosnmp.OctetString},
		{Name: oidIfDescr + ".2", Value: []uint8("ether2"), Type: gosnmp.OctetString},
		{Name: oidIfDescr + ".17", Value: []uint8("sfp-sfpplus2"), Type: gosnmp.OctetString},
		{Name: oidIfDescr + ".18", Value: []uint8("sfp-sfpplus1"), Type: gosnmp.OctetString},
		{Name: oidIfType + ".1", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfType + ".2", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfType + ".17", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfType + ".18", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfMtu + ".1", Value: 1500, Type: gosnmp.Integer},
		{Name: oidIfMtu + ".2", Value: 1500, Type: gosnmp.Integer},
		{Name: oidIfMtu + ".17", Value: 1500, Type: gosnmp.Integer},
		{Name: oidIfMtu + ".18", Value: 1500, Type: gosnmp.Integer},
		{Name: oidIfSpeed + ".1", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfSpeed + ".2", Value: 1000000000, Type: gosnmp.Gauge32},
		{Name: oidIfSpeed + ".17", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfSpeed + ".18", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfPhysAddress + ".1", Value: decodePhysAddr("18:fd:74:7e:c5:80"), Type: gosnmp.OctetString},
		{Name: oidIfPhysAddress + ".2", Value: decodePhysAddr("18:fd:74:7e:c5:81"), Type: gosnmp.OctetString},
		{Name: oidIfPhysAddress + ".17", Value: decodePhysAddr("18:fd:74:7e:c5:90"), Type: gosnmp.OctetString},
		{Name: oidIfPhysAddress + ".18", Value: decodePhysAddr("18:fd:74:7e:c5:91"), Type: gosnmp.OctetString},
		{Name: oidIfAdminStatus + ".1", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfAdminStatus + ".2", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfAdminStatus + ".17", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfAdminStatus + ".18", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfOperStatus + ".1", Value: 2, Type: gosnmp.Integer},
		{Name: oidIfOperStatus + ".2", Value: 1, Type: gosnmp.Integer},
		{Name: oidIfOperStatus + ".17", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfOperStatus + ".18", Value: 6, Type: gosnmp.Integer},
		{Name: oidIfLastChange + ".1", Value: 0, Type: gosnmp.TimeTicks},
		{Name: oidIfLastChange + ".2", Value: 3243, Type: gosnmp.TimeTicks},
		{Name: oidIfLastChange + ".17", Value: 0, Type: gosnmp.TimeTicks},
		{Name: oidIfLastChange + ".18", Value: 0, Type: gosnmp.TimeTicks},
		{Name: oidIfInOctets + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInOctets + ".2", Value: 3827243723, Type: gosnmp.Counter32},
		{Name: oidIfInOctets + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInOctets + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUcastPkts + ".2", Value: 71035992, Type: gosnmp.Counter32},
		{Name: oidIfInUcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInNUcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInNUcastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInNUcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInNUcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInDiscards + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInDiscards + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInDiscards + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInDiscards + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInErrors + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInErrors + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInErrors + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInErrors + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUnknownProtos + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUnknownProtos + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUnknownProtos + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInUnknownProtos + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutOctets + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutOctets + ".2", Value: 2769838772, Type: gosnmp.Counter32},
		{Name: oidIfOutOctets + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutOctets + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutUcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutUcastPkts + ".2", Value: 39482929, Type: gosnmp.Counter32},
		{Name: oidIfOutUcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutUcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutNUcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutNUcastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutNUcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutNUcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutDiscards + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutDiscards + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutDiscards + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutDiscards + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutErrors + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutErrors + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutErrors + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutErrors + ".18", Value: 0, Type: gosnmp.Counter32},
	}, nil).MinTimes(1)

	m.EXPECT().WalkAll(rootOidIfMibIfXTable).Return([]gosnmp.SnmpPDU{
		{Name: oidIfName + ".1", Value: []uint8("ether1"), Type: gosnmp.OctetString},
		{Name: oidIfName + ".2", Value: []uint8("ether2"), Type: gosnmp.OctetString},
		{Name: oidIfName + ".17", Value: []uint8("sfp-sfpplus2"), Type: gosnmp.OctetString},
		{Name: oidIfName + ".18", Value: []uint8("sfp-sfpplus1"), Type: gosnmp.OctetString},
		{Name: oidIfInMulticastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInMulticastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInMulticastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInMulticastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInBroadcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInBroadcastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInBroadcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfInBroadcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutMulticastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutMulticastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutMulticastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutMulticastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutBroadcastPkts + ".1", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutBroadcastPkts + ".2", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutBroadcastPkts + ".17", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfOutBroadcastPkts + ".18", Value: 0, Type: gosnmp.Counter32},
		{Name: oidIfHCInOctets + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInOctets + ".2", Value: 76882188712, Type: gosnmp.Counter64},
		{Name: oidIfHCInOctets + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInOctets + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInUcastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInUcastPkts + ".2", Value: 71080332, Type: gosnmp.Counter64},
		{Name: oidIfHCInUcastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInUcastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInMulticastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInMulticastPkts + ".2", Value: 1891, Type: gosnmp.Counter64},
		{Name: oidIfHCInMulticastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInMulticastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInBroadcastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInBroadcastPkts + ".2", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInBroadcastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCInBroadcastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutOctets + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutOctets + ".2", Value: 19959650810, Type: gosnmp.Counter64},
		{Name: oidIfHCOutOctets + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutOctets + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutUcastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutUcastPkts + ".2", Value: 39509661, Type: gosnmp.Counter64},
		{Name: oidIfHCOutUcastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutUcastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutMulticastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutMulticastPkts + ".2", Value: 28844, Type: gosnmp.Counter64},
		{Name: oidIfHCOutMulticastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutMulticastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutBroadcastPkts + ".1", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutBroadcastPkts + ".2", Value: 7386, Type: gosnmp.Counter64},
		{Name: oidIfHCOutBroadcastPkts + ".17", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHCOutBroadcastPkts + ".18", Value: 0, Type: gosnmp.Counter64},
		{Name: oidIfHighSpeed + ".1", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfHighSpeed + ".2", Value: 1000, Type: gosnmp.Gauge32},
		{Name: oidIfHighSpeed + ".17", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfHighSpeed + ".18", Value: 0, Type: gosnmp.Gauge32},
		{Name: oidIfAlias + ".1", Value: []uint8(""), Type: gosnmp.OctetString},
		{Name: oidIfAlias + ".2", Value: []uint8("UPLINK2 (2.1)"), Type: gosnmp.OctetString},
		{Name: oidIfAlias + ".17", Value: []uint8(""), Type: gosnmp.OctetString},
		{Name: oidIfAlias + ".18", Value: []uint8(""), Type: gosnmp.OctetString},
	}, nil).MinTimes(1)
}

func decodePhysAddr(s string) []uint8 {
	s = strings.ReplaceAll(s, ":", "")
	v, _ := hex.DecodeString(s)
	return v
}
