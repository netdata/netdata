// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ping"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare func(m *snmpmock.MockHandler) *Collector
		wantErr bool
	}{
		"success: connects and reads sysInfo": {
			wantErr: false,
			prepare: func(m *snmpmock.MockHandler) *Collector {
				setMockClientInitExpect(m)
				setMockClientSysInfoExpect(m)

				c := New()
				c.Config = prepareV2Config()
				c.CreateVnode = false
				c.Ping.Enabled = false
				c.newSnmpClient = func() gosnmp.Handler { return m }
				return c
			},
		},

		"failure: SNMP connect error": {
			wantErr: true,
			prepare: func(m *snmpmock.MockHandler) *Collector {
				setMockClientSetterExpect(m)
				m.EXPECT().Connect().Return(errors.New("connect failed")).AnyTimes()

				c := New()
				c.Config = prepareV2Config()
				c.CreateVnode = false
				c.Ping.Enabled = false
				c.newSnmpClient = func() gosnmp.Handler { return m }
				return c
			},
		},

		"failure: sysInfo walk error": {
			wantErr: true,
			prepare: func(m *snmpmock.MockHandler) *Collector {
				// Normal init succeeds
				setMockClientInitExpect(m)
				// But sysInfo retrieval (WalkAll on system tree) fails
				// If your helper is too opinionated, stub directly:
				// The collector ultimately calls WalkAll on the system OID tree.
				m.EXPECT().
					WalkAll(gomock.Any()).
					Return(nil, errors.New("walk failed"))

				c := New()
				c.Config = prepareV2Config()
				c.CreateVnode = false
				c.Ping.Enabled = false
				c.newSnmpClient = func() gosnmp.Handler { return m }
				return c
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSNMP := snmpmock.NewMockHandler(ctrl)

			collr := tc.prepare(mockSNMP)
			require.NoError(t, collr.Init(context.Background()))

			err := collr.Check(context.Background())
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare func(m *snmpmock.MockHandler) *Collector
		want    map[string]int64
	}{
		"collects scalar metric": {
			prepare: func(m *snmpmock.MockHandler) *Collector {
				setMockClientInitExpect(m)
				setMockClientSysInfoExpect(m)

				collr := New()
				collr.Config = prepareV2Config()
				collr.CreateVnode = false
				collr.Ping.Enabled = false
				collr.snmpProfiles = []*ddsnmp.Profile{{}} // non-empty to enable collectSNMP()
				collr.newSnmpClient = func() gosnmp.Handler { return m }
				collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
					return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{
						{
							Metrics: []ddsnmp.Metric{
								{
									Name:    "uptime",
									IsTable: false,
									Value:   123,
									Unit:    "s",
									Tags:    map[string]string{},
									Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
								},
							},
						},
					}}
				}
				return collr
			},
			want: map[string]int64{
				// scalar → "snmp_device_prof_<name>"
				"snmp_device_prof_uptime": 123,
			},
		},
		"collects table multivalue metric": {
			prepare: func(m *snmpmock.MockHandler) *Collector {
				setMockClientInitExpect(m)
				setMockClientSysInfoExpect(m)

				collr := New()
				collr.Config = prepareV2Config()
				collr.CreateVnode = false
				collr.Ping.Enabled = false
				collr.snmpProfiles = []*ddsnmp.Profile{{}}
				collr.newSnmpClient = func() gosnmp.Handler { return m }
				collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
					return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{
						{
							Metrics: []ddsnmp.Metric{
								{
									Name:    "if_octets",
									IsTable: true,
									Unit:    "bit/s",
									Tags:    map[string]string{"ifName": "eth0"},
									Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
									MultiValue: map[string]int64{
										"in":  1,
										"out": 2,
									},
								},
							},
						},
					}}
				}
				return collr
			},
			want: map[string]int64{
				// table key: "snmp_device_prof_<name>_<sorted tag values>_<subkey>"
				// here tags = {"ifName":"eth0"} → key part becomes "_eth0"
				"snmp_device_prof_if_octets_eth0_in":  1,
				"snmp_device_prof_if_octets_eth0_out": 2,
			},
		},
		"collects ping only metrics": {
			prepare: func(m *snmpmock.MockHandler) *Collector {
				setMockClientInitExpect(m)
				setMockClientSysInfoExpect(m)

				collr := New()
				collr.Config = prepareV2Config()
				collr.PingOnly = true
				collr.CreateVnode = false
				collr.newSnmpClient = func() gosnmp.Handler { return m }
				collr.newProber = func(cfg ping.ProberConfig, log *logger.Logger) ping.Prober { return &mockProber{} }

				return collr
			},
			want: map[string]int64{
				"ping_rtt_min":    (10 * time.Millisecond).Microseconds(),
				"ping_rtt_max":    (20 * time.Millisecond).Microseconds(),
				"ping_rtt_avg":    (15 * time.Millisecond).Microseconds(),
				"ping_rtt_stddev": (5 * time.Millisecond).Microseconds(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			mockSNMP := snmpmock.NewMockHandler(mockCtl)

			collr := tc.prepare(mockSNMP)
			require.NoError(t, collr.Init(context.Background()))

			_ = collr.Check(context.Background())

			got := collr.Collect(context.Background())
			assert.Equal(t, tc.want, got)
		})
	}
}

type mockProber struct {
	errOnPing bool
}

func (m *mockProber) Ping(host string) (*probing.Statistics, error) {
	if m.errOnPing {
		return nil, errors.New("mock.Ping() error")
	}

	stats := probing.Statistics{
		PacketsRecv:           5,
		PacketsSent:           5,
		PacketsRecvDuplicates: 0,
		PacketLoss:            0,
		Addr:                  host,
		Rtts:                  nil,
		MinRtt:                time.Millisecond * 10,
		MaxRtt:                time.Millisecond * 20,
		AvgRtt:                time.Millisecond * 15,
		StdDevRtt:             time.Millisecond * 5,
	}

	return &stats, nil
}

type mockDdSnmpCollector struct {
	pms  []*ddsnmp.ProfileMetrics
	meta map[string]ddsnmp.MetaTag
	err  error
}

func (m *mockDdSnmpCollector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	return m.pms, m.err
}

func (m *mockDdSnmpCollector) CollectDeviceMetadata() (map[string]ddsnmp.MetaTag, error) {
	return m.meta, nil
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

func mockInit(t *testing.T) (*snmpmock.MockHandler, func()) {
	mockCtl := gomock.NewController(t)
	cleanup := func() { mockCtl.Finish() }
	mockSNMP := snmpmock.NewMockHandler(mockCtl)

	return mockSNMP, cleanup
}

func setMockClientInitExpect(m *snmpmock.MockHandler) {
	setMockClientSetterExpect(m)
	m.EXPECT().Connect().Return(nil).AnyTimes()
}

func setMockClientSetterExpect(m *snmpmock.MockHandler) {
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
	m.EXPECT().MaxRepetitions().Return(uint32(25)).AnyTimes()
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
