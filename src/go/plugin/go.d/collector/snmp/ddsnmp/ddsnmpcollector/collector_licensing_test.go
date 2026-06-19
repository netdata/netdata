// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_LicenseRowsFromScalarLicensingConfig(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler,
		[]string{
			"1.3.6.1.4.1.99999.1.1.0",
			"1.3.6.1.4.1.99999.1.2.0",
			"1.3.6.1.4.1.99999.1.3.0",
		},
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.99999.1.1.0", "active"),
			createStringPDU("1.3.6.1.4.1.99999.1.2.0", "2031-11-11"),
			createStringPDU("1.3.6.1.4.1.99999.1.3.0", "datacenter-a"),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "vendor-device.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Licensing: []ddprofiledefinition.LicensingConfig{
				{
					OriginProfileID: "_vendor-licensing.yaml",
					ID:              "scalar-license-group",
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID:   ddprofiledefinition.LicenseValueConfig{Value: "license-a"},
						Name: ddprofiledefinition.LicenseValueConfig{Value: "License A"},
					},
					Descriptors: ddprofiledefinition.LicenseDescriptorsConfig{
						Type:      ddprofiledefinition.LicenseValueConfig{Value: "subscription"},
						Perpetual: ddprofiledefinition.LicenseValueConfig{Value: "false"},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.1.1.0",
								Name: "licenseState",
								Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
									"active": "0",
								}),
							},
						},
						Policy: ddprofiledefinition.LicenseStatePolicyDefault,
					},
					Signals: ddprofiledefinition.LicenseSignalsConfig{
						Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{
							LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
								From:     "1.3.6.1.4.1.99999.1.2.0",
								Format:   "text_date",
								Sentinel: []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerZeroOrNegative},
							},
						},
					},
					MetricTags: ddprofiledefinition.MetricTagConfigList{
						{
							Tag: "license_site",
							Symbol: ddprofiledefinition.SymbolConfigCompat(ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.1.3.0",
								Name: "licenseSite",
							}),
						},
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Empty(t, pm.HiddenMetrics)
	require.Empty(t, pm.Metrics)
	require.Empty(t, pm.TopologyMetrics)
	require.Len(t, pm.LicenseRows, 1)

	row := pm.LicenseRows[0]
	assert.Equal(t, "_vendor-licensing.yaml", row.OriginProfileID)
	assert.Equal(t, "scalar-license-group", row.RowKey)
	assert.Equal(t, "_vendor-licensing.yaml|scalar|scalar-license-group", row.StructuralID)
	assert.Equal(t, "license-a", row.ID)
	assert.Equal(t, "License A", row.Name)
	assert.Equal(t, "subscription", row.Type)
	assert.False(t, row.IsPerpetual)
	assert.True(t, row.State.Has)
	assert.EqualValues(t, 0, row.State.Severity)
	assert.Equal(t, "active", row.State.Raw)
	assert.Equal(t, "1.3.6.1.4.1.99999.1.1.0", row.State.SourceOID)
	assert.True(t, row.Expiry.Has)
	assert.Equal(t, time.Date(2031, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), row.Expiry.Timestamp)
	assert.Equal(t, "1.3.6.1.4.1.99999.1.2.0", row.Expiry.SourceOID)
	assert.Equal(t, map[string]string{"license_site": "datacenter-a"}, row.Tags)
}

func TestCollector_Collect_LicenseRowsBestEffortForRegularMetrics(t *testing.T) {
	tests := map[string]struct {
		licensePDU gosnmp.SnmpPDU
	}{
		"invalid licensing state does not drop scalar metric": {
			licensePDU: createStringPDU("1.3.6.1.4.1.99999.7.1.0", "not-a-number"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456)},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.7.1.0"},
				[]gosnmp.SnmpPDU{tc.licensePDU},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							Metrics: []ddprofiledefinition.MetricsConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.3.0",
										Name: "sysUpTime",
									},
								},
							},
							Licensing: []ddprofiledefinition.LicensingConfig{
								{
									OriginProfileID: "_vendor-licensing.yaml",
									ID:              "bad-license-row",
									Identity: ddprofiledefinition.LicenseIdentityConfig{
										ID: ddprofiledefinition.LicenseValueConfig{Value: "bad-license-row"},
									},
									State: ddprofiledefinition.LicenseStateConfig{
										LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
											Symbol: ddprofiledefinition.SymbolConfig{
												OID:  "1.3.6.1.4.1.99999.7.1.0",
												Name: "licenseState",
											},
										},
									},
								},
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].Metrics, 1)
			assert.Equal(t, "sysUpTime", results[0].Metrics[0].Name)
			assert.Empty(t, results[0].LicenseRows)
			assert.EqualValues(t, 1, results[0].Stats.Errors.Processing.Licensing)
		})
	}
}

