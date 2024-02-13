// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.IsType(t, (*SNMP)(nil), New())
}

func TestSNMP_Init(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP func() *SNMP
		wantFail    bool
	}{
		"fail with default config": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				return New()
			},
		},
		"fail when 'charts' not set": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				snmp.ChartsInput = nil
				return snmp
			},
		},
		"fail when using SNMPv3 but 'user.name' not set": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV3Config()
				snmp.User.Name = ""
				return snmp
			},
		},
		"fail when using SNMPv3 but 'user.level' is invalid": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV3Config()
				snmp.User.SecurityLevel = "invalid"
				return snmp
			},
		},
		"fail when using SNMPv3 but 'user.auth_proto' is invalid": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV3Config()
				snmp.User.AuthProto = "invalid"
				return snmp
			},
		},
		"fail when using SNMPv3 but 'user.priv_proto' is invalid": {
			wantFail: true,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV3Config()
				snmp.User.PrivProto = "invalid"
				return snmp
			},
		},
		"success when using SNMPv1 with valid config": {
			wantFail: false,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV1Config()
				return snmp
			},
		},
		"success when using SNMPv2 with valid config": {
			wantFail: false,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				return snmp
			},
		},
		"success when using SNMPv3 with valid config": {
			wantFail: false,
			prepareSNMP: func() *SNMP {
				snmp := New()
				snmp.Config = prepareV3Config()
				return snmp
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			snmp := test.prepareSNMP()

			if test.wantFail {
				assert.False(t, snmp.Init())
			} else {
				assert.True(t, snmp.Init())
			}
		})
	}
}

func TestSNMP_Check(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP func(m *snmpmock.MockHandler) *SNMP
		wantFail    bool
	}{
		"success when 'max_request_size' > returned OIDs": {
			wantFail: false,
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: 10, Type: gosnmp.Gauge32},
						{Value: 20, Type: gosnmp.Gauge32},
					},
				}, nil).Times(1)

				return snmp
			},
		},
		"success when 'max_request_size' < returned OIDs": {
			wantFail: false,
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				snmp.Config.Options.MaxOIDs = 1

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: 10, Type: gosnmp.Gauge32},
						{Value: 20, Type: gosnmp.Gauge32},
					},
				}, nil).Times(2)

				return snmp
			},
		},
		"success when using 'multiply_range'": {
			wantFail: false,
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareConfigWithIndexRange(prepareV2Config, 0, 1)

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: 10, Type: gosnmp.Gauge32},
						{Value: 20, Type: gosnmp.Gauge32},
						{Value: 30, Type: gosnmp.Gauge32},
						{Value: 40, Type: gosnmp.Gauge32},
					},
				}, nil).Times(1)

				return snmp
			},
		},
		"fail when snmp client Get fails": {
			wantFail: true,
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()

				m.EXPECT().Get(gomock.Any()).Return(nil, errors.New("mock Get() error")).Times(1)

				return snmp
			},
		},
		"fail when all OIDs type is unsupported": {
			wantFail: true,
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()

				m.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						{Value: nil, Type: gosnmp.NoSuchInstance},
						{Value: nil, Type: gosnmp.NoSuchInstance},
					},
				}, nil).Times(1)

				return snmp
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			newSNMPClient = func() gosnmp.Handler { return mockSNMP }
			defaultMockExpects(mockSNMP)

			snmp := test.prepareSNMP(mockSNMP)
			require.True(t, snmp.Init())

			if test.wantFail {
				assert.False(t, snmp.Check())
			} else {
				assert.True(t, snmp.Check())
			}
		})
	}
}

