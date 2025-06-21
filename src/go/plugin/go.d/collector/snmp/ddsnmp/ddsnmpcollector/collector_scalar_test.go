// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestScalarCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profile        *ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []ddsnmp.Metric
		expectedError  bool
		errorContains  string
	}{
		"successful collection with scalar metrics only": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.2.1.1.3.0", "sysUpTime"),
				createScalarMetric("1.3.6.1.2.1.1.5.0", "sysName"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0"}, []gosnmp.SnmpPDU{
					createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
					createStringPDU("1.3.6.1.2.1.1.5.0", "test-host"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "sysUpTime",
					Value:      123456,
					MetricType: "gauge",
				},
				// sysName will be skipped because it can't be converted to int64
			},
			expectedError: false,
		},
		"metric with scale factor": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:         "1.3.6.1.4.1.12124.1.1.2",
								Name:        "memoryKilobytes",
								ScaleFactor: 1000, // Convert KB to bytes
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.2"}, []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.12124.1.1.2", 1024),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "memoryKilobytes",
					Value:      1024000, // 1024 * 1000
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},
		"metric with extract_value": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:                  "1.3.6.1.4.1.12124.1.1.8",
								Name:                 "temperature",
								ExtractValueCompiled: mustCompileRegex(`(\d+)C`),
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.8"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.8", "25C"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "temperature",
					Value:      25,
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},
		"OID not found - returns empty metrics": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.2.1.1.3.0", "sysUpTime"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.1.3.0"}, []gosnmp.SnmpPDU{
					createNoSuchObjectPDU("1.3.6.1.2.1.1.3.0"),
				})
			},
			expectedResult: []ddsnmp.Metric{},
			expectedError:  false,
		},
		"empty profile - no metrics defined": {
			profile: createTestProfile("empty-profile.yaml", []ddprofiledefinition.MetricsConfig{}),
			setupMock: func(m *snmpmock.MockHandler) {
				// No SNMP calls expected
			},
			expectedResult: nil,
			expectedError:  false,
		},
		"SNMP error": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.2.1.1.3.0", "sysUpTime"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGetError(m, []string{"1.3.6.1.2.1.1.3.0"}, errors.New("SNMP timeout"))
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "SNMP timeout",
		},
		"opaque float metric": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.4.1.9.9.305.1.1.1.0", "cpmCPULoadAvg1min"),
				createScalarMetric("1.3.6.1.4.1.9.9.305.1.1.2.0", "cpmCPULoadAvg5min"),
				createScalarMetric("1.3.6.1.4.1.9.9.305.1.1.3.0", "cpmCPULoadAvg15min"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{
					"1.3.6.1.4.1.9.9.305.1.1.1.0",
					"1.3.6.1.4.1.9.9.305.1.1.2.0",
					"1.3.6.1.4.1.9.9.305.1.1.3.0",
				}, []gosnmp.SnmpPDU{
					createPDU("1.3.6.1.4.1.9.9.305.1.1.1.0", gosnmp.OpaqueFloat, float32(0.75)),
					createPDU("1.3.6.1.4.1.9.9.305.1.1.2.0", gosnmp.OpaqueFloat, float32(1.23)),
					createPDU("1.3.6.1.4.1.9.9.305.1.1.3.0", gosnmp.OpaqueFloat, float32(2.45)),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cpmCPULoadAvg1min",
					Value:      0, // 0.75 truncated to int64
					MetricType: "gauge",
				},
				{
					Name:       "cpmCPULoadAvg5min",
					Value:      1, // 1.23 truncated to int64
					MetricType: "gauge",
				},
				{
					Name:       "cpmCPULoadAvg15min",
					Value:      2, // 2.45 truncated to int64
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},
		"opaque double metric": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1", "cpqHeSysBatteryVoltage"),
				createScalarMetric("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2", "cpqHeSysBatteryCurrent"),
				createScalarMetric("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3", "cpqHeSysBatteryCapacity"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1",
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2",
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3",
				}, []gosnmp.SnmpPDU{
					createPDU("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1", gosnmp.OpaqueDouble, 12.6),
					createPDU("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2", gosnmp.OpaqueDouble, 2.4),
					createPDU("1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3", gosnmp.OpaqueDouble, 98.5),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cpqHeSysBatteryVoltage",
					Value:      12, // 12.6 truncated to int64
					MetricType: "gauge",
				},
				{
					Name:       "cpqHeSysBatteryCurrent",
					Value:      2, // 2.4 truncated to int64
					MetricType: "gauge",
				},
				{
					Name:       "cpqHeSysBatteryCapacity",
					Value:      98, // 98.5 truncated to int64
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},
		"multiple scalar metrics with different types": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.1.3.0",
								Name: "sysUpTime",
							},
						},
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:        "1.3.6.1.2.1.1.7.0",
								Name:       "sysServices",
								MetricType: "gauge",
							},
						},
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:        "1.3.6.1.2.1.2.1.0",
								Name:       "ifNumber",
								MetricType: "gauge",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{
					"1.3.6.1.2.1.1.3.0",
					"1.3.6.1.2.1.1.7.0",
					"1.3.6.1.2.1.2.1.0",
				}, []gosnmp.SnmpPDU{
					createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
					createIntegerPDU("1.3.6.1.2.1.1.7.0", 72),
					createIntegerPDU("1.3.6.1.2.1.2.1.0", 4),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "sysUpTime",
					Value:      123456,
					MetricType: "gauge",
				},
				{
					Name:       "sysServices",
					Value:      72,
					MetricType: "gauge",
				},
				{
					Name:       "ifNumber",
					Value:      4,
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},
		"scalar metrics with static tags": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.1.3.0",
								Name: "sysUpTime",
							},
							StaticTags: []string{
								"source:system",
								"type:uptime",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.1.3.0"}, []gosnmp.SnmpPDU{
					createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "sysUpTime",
					Value: 123456,
					StaticTags: map[string]string{
						"source": "system",
						"type":   "uptime",
					},
					MetricType: "gauge",
				},
			},
			expectedError: false,
		},

		"metric with string to int mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12124.1.1.2",
								Name: "clusterHealth",
								Mapping: map[string]string{
									"OK":       "0",
									"WARNING":  "1",
									"CRITICAL": "2",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.2"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.2", "WARNING"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "clusterHealth",
					Value:      1,
					MetricType: "gauge",
					Mappings: map[int64]string{
						0: "OK",
						1: "WARNING",
						2: "CRITICAL",
					},
				},
			},
			expectedError: false,
		},
		"metric with int to string mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2.1.8",
								Name: "ifOperStatus",
								Mapping: map[string]string{
									"1": "up",
									"2": "down",
									"3": "testing",
									"4": "unknown",
									"5": "dormant",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.2.2.1.8"}, []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8", 2), // down
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifOperStatus",
					Value:      2,
					MetricType: "gauge",
					Mappings: map[int64]string{
						1: "up",
						2: "down",
						3: "testing",
						4: "unknown",
						5: "dormant",
					},
				},
			},
			expectedError: false,
		},
		"metric with extract_value and int to string mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:                  "1.3.6.1.4.1.12124.1.1.8",
								Name:                 "fanStatus",
								ExtractValueCompiled: mustCompileRegex(`Fan(\d+)`),
								Mapping: map[string]string{
									"1": "normal",
									"2": "warning",
									"3": "critical",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.8"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.8", "Fan2"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "fanStatus",
					Value:      2,
					MetricType: "gauge",
					Mappings: map[int64]string{
						1: "normal",
						2: "warning",
						3: "critical",
					},
				},
			},
			expectedError: false,
		},
		"metric with int to int mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2.1.7",
								Name: "ifAdminStatus",
								Mapping: map[string]string{
									"1": "1", // up -> 1
									"2": "0", // down -> 0
									"3": "0", // testing -> 0
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.2.2.1.7"}, []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.7", 2), // down
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifAdminStatus",
					Value:      0, // mapped from 2 -> 0
					MetricType: "gauge",
					Mappings: map[int64]string{
						1: "1",
						2: "0",
						3: "0",
					},
				},
			},
			expectedError: false,
		},
		"metric with partial string to int mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12124.1.1.2",
								Name: "clusterHealth",
								Mapping: map[string]string{
									"OK":      "0",
									"WARNING": "1",
									// CRITICAL is not mapped
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.2"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.2", "CRITICAL"),
				})
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "strconv.ParseInt",
		},
		"metric with mixed mapping values": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12124.1.1.2",
								Name: "deviceStatus",
								Mapping: map[string]string{
									"OK":      "0",
									"WARNING": "1",
									"ERROR":   "invalid", // This will cause metric to be skipped
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.12124.1.1.2"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.2", "ERROR"),
				})
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "strconv.ParseInt",
		},
		"metric with no mapping": {
			profile: createTestProfile("test-profile.yaml", []ddprofiledefinition.MetricsConfig{
				createScalarMetric("1.3.6.1.2.1.1.3.0", "sysUpTime"),
			}),
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.1.3.0"}, []gosnmp.SnmpPDU{
					createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "sysUpTime",
					Value:      123456,
					MetricType: "gauge",
					Mappings:   nil, // No mappings
				},
			},
			expectedError: false,
		},
		"metric with numeric value and int to string mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2.1.7",
								Name: "ifAdminStatus",
								Mapping: map[string]string{
									"1": "up",
									"2": "down",
									"3": "testing",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.2.1.2.2.1.7"}, []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.7", 1), // up
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifAdminStatus",
					Value:      1,
					MetricType: "gauge",
					Mappings: map[int64]string{
						1: "up",
						2: "down",
						3: "testing",
					},
				},
			},
			expectedError: false,
		},
		"metric with string value and string to int mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.318.1.1.1.2.2.1.0",
								Name: "upsBasicBatteryStatus",
								Mapping: map[string]string{
									"batteryNormal":   "0",
									"batteryLow":      "1",
									"batteryDepleted": "2",
									"batteryCharging": "3",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{"1.3.6.1.4.1.318.1.1.1.2.2.1.0"}, []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.318.1.1.1.2.2.1.0", "batteryLow"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "upsBasicBatteryStatus",
					Value:      1,
					MetricType: "gauge",
					Mappings: map[int64]string{
						0: "batteryNormal",
						1: "batteryLow",
						2: "batteryDepleted",
						3: "batteryCharging",
					},
				},
			},
			expectedError: false,
		},
		"partial success with missing OIDs": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						createScalarMetric("1.3.6.1.2.1.1.3.0", "sysUpTime"),
						createScalarMetric("1.3.6.1.2.1.1.5.0", "sysName"),
						createScalarMetric("1.3.6.1.2.1.1.6.0", "sysLocation"),
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPGet(m, []string{
					"1.3.6.1.2.1.1.3.0",
					"1.3.6.1.2.1.1.5.0",
					"1.3.6.1.2.1.1.6.0",
				}, []gosnmp.SnmpPDU{
					createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
					createNoSuchObjectPDU("1.3.6.1.2.1.1.5.0"),
					createStringPDU("1.3.6.1.2.1.1.6.0", "DataCenter"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "sysUpTime",
					Value:      123456,
					MetricType: "gauge",
				},
				// sysName is missing (NoSuchObject)
				// sysLocation is string and can't be converted to int64
			},
			expectedError: false,
		},
		"chunked requests with many metrics": {
			profile: func() *ddsnmp.Profile {
				// Create a profile with many metrics to force chunking
				var metrics []ddprofiledefinition.MetricsConfig
				for i := 0; i < 25; i++ {
					metrics = append(metrics, createScalarMetric(
						fmt.Sprintf("1.3.6.1.2.1.1.%02d.0", i),
						fmt.Sprintf("metric%d", i),
					))
				}
				return &ddsnmp.Profile{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: metrics,
					},
				}
			}(),
			setupMock: func(m *snmpmock.MockHandler) {
				// Expect 3 chunks (10 + 10 + 5)
				chunk1OIDs := make([]string, 10)
				chunk1PDUs := make([]gosnmp.SnmpPDU, 10)
				for i := 0; i < 10; i++ {
					chunk1OIDs[i] = fmt.Sprintf("1.3.6.1.2.1.1.%02d.0", i)
					chunk1PDUs[i] = createIntegerPDU(chunk1OIDs[i], i*100)
				}

				chunk2OIDs := make([]string, 10)
				chunk2PDUs := make([]gosnmp.SnmpPDU, 10)
				for i := 0; i < 10; i++ {
					chunk2OIDs[i] = fmt.Sprintf("1.3.6.1.2.1.1.%02d.0", i+10)
					chunk2PDUs[i] = createIntegerPDU(chunk2OIDs[i], (i+10)*100)
				}

				chunk3OIDs := make([]string, 5)
				chunk3PDUs := make([]gosnmp.SnmpPDU, 5)
				for i := 0; i < 5; i++ {
					chunk3OIDs[i] = fmt.Sprintf("1.3.6.1.2.1.1.%02d.0", i+20)
					chunk3PDUs[i] = createIntegerPDU(chunk3OIDs[i], (i+20)*100)
				}

				m.EXPECT().Get(chunk1OIDs).Return(&gosnmp.SnmpPacket{Variables: chunk1PDUs}, nil)
				m.EXPECT().Get(chunk2OIDs).Return(&gosnmp.SnmpPacket{Variables: chunk2PDUs}, nil)
				m.EXPECT().Get(chunk3OIDs).Return(&gosnmp.SnmpPacket{Variables: chunk3PDUs}, nil)
			},
			expectedResult: func() []ddsnmp.Metric {
				// Generate expected metrics
				var metrics []ddsnmp.Metric
				for i := 0; i < 25; i++ {
					metrics = append(metrics, ddsnmp.Metric{
						Name:       fmt.Sprintf("metric%d", i),
						Value:      int64(i * 100),
						MetricType: "gauge",
					})
				}
				return metrics
			}(),
			expectedError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			tc.setupMock(mockHandler)

			missingOIDs := make(map[string]bool)
			collector := newScalarCollector(mockHandler, missingOIDs, logger.New())

			result, err := collector.Collect(tc.profile)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tc.expectedResult != nil {
				assertMetricsEqual(t, tc.expectedResult, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}