func TestCollector_Collect_LicenseRowsSkipsKnownMissingScalarOIDs(t *testing.T) {
	tests := map[string]struct {
		missingPDU gosnmp.SnmpPDU
	}{
		"no such object cached after first licensing poll": {
			missingPDU: createNoSuchObjectPDU("1.3.6.1.4.1.99999.8.1.0"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456)},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.8.1.0"},
				[]gosnmp.SnmpPDU{tc.missingPDU},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 223456)},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							Metrics: []ddprofiledefinition.MetricsConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.3.0",
										Name: "sysUpTime",
									},
								},
							},
							Licensing: []ddprofiledefinition.LicensingConfig{
								{
									OriginProfileID: "_vendor-licensing.yaml",
									ID:              "missing-license-row",
									Identity: ddprofiledefinition.LicenseIdentityConfig{
										ID: ddprofiledefinition.LicenseValueConfig{Value: "missing-license-row"},
									},
									State: ddprofiledefinition.LicenseStateConfig{
										LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
											Symbol: ddprofiledefinition.SymbolConfig{
												OID:  "1.3.6.1.4.1.99999.8.1.0",
												Name: "licenseState",
											},
										},
									},
								},
							},
						},
					},
				},
				Log: logger.New(),
			})

			first, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, first, 1)
			assert.Empty(t, first[0].LicenseRows)
			assert.EqualValues(t, 2, first[0].Stats.SNMP.GetOIDs)
			assert.EqualValues(t, 1, first[0].Stats.Errors.MissingOIDs)

			second, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, second, 1)
			assert.Empty(t, second[0].LicenseRows)
			assert.EqualValues(t, 1, second[0].Stats.SNMP.GetOIDs)
			assert.EqualValues(t, 1, second[0].Stats.Errors.MissingOIDs)
		})
	}
}

