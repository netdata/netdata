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

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"

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
		"custom, no if-mib": {
			wantNumCharts: 10,
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 9)

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
				m.EXPECT().WalkAll(snmputils.RootOidMibSystem).Return(nil, errors.New("mock Get() error")).Times(1)

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
		"success only custom OIDs supported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 3)
				collr.enableProfiles = false

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
			},
		},
		"success only custom OIDs supported and unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 2)
				collr.enableProfiles = false

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
			},
		},
		"fails when only custom OIDs unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *Collector {
				collr := New()
				collr.Config = prepareConfigWithUserCharts(prepareV2Config(), 0, 2)
				collr.enableProfiles = false

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
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			setMockClientInitExpect(mockSNMP)
			setMockClientSysInfoExpect(mockSNMP)

			collr := test.prepareSNMP(mockSNMP)
			collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }

			require.NoError(t, collr.Init(context.Background()))

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

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
	cfg.User = UserConfig{
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
		Options: OptionsConfig{
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
	m.EXPECT().MaxRepetitions().Return(uint32(25)).AnyTimes()
}

func setMockClientSysObjectidExpect(m *snmpmock.MockHandler) {
	m.EXPECT().Get([]string{snmputils.OidSysObject}).Return(&gosnmp.SnmpPacket{
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
	m.EXPECT().WalkAll(snmputils.RootOidMibSystem).Return([]gosnmp.SnmpPDU{
		{Name: snmputils.OidSysDescr, Value: []uint8("mock sysDescr"), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysObject, Value: ".1.3.6.1.4.1.14988.1", Type: gosnmp.ObjectIdentifier},
		{Name: snmputils.OidSysContact, Value: []uint8("mock sysContact"), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysName, Value: []uint8("mock sysName"), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysLocation, Value: []uint8("mock sysLocation"), Type: gosnmp.OctetString},
	}, nil).MinTimes(1)
}

func decodePhysAddr(s string) []uint8 {
	s = strings.ReplaceAll(s, ":", "")
	v, _ := hex.DecodeString(s)
	return v
}
