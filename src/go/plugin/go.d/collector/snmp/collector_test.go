// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
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
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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
							Source: "test",
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
				"snmp_device_prof_test_stats_errors_processing_scalar": 0,
				"snmp_device_prof_test_stats_errors_processing_table":  0,
				"snmp_device_prof_test_stats_errors_snmp":              0,
				"snmp_device_prof_test_stats_metrics_rows":             0,
				"snmp_device_prof_test_stats_metrics_scalar":           0,
				"snmp_device_prof_test_stats_metrics_table":            0,
				"snmp_device_prof_test_stats_metrics_tables":           0,
				"snmp_device_prof_test_stats_metrics_virtual":          0,
				"snmp_device_prof_test_stats_snmp_get_oids":            0,
				"snmp_device_prof_test_stats_snmp_get_requests":        0,
				"snmp_device_prof_test_stats_snmp_tables_cached":       0,
				"snmp_device_prof_test_stats_snmp_tables_walked":       0,
				"snmp_device_prof_test_stats_snmp_walk_pdus":           0,
				"snmp_device_prof_test_stats_snmp_walk_requests":       0,
				"snmp_device_prof_test_stats_table_cache_hits":         0,
				"snmp_device_prof_test_stats_table_cache_misses":       0,
				"snmp_device_prof_test_stats_timings_scalar":           0,
				"snmp_device_prof_test_stats_timings_table":            0,
				"snmp_device_prof_test_stats_timings_virtual":          0,
				"snmp_device_prof_uptime":                              123,
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
							Source: "test",
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
				"snmp_device_prof_test_stats_errors_processing_scalar": 0,
				"snmp_device_prof_test_stats_errors_processing_table":  0,
				"snmp_device_prof_test_stats_errors_snmp":              0,
				"snmp_device_prof_test_stats_metrics_rows":             0,
				"snmp_device_prof_test_stats_metrics_scalar":           0,
				"snmp_device_prof_test_stats_metrics_table":            0,
				"snmp_device_prof_test_stats_metrics_tables":           0,
				"snmp_device_prof_test_stats_metrics_virtual":          0,
				"snmp_device_prof_test_stats_snmp_get_oids":            0,
				"snmp_device_prof_test_stats_snmp_get_requests":        0,
				"snmp_device_prof_test_stats_snmp_tables_cached":       0,
				"snmp_device_prof_test_stats_snmp_tables_walked":       0,
				"snmp_device_prof_test_stats_snmp_walk_pdus":           0,
				"snmp_device_prof_test_stats_snmp_walk_requests":       0,
				"snmp_device_prof_test_stats_table_cache_hits":         0,
				"snmp_device_prof_test_stats_table_cache_misses":       0,
				"snmp_device_prof_test_stats_timings_scalar":           0,
				"snmp_device_prof_test_stats_timings_table":            0,
				"snmp_device_prof_test_stats_timings_virtual":          0,
				"snmp_device_prof_if_octets_eth0_in":                   1,
				"snmp_device_prof_if_octets_eth0_out":                  2,
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