func TestCollector_Collect_LicenseRowsFromTableLicensingConfig(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.99999.2",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.99999.2.1.1", "firewall"),
			createStringPDU("1.3.6.1.4.1.99999.2.2.1", "Firewall subscription"),
			createStringPDU("1.3.6.1.4.1.99999.2.3.1", "healthy"),
			createGauge32PDU("1.3.6.1.4.1.99999.2.4.1", 3600),
			createGauge32PDU("1.3.6.1.4.1.99999.2.5.1", 10),
			createGauge32PDU("1.3.6.1.4.1.99999.2.6.1", 25),
			createStringPDU("1.3.6.1.4.1.99999.2.1.2", "vpn"),
			createStringPDU("1.3.6.1.4.1.99999.2.2.2", "VPN subscription"),
			createStringPDU("1.3.6.1.4.1.99999.2.3.2", "warning"),
			createGauge32PDU("1.3.6.1.4.1.99999.2.4.2", 7200),
			createGauge32PDU("1.3.6.1.4.1.99999.2.5.2", 50),
			createGauge32PDU("1.3.6.1.4.1.99999.2.6.2", 100),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "vendor-device.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Licensing: []ddprofiledefinition.LicensingConfig{
				{
					OriginProfileID: "_vendor-licensing.yaml",
					ID:              "table-license",
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.2",
						Name: "licenseTable",
					},
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{
							Index: 1,
						},
						Name: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.2.2",
								Name: "licenseName",
							},
						},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.2.3",
								Name: "licenseState",
								Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
									"healthy": "0",
									"warning": "1",
								}),
							},
						},
						Policy: ddprofiledefinition.LicenseStatePolicyDefault,
					},
					Signals: ddprofiledefinition.LicenseSignalsConfig{
						Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{
							Remaining: ddprofiledefinition.LicenseValueConfig{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.99999.2.4",
									Name: "licenseExpiryRemaining",
								},
								Sentinel: []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerZeroOrNegative},
							},
						},
						Usage: ddprofiledefinition.LicenseUsageSignalsConfig{
							Used: ddprofiledefinition.LicenseValueConfig{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.99999.2.5",
									Name: "licenseUsed",
								},
							},
							Capacity: ddprofiledefinition.LicenseValueConfig{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.99999.2.6",
									Name: "licenseCapacity",
								},
							},
						},
					},
					StaticTags: []ddprofiledefinition.StaticMetricTagConfig{
						{Tag: "license_vendor", Value: "test"},
					},
					MetricTags: ddprofiledefinition.MetricTagConfigList{
						{
							Tag:   "license_feature",
							Index: 1,
						},
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Empty(t, pm.HiddenMetrics)
	require.Empty(t, pm.Metrics)
	require.Empty(t, pm.TopologyMetrics)
	require.Len(t, pm.LicenseRows, 2)

	rowsByID := make(map[string]ddsnmp.LicenseRow, len(pm.LicenseRows))
	for _, row := range pm.LicenseRows {
		if _, ok := rowsByID[row.ID]; ok {
			t.Fatalf("duplicate license row id %q", row.ID)
		}
		rowsByID[row.ID] = row
	}

	firewall := rowsByID["1"]
	assert.Equal(t, "licenseTable", firewall.Table)
	assert.Equal(t, "1.3.6.1.4.1.99999.2", firewall.TableOID)
	assert.Equal(t, "1", firewall.RowKey)
	assert.Equal(t, "_vendor-licensing.yaml|table|1.3.6.1.4.1.99999.2|1", firewall.StructuralID)
	assert.Equal(t, "Firewall subscription", firewall.Name)
	assert.True(t, firewall.State.Has)
	assert.EqualValues(t, 0, firewall.State.Severity)
	assert.Equal(t, "healthy", firewall.State.Raw)
	assert.True(t, firewall.Expiry.Has)
	assert.EqualValues(t, 3600, firewall.Expiry.RemainingSeconds)
	assert.True(t, firewall.Usage.HasUsed)
	assert.EqualValues(t, 10, firewall.Usage.Used)
	assert.True(t, firewall.Usage.HasCapacity)
	assert.EqualValues(t, 25, firewall.Usage.Capacity)
	assert.Equal(t, map[string]string{
		"license_vendor":  "test",
		"license_feature": "1",
	}, firewall.Tags)

	vpn := rowsByID["2"]
	assert.Equal(t, "VPN subscription", vpn.Name)
	assert.EqualValues(t, 1, vpn.State.Severity)
	assert.Equal(t, "warning", vpn.State.Raw)
	assert.EqualValues(t, 7200, vpn.Expiry.RemainingSeconds)
	assert.EqualValues(t, 50, vpn.Usage.Used)
	assert.EqualValues(t, 100, vpn.Usage.Capacity)
}

func TestCollector_Collect_LicenseRowsFromTableLicensingConfig_ResolvesCrossTableTags(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.99999.6",
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.99999.6.1.1", 0),
			createIntegerPDU("1.3.6.1.4.1.99999.6.1.2", 1),
		},
	)
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.2.1.31.1.1.1.1",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "eth1"),
			createStringPDU("1.3.6.1.2.1.31.1.1.1.1.2", "eth2"),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "vendor-device.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Licensing: []ddprofiledefinition.LicensingConfig{
				{
					OriginProfileID: "_vendor-licensing.yaml",
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.6",
						Name: "licenseIfTable",
					},
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{Index: 1},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.6.1",
								Name: "licenseState",
							},
						},
						Policy: ddprofiledefinition.LicenseStatePolicyDefault,
					},
					MetricTags: ddprofiledefinition.MetricTagConfigList{
						{
							Tag:   "if_name",
							Table: "ifXTable",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.31.1.1.1.1",
								Name: "ifName",
							},
						},
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].LicenseRows, 2)
	assert.EqualValues(t, 2, results[0].Stats.SNMP.TablesWalked)

	rowsByID := make(map[string]ddsnmp.LicenseRow, len(results[0].LicenseRows))
	for _, row := range results[0].LicenseRows {
		rowsByID[row.ID] = row
	}
	assert.Equal(t, map[string]string{"if_name": "eth1"}, rowsByID["1"].Tags)
	assert.Equal(t, map[string]string{"if_name": "eth2"}, rowsByID["2"].Tags)
}