func TestSNMP_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP   func(m *snmpmock.MockHandler) *SNMP
		wantCollected map[string]int64
	}{
		"success when collecting supported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareConfigWithIndexRange(prepareV2Config, 0, 3)

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

				return snmp
			},
			wantCollected: map[string]int64{
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
		"success when collecting supported and unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareConfigWithIndexRange(prepareV2Config, 0, 2)

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

				return snmp
			},
			wantCollected: map[string]int64{
				"1.3.6.1.2.1.2.2.1.10.0": 10,
				"1.3.6.1.2.1.2.2.1.16.0": 20,
				"1.3.6.1.2.1.2.2.1.10.1": 30,
			},
		},
		"fails when collecting unsupported type": {
			prepareSNMP: func(m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareConfigWithIndexRange(prepareV2Config, 0, 2)

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

				return snmp
			},
			wantCollected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			newSNMPClient = func() gosnmp.Handler { return mockSNMP }
			defaultMockExpects(mockSNMP)

			snmp := test.prepareSNMP(mockSNMP)
			require.True(t, snmp.Init())

			collected := snmp.Collect()

			assert.Equal(t, test.wantCollected, collected)
		})
	}
}

func TestSNMP_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP func(t *testing.T, m *snmpmock.MockHandler) *SNMP
	}{
		"cleanup call if snmpClient initialized": {
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				require.True(t, snmp.Init())

				m.EXPECT().Close().Times(1)

				return snmp
			},
		},
		"cleanup call does not panic if snmpClient not initialized": {
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				require.True(t, snmp.Init())
				snmp.snmpClient = nil

				return snmp
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			newSNMPClient = func() gosnmp.Handler { return mockSNMP }
			defaultMockExpects(mockSNMP)

			snmp := test.prepareSNMP(t, mockSNMP)
			assert.NotPanics(t, snmp.Cleanup)
		})
	}
}

func TestSNMP_Charts(t *testing.T) {
	tests := map[string]struct {
		prepareSNMP   func(t *testing.T, m *snmpmock.MockHandler) *SNMP
		wantNumCharts int
	}{
		"without 'multiply_range': got expected number of charts": {
			wantNumCharts: 1,
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareV2Config()
				require.True(t, snmp.Init())

				return snmp
			},
		},
		"with 'multiply_range': got expected number of charts": {
			wantNumCharts: 10,
			prepareSNMP: func(t *testing.T, m *snmpmock.MockHandler) *SNMP {
				snmp := New()
				snmp.Config = prepareConfigWithIndexRange(prepareV2Config, 0, 9)
				require.True(t, snmp.Init())

				return snmp
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockSNMP, cleanup := mockInit(t)
			defer cleanup()

			newSNMPClient = func() gosnmp.Handler { return mockSNMP }
			defaultMockExpects(mockSNMP)

			snmp := test.prepareSNMP(t, mockSNMP)
			assert.Equal(t, test.wantNumCharts, len(*snmp.Charts()))
		})
	}
}

func mockInit(t *testing.T) (*snmpmock.MockHandler, func()) {
	mockCtl := gomock.NewController(t)
	cleanup := func() { mockCtl.Finish() }
	mockSNMP := snmpmock.NewMockHandler(mockCtl)

	return mockSNMP, cleanup
}

func defaultMockExpects(m *snmpmock.MockHandler) {
	m.EXPECT().Target().AnyTimes()
	m.EXPECT().Port().AnyTimes()
	m.EXPECT().Retries().AnyTimes()
	m.EXPECT().Timeout().AnyTimes()
	m.EXPECT().MaxOids().AnyTimes()
	m.EXPECT().Version().AnyTimes()
	m.EXPECT().Community().AnyTimes()
	m.EXPECT().SetTarget(gomock.Any()).AnyTimes()
	m.EXPECT().SetPort(gomock.Any()).AnyTimes()
	m.EXPECT().SetRetries(gomock.Any()).AnyTimes()
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

func prepareConfigWithIndexRange(p func() Config, start, end int) Config {
	if start > end || start < 0 || end < 1 {
		panic(fmt.Sprintf("invalid index range ('%d'-'%d')", start, end))
	}
	cfg := p()
	for i := range cfg.ChartsInput {
		cfg.ChartsInput[i].IndexRange = []int{start, end}
	}
	return cfg
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
		UpdateEvery: defaultUpdateEvery,
		Hostname:    defaultHostname,
		Community:   defaultCommunity,
		Options: Options{
			Port:    defaultPort,
			Retries: defaultRetries,
			Timeout: defaultTimeout,
			Version: gosnmp.Version1.String(),
			MaxOIDs: defaultMaxOIDs,
		},
		ChartsInput: []ChartConfig{
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
		},
	}
}