func TestCollector_Collect_LicensingAggregation(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	expiry := time.Now().UTC().Add(48 * time.Hour).Unix()

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		pm := &ddsnmp.ProfileMetrics{
			Source: "checkpoint.yaml",
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name:  licenseSourceMetricName,
					Value: 17,
					Tags: map[string]string{
						tagLicenseID:          "17",
						tagLicenseName:        "Application Control",
						tagLicenseStateRaw:    "about-to-expire",
						tagLicenseExpiryRaw:   strconv.FormatInt(expiry, 10),
						tagLicenseUsageRaw:    "95",
						tagLicenseCapacityRaw: "100",
					},
				},
			},
		}
		for i := range pm.HiddenMetrics {
			pm.HiddenMetrics[i].Profile = pm
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	got := collr.Collect(context.Background())
	require.NotNil(t, got)

	assert.EqualValues(t, 0, got[metricIDLicenseStateHealthy])
	assert.EqualValues(t, 1, got[metricIDLicenseStateDegraded])
	assert.EqualValues(t, 0, got[metricIDLicenseStateBroken])
	assert.EqualValues(t, 0, got[metricIDLicenseStateIgnored])
	assert.EqualValues(t, 95, got[metricIDLicenseUsagePercent])
	assert.GreaterOrEqual(t, got[metricIDLicenseRemainingTime], int64((48*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseRemainingTime], int64(48*time.Hour/time.Second))
	assert.Contains(t, got, "snmp_device_prof_checkpoint_stats_metrics_table")
}

func TestCollector_Collect_LicensingAggregation_CiscoSmartPartialData(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	authExpiry := time.Now().UTC().Add(48 * time.Hour).Unix()
	certExpiry := time.Now().UTC().Add(72 * time.Hour).Unix()

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		pm := &ddsnmp.ProfileMetrics{
			Source: "cisco.yaml",
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name:  licenseSourceMetricName,
					Value: 2,
					StaticTags: map[string]string{
						tagLicenseID:        "smart_authorization_state",
						tagLicenseName:      "Smart Licensing authorization state",
						tagLicenseComponent: "smart_licensing",
						tagLicenseType:      "authorization",
						tagLicenseValueKind: licenseValueKindStateSeverity,
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: authExpiry,
					StaticTags: map[string]string{
						tagLicenseID:                  "smart_authorization_expiry",
						tagLicenseName:                "Smart Licensing authorization",
						tagLicenseComponent:           "smart_licensing",
						tagLicenseType:                "authorization",
						tagLicenseValueKind:           licenseValueKindAuthorizationTimestamp,
						tagLicenseAuthorizationSource: "ciscoSlaAuthExpireTime",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: certExpiry,
					StaticTags: map[string]string{
						tagLicenseID:                "smart_id_certificate_expiry",
						tagLicenseName:              "Smart Licensing ID certificate",
						tagLicenseComponent:         "smart_licensing",
						tagLicenseType:              "certificate",
						tagLicenseValueKind:         licenseValueKindCertificateTimestamp,
						tagLicenseCertificateSource: "ciscoSlaNextCertificateExpireTime",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 1,
					Tags: map[string]string{
						tagLicenseID:               "dna_advantage",
						tagLicenseName:             "network-advantage",
						tagLicenseStateRaw:         "authorization_expired",
						tagLicenseStateSeverityRaw: "2",
						tagLicenseUsageRaw:         "42",
					},
					StaticTags: map[string]string{
						tagLicenseComponent: "smart_licensing",
						tagLicenseType:      "entitlement",
					},
				},
			},
		}
		for i := range pm.HiddenMetrics {
			pm.HiddenMetrics[i].Profile = pm
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	got := collr.Collect(context.Background())
	require.NotNil(t, got)

	assert.EqualValues(t, 2, got[metricIDLicenseStateHealthy])
	assert.EqualValues(t, 0, got[metricIDLicenseStateDegraded])
	assert.EqualValues(t, 2, got[metricIDLicenseStateBroken])
	assert.EqualValues(t, 0, got[metricIDLicenseStateIgnored])
	assert.GreaterOrEqual(t, got[metricIDLicenseAuthorizationRemainingTime], int64((48*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseAuthorizationRemainingTime], int64(48*time.Hour/time.Second))
	assert.GreaterOrEqual(t, got[metricIDLicenseCertificateRemainingTime], int64((72*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseCertificateRemainingTime], int64(72*time.Hour/time.Second))
	assert.NotContains(t, got, metricIDLicenseRemainingTime)
	assert.NotContains(t, got, metricIDLicenseGraceRemainingTime)
	assert.NotContains(t, got, metricIDLicenseUsagePercent)
}

func TestCollector_Collect_LicensingAggregation_CiscoTraditional(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	expiry := time.Now().UTC().Add(72 * time.Hour).Unix()

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		pm := &ddsnmp.ProfileMetrics{
			Source: "cisco.yaml",
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:           "17",
						tagLicenseName:         "SECURITYK9",
						tagLicenseType:         "paid_subscription",
						tagLicenseExpiryRaw:    strconv.FormatInt(expiry, 10),
						tagLicenseCapacityRaw:  "100",
						tagLicenseAvailableRaw: "15",
						tagLicenseStateRaw:     "in_use",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 3600,
					Tags: map[string]string{
						tagLicenseID:               "23",
						tagLicenseName:             "APPXK9",
						tagLicenseType:             "grace_period",
						tagLicenseCapacityRaw:      "10",
						tagLicenseAvailableRaw:     "0",
						tagLicenseStateRaw:         "usage_count_consumed",
						tagLicenseStateSeverityRaw: "2",
						tagLicenseValueKind:        licenseValueKindExpiryRemaining,
						tagLicenseComponent:        "traditional_licensing",
					},
				},
			},
		}
		for i := range pm.HiddenMetrics {
			pm.HiddenMetrics[i].Profile = pm
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	got := collr.Collect(context.Background())
	require.NotNil(t, got)

	assert.EqualValues(t, 1, got[metricIDLicenseStateHealthy])
	assert.EqualValues(t, 0, got[metricIDLicenseStateDegraded])
	assert.EqualValues(t, 1, got[metricIDLicenseStateBroken])
	assert.EqualValues(t, 0, got[metricIDLicenseStateIgnored])
	assert.EqualValues(t, 100, got[metricIDLicenseUsagePercent])
	assert.GreaterOrEqual(t, got[metricIDLicenseRemainingTime], int64((time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseRemainingTime], int64(time.Hour/time.Second))
}

func TestCollector_Collect_LicensingAggregation_SelectsWorstCaseAcrossMixedRows(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	now := time.Now().UTC()
	perpetualExpiry := now.Add(30 * time.Minute).Unix()
	earliestRealExpiry := now.Add(6 * time.Hour).Unix()
	authExpiry := now.Add(30 * time.Hour).Unix()
	certExpiry := now.Add(20 * time.Hour).Unix()
	graceExpiry := now.Add(10 * time.Hour).Unix()

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		pm := &ddsnmp.ProfileMetrics{
			Source: "mixed-licensing.yaml",
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:          "perpetual",
						tagLicenseName:        "Perpetual base",
						tagLicenseExpiryRaw:   strconv.FormatInt(perpetualExpiry, 10),
						tagLicensePerpetual:   "true",
						tagLicenseStateRaw:    "active",
						tagLicenseUsageRaw:    "50",
						tagLicenseCapacityRaw: "100",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:          "soonest_expiring",
						tagLicenseName:        "Threat prevention",
						tagLicenseExpiryRaw:   strconv.FormatInt(earliestRealExpiry, 10),
						tagLicenseStateRaw:    "about-to-expire",
						tagLicenseUsageRaw:    "90",
						tagLicenseCapacityRaw: "100",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:                  "auth",
						tagLicenseName:                "Smart auth",
						tagLicenseAuthorizationRaw:    strconv.FormatInt(authExpiry, 10),
						tagLicenseAuthorizationSource: "auth_timer",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:                "cert",
						tagLicenseName:              "Smart cert",
						tagLicenseCertificateRaw:    strconv.FormatInt(certExpiry, 10),
						tagLicenseCertificateSource: "cert_timer",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:               "grace",
						tagLicenseName:             "Eval grace",
						tagLicenseGraceRaw:         strconv.FormatInt(graceExpiry, 10),
						tagLicenseStateSeverityRaw: "1",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:               "broken",
						tagLicenseName:             "Broken feature",
						tagLicenseStateSeverityRaw: "2",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:              "unlimited",
						tagLicenseName:            "Unlimited pool",
						tagLicenseUsagePercentRaw: "100",
						tagLicenseUnlimited:       "true",
					},
				},
			},
		}
		for i := range pm.HiddenMetrics {
			pm.HiddenMetrics[i].Profile = pm
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	got := collr.Collect(context.Background())
	require.NotNil(t, got)

	assert.EqualValues(t, 4, got[metricIDLicenseStateHealthy])
	assert.EqualValues(t, 2, got[metricIDLicenseStateDegraded])
	assert.EqualValues(t, 1, got[metricIDLicenseStateBroken])
	assert.EqualValues(t, 0, got[metricIDLicenseStateIgnored])
	assert.EqualValues(t, 90, got[metricIDLicenseUsagePercent])
	assert.GreaterOrEqual(t, got[metricIDLicenseRemainingTime], int64((6*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseRemainingTime], int64(6*time.Hour/time.Second))
	assert.GreaterOrEqual(t, got[metricIDLicenseAuthorizationRemainingTime], int64((30*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseAuthorizationRemainingTime], int64(30*time.Hour/time.Second))
	assert.GreaterOrEqual(t, got[metricIDLicenseCertificateRemainingTime], int64((20*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseCertificateRemainingTime], int64(20*time.Hour/time.Second))
	assert.GreaterOrEqual(t, got[metricIDLicenseGraceRemainingTime], int64((10*time.Hour/time.Second)-5))
	assert.LessOrEqual(t, got[metricIDLicenseGraceRemainingTime], int64(10*time.Hour/time.Second))
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