func TestCollector_Collect_LicenseRowsFromTableLicensingConfig_UsesTableCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.99999.3",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.99999.3.1.7", "security"),
			createStringPDU("1.3.6.1.4.1.99999.3.2.7", "Security subscription"),
			createIntegerPDU("1.3.6.1.4.1.99999.3.3.7", 0),
		},
	)
	expectSNMPGet(mockHandler,
		[]string{
			"1.3.6.1.4.1.99999.3.1.7",
			"1.3.6.1.4.1.99999.3.2.7",
			"1.3.6.1.4.1.99999.3.3.7",
		},
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.99999.3.1.7", "security"),
			createStringPDU("1.3.6.1.4.1.99999.3.2.7", "Security subscription"),
			createIntegerPDU("1.3.6.1.4.1.99999.3.3.7", 1),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "vendor-device.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Licensing: []ddprofiledefinition.LicensingConfig{
				{
					OriginProfileID: "_vendor-licensing.yaml",
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.3",
						Name: "cachedLicenseTable",
					},
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.3.1",
								Name: "licenseID",
							},
						},
						Name: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.3.2",
								Name: "licenseName",
							},
						},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.99999.3.3",
								Name: "licenseState",
							},
						},
						Policy: ddprofiledefinition.LicenseStatePolicyDefault,
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	first, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, first[0].LicenseRows, 1)
	assert.EqualValues(t, 0, first[0].LicenseRows[0].State.Severity)
	assert.EqualValues(t, 1, first[0].Stats.TableCache.Misses)
	assert.EqualValues(t, 0, first[0].Stats.TableCache.Hits)

	second, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Len(t, second[0].LicenseRows, 1)
	assert.EqualValues(t, 1, second[0].LicenseRows[0].State.Severity)
	assert.EqualValues(t, 0, second[0].Stats.TableCache.Misses)
	assert.EqualValues(t, 1, second[0].Stats.TableCache.Hits)
	assert.EqualValues(t, 1, second[0].Stats.SNMP.TablesCached)
}

func TestCollector_Collect_LicenseRowsRejectsSentinelValues(t *testing.T) {
	tests := map[string]struct {
		value   int
		signals func(ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig
	}{
		"expiry timestamp zero": {
			value: 0,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Timestamp: value},
				}
			},
		},
		"expiry timestamp u32 max": {
			value: 4294967295,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				value.Sentinel = []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerU32Max}
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Timestamp: value},
				}
			},
		},
		"expiry timestamp pre 1971": {
			value: 1,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				value.Sentinel = []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerPre1971}
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Timestamp: value},
				}
			},
		},
		"expiry remaining zero": {
			value: 0,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Remaining: value},
				}
			},
		},
		"expiry remaining u32 max": {
			value: 4294967295,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				value.Sentinel = []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerU32Max}
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Remaining: value},
				}
			},
		},
		"expiry remaining pre 1971": {
			value: 1,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				value.Sentinel = []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerPre1971}
				return ddprofiledefinition.LicenseSignalsConfig{
					Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Remaining: value},
				}
			},
		},
		"usage used pre 1971": {
			value: 1,
			signals: func(value ddprofiledefinition.LicenseValueConfig) ddprofiledefinition.LicenseSignalsConfig {
				value.Sentinel = []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerPre1971}
				return ddprofiledefinition.LicenseSignalsConfig{
					Usage: ddprofiledefinition.LicenseUsageSignalsConfig{Used: value},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.4.1.0"},
				[]gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.99999.4.1.0", uint(tc.value)),
				},
			)

			value := ddprofiledefinition.LicenseValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.4.1.0",
					Name: "sentinelValue",
				},
				Sentinel: []ddprofiledefinition.LicenseSentinelPolicy{ddprofiledefinition.LicenseSentinelTimerZeroOrNegative},
			}
			profile := &ddsnmp.Profile{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Licensing: []ddprofiledefinition.LicensingConfig{
						{
							OriginProfileID: "_vendor-licensing.yaml",
							ID:              "sentinel-row",
							Identity: ddprofiledefinition.LicenseIdentityConfig{
								ID: ddprofiledefinition.LicenseValueConfig{Value: "sentinel-row"},
							},
							Signals: tc.signals(value),
						},
					},
				},
			}

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Empty(t, results[0].LicenseRows)
			assert.Zero(t, results[0].Stats.Metrics.Licensing)
		})
	}
}
