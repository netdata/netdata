// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profile        *ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []ddsnmp.Metric
		expectedError  bool
		errorContains  string
		checkMissing   map[string]bool // OIDs that should be marked as missing
	}{
		"basic table with single metric": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with multiple metrics": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.16",
									Name: "ifOutOctets",
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					// Row 1
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.1", 500),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.1", 1), // up
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					// Row 2
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.2", 1500),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.2", 2), // down
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				// Row 1
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      500,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOperStatus",
					Value:      1,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "gauge",
					IsTable:    true,
				},
				// Row 2
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      1500,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOperStatus",
					Value:      2,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"empty table": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{})
			},
			expectedResult: []ddsnmp.Metric{},
			expectedError:  false,
			checkMissing:   map[string]bool{"1.3.6.1.2.1.2.2": true},
		},
		"table walk error": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalkError(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", errors.New("timeout walking table"))
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "timeout walking table",
		},
		"table with snmpv1": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version1, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"multiple tables in profile": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.4.31.1",
								Name: "ipSystemStatsTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.4.31.1.1.4",
									Name: "ipSystemStatsHCInReceives",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// First table walk
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
				})
				// Second table walk
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.4.31.1", []gosnmp.SnmpPDU{
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.1", 5000),
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.2", 6000),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      5000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      6000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"partial table data with missing rows": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.16",
									Name: "ifOutOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk returns partial data - some metrics missing for some rows
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					// Row 1 - complete
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.1", 500),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					// Row 2 - missing ifOutOctets
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
					// Row 3 - missing tag but has metrics
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.3", 3000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.3", 1500),
				})
			},
			expectedResult: []ddsnmp.Metric{
				// Row 1 - complete
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      500,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				// Row 2 - only ifInOctets
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "rate",
					IsTable:    true,
				},
				// Row 3 - both metrics but no tag
				{
					Name:       "ifInOctets",
					Value:      3000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      1500,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"no symbols defined": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{}, // No symbols
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Table walk happens even without symbols (for tags)
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
				})
			},
			expectedResult: []ddsnmp.Metric{}, // No metrics because no symbols defined
			expectedError:  false,
		},
		"table with non-numeric values": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.2",
									Name: "ifDescr", // String column, should be skipped
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},

		"table with tag mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
									Mapping: map[string]string{
										"1":  "other",
										"6":  "ethernetCsmacd",
										"24": "softwareLoopback",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "lo0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 24), // softwareLoopback
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.2", 5),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.2", 6), // ethernetCsmacd
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "lo0", "if_type": "softwareLoopback"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      5,
					Tags:       map[string]string{"interface": "eth0", "if_type": "ethernetCsmacd"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with extract_value in tag": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:                  "1.3.6.1.2.1.2.2.1.2",
										Name:                 "ifDescr",
										ExtractValueCompiled: mustCompileRegex(`^(\S+)`), // Extract first word
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0 - Primary Network Interface"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.2", 5),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "lo0 - Loopback"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      5,
					Tags:       map[string]string{"interface": "lo0"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with multiple tag columns": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
								},
								{
									Tag: "admin_status",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.7",
										Name: "ifAdminStatus",
									},
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 6),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.7.1", 1), // up
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "ifInOctets",
					Value: 1000,
					Tags: map[string]string{
						"interface":    "eth0",
						"if_type":      "6",
						"admin_status": "up",
					},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with static tags": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							StaticTags: []string{
								"source:interface",
								"table:if",
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "ifInOctets",
					Value: 1000,
					StaticTags: map[string]string{
						"source": "interface",
						"table":  "if",
					},
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with pattern matching tag": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
									Pattern: mustCompileRegex(`(\w+)(\d+)/(\d+)`),
									Tags: map[string]string{
										"interface_type": "$1",
										"slot":           "$2",
										"port":           "$3",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "GigabitEthernet1/0"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "FastEthernet2/1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "ifInOctets",
					Value: 1000,
					Tags: map[string]string{
						"interface_type": "GigabitEthernet",
						"slot":           "1",
						"port":           "0",
					},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:  "ifInOctets",
					Value: 2000,
					Tags: map[string]string{
						"interface_type": "FastEthernet",
						"slot":           "2",
						"port":           "1",
					},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with format mac_address in tag": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.29671.1.1.4",
								Name: "devTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.29671.1.1.4.1.5",
									Name: "devClientCount",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:    "1.3.6.1.4.1.29671.1.1.4.1.1",
										Name:   "devMac",
										Format: "mac_address",
									},
									Tag: "mac_address",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.29671.1.1.4", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.4.1.29671.1.1.4.1.5.1", 10),
					createPDU("1.3.6.1.4.1.29671.1.1.4.1.1.1", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xAB, 0xCD, 0xEF}),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "devClientCount",
					Value:      10,
					Tags:       map[string]string{"mac_address": "00:50:56:AB:CD:EF"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with format ip_address in tag": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.4.20",
								Name: "ipAddrTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.4.20.1.3",
									Name: "ipAdEntBcastAddr",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:    "1.3.6.1.2.1.4.20.1.1",
										Name:   "ipAdEntAddr",
										Format: "ip_address",
									},
									Tag: "ip_address",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.4.20", []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.4.20.1.3.1", 1),
					createPDU("1.3.6.1.2.1.4.20.1.1.1", gosnmp.IPAddress, "192.168.1.1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ipAdEntBcastAddr",
					Value:      1,
					Tags:       map[string]string{"ip_address": "192.168.1.1"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with mixed tag types": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							StaticTags: []string{
								"source:network",
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:                  "1.3.6.1.2.1.2.2.1.2",
										Name:                 "ifDescr",
										ExtractValueCompiled: mustCompileRegex(`^(\S+)`),
									},
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
									Mapping: map[string]string{
										"6": "ethernet",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0 - Primary Interface"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 6),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					StaticTags: map[string]string{"source": "network"},
					Tags:       map[string]string{"interface": "eth0", "if_type": "ethernet"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with tag but no mapping match": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
									Mapping: map[string]string{
										"6": "ethernet",
										// No mapping for value 131
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "tunnel0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 131), // No mapping for this value
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "tunnel0", "if_type": "131"}, // Raw value when no mapping
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with empty tag value": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", ""), // Empty string
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": ""}, // Empty tag value
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},

		"table with missing rows": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.16",
									Name: "ifOutOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					// Row 1 - complete
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.1", 500),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					// Row 2 - missing ifOutOctets
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
					// Row 3 - missing tag (ifDescr)
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.3", 3000),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.16.3", 1500),
				})
			},
			expectedResult: []ddsnmp.Metric{
				// Row 1 - complete
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      500,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "rate",
					IsTable:    true,
				},
				// Row 2 - only ifInOctets
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "rate",
					IsTable:    true,
				},
				// Row 3 - both metrics but no tag
				{
					Name:       "ifInOctets",
					Value:      3000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifOutOctets",
					Value:      1500,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with scale factor": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.109.1.1.1",
								Name: "cpmCPUTotalTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:         "1.3.6.1.4.1.9.9.109.1.1.1.1.12",
									Name:        "cpmCPUMemoryUsed",
									ScaleFactor: 1024, // Convert KB to bytes
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "cpu_id",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.9.9.109.1.1.1.1.2",
										Name: "cpmCPUTotalPhysicalIndex",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.9.9.109.1.1.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.9.9.109.1.1.1.1.12.1", 2048), // 2048 KB
					createIntegerPDU("1.3.6.1.4.1.9.9.109.1.1.1.1.2.1", 1),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cpmCPUMemoryUsed",
					Value:      2097152, // 2048 * 1024
					Tags:       map[string]string{"cpu_id": "1"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with value mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
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
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.1", 1), // up
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.2", 2), // down
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifOperStatus",
					Value:      1,
					Tags:       map[string]string{"interface": "eth0"},
					MetricType: "gauge",
					IsTable:    true,
					Mappings: map[int64]string{
						1: "up",
						2: "down",
						3: "testing",
						4: "unknown",
						5: "dormant",
					},
				},
				{
					Name:       "ifOperStatus",
					Value:      2,
					Tags:       map[string]string{"interface": "eth1"},
					MetricType: "gauge",
					IsTable:    true,
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
		"table with extract value and mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12124.1.13",
								Name: "fanTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:                  "1.3.6.1.4.1.12124.1.13.1.3",
									Name:                 "fanStatus",
									ExtractValueCompiled: mustCompileRegex(`Status(\d+)`),
									Mapping: map[string]string{
										"1": "normal",
										"2": "warning",
										"3": "critical",
									},
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "fan_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.12124.1.13.1.2",
										Name: "fanName",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12124.1.13", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.13.1.3.1", "Status1"), // normal
					createStringPDU("1.3.6.1.4.1.12124.1.13.1.2.1", "Fan1"),
					createStringPDU("1.3.6.1.4.1.12124.1.13.1.3.2", "Status3"), // critical
					createStringPDU("1.3.6.1.4.1.12124.1.13.1.2.2", "Fan2"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "fanStatus",
					Value:      1,
					Tags:       map[string]string{"fan_name": "Fan1"},
					MetricType: "gauge",
					IsTable:    true,
					Mappings: map[int64]string{
						1: "normal",
						2: "warning",
						3: "critical",
					},
				},
				{
					Name:       "fanStatus",
					Value:      3,
					Tags:       map[string]string{"fan_name": "Fan2"},
					MetricType: "gauge",
					IsTable:    true,
					Mappings: map[int64]string{
						1: "normal",
						2: "warning",
						3: "critical",
					},
				},
			},
			expectedError: false,
		},
		"table with multiple scale factors": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.305.1.1.1",
								Name: "cempMemPoolTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:         "1.3.6.1.4.1.9.9.305.1.1.1.1.5",
									Name:        "cempMemPoolUsed",
									ScaleFactor: 1000, // Convert to larger unit
								},
								{
									OID:         "1.3.6.1.4.1.9.9.305.1.1.1.1.6",
									Name:        "cempMemPoolFree",
									ScaleFactor: 0.001, // Convert to smaller unit
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "pool_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.9.9.305.1.1.1.1.2",
										Name: "cempMemPoolName",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.9.9.305.1.1.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.9.9.305.1.1.1.1.5.1", 1024),
					createGauge32PDU("1.3.6.1.4.1.9.9.305.1.1.1.1.6.1", 2048000),
					createStringPDU("1.3.6.1.4.1.9.9.305.1.1.1.1.2.1", "Processor"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cempMemPoolUsed",
					Value:      1024000, // 1024 * 1000
					Tags:       map[string]string{"pool_name": "Processor"},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "cempMemPoolFree",
					Value:      2048, // 2048000 * 0.001
					Tags:       map[string]string{"pool_name": "Processor"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with missing value in mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.318.1.1.10.4.3.3",
								Name: "upsPhaseInputTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.318.1.1.10.4.3.3.1.4",
									Name: "upsPhaseInputCurrent",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "phase",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.318.1.1.10.4.3.3.1.1",
										Name: "upsPhaseInputPhaseIndex",
									},
									Mapping: map[string]string{
										"1": "L1",
										"2": "L2",
										"3": "L3",
										// No mapping for value 4
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.318.1.1.10.4.3.3", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.318.1.1.10.4.3.3.1.4.1", 150),
					createIntegerPDU("1.3.6.1.4.1.318.1.1.10.4.3.3.1.1.1", 1), // L1
					createGauge32PDU("1.3.6.1.4.1.318.1.1.10.4.3.3.1.4.2", 160),
					createIntegerPDU("1.3.6.1.4.1.318.1.1.10.4.3.3.1.1.2", 4), // No mapping
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "upsPhaseInputCurrent",
					Value:      150,
					Tags:       map[string]string{"phase": "L1"},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "upsPhaseInputCurrent",
					Value:      160,
					Tags:       map[string]string{"phase": "4"}, // Raw value when no mapping
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with string to int mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12124.1.1",
								Name: "nodeTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12124.1.1.1.5",
									Name: "nodeHealth",
									Mapping: map[string]string{
										"OK":      "0",
										"ATTN":    "1",
										"DOWN":    "2",
										"INVALID": "3",
									},
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "node_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.12124.1.1.1.2",
										Name: "nodeName",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12124.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12124.1.1.1.5.1", "OK"),
					createStringPDU("1.3.6.1.4.1.12124.1.1.1.2.1", "node1"),
					createStringPDU("1.3.6.1.4.1.12124.1.1.1.5.2", "DOWN"),
					createStringPDU("1.3.6.1.4.1.12124.1.1.1.2.2", "node2"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "nodeHealth",
					Value:      0,
					Tags:       map[string]string{"node_name": "node1"},
					MetricType: "gauge",
					IsTable:    true,
					Mappings: map[int64]string{
						0: "OK",
						1: "ATTN",
						2: "DOWN",
						3: "INVALID",
					},
				},
				{
					Name:       "nodeHealth",
					Value:      2,
					Tags:       map[string]string{"node_name": "node2"},
					MetricType: "gauge",
					IsTable:    true,
					Mappings: map[int64]string{
						0: "OK",
						1: "ATTN",
						2: "DOWN",
						3: "INVALID",
					},
				},
			},
			expectedError: false,
		},
		"table with OpaqueFloat values": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.305.1.1.2",
								Name: "cpmCPULoadTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.2.1.1",
									Name: "cpmCPULoadAvg1min",
								},
								{
									OID:         "1.3.6.1.4.1.9.9.305.1.1.2.1.2",
									Name:        "cpmCPULoadAvg5min",
									ScaleFactor: 100, // Convert to percentage
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "cpu_index",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.9.9.305.1.1.2.1.0",
										Name: "cpmCPULoadIndex",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.9.9.305.1.1.2", []gosnmp.SnmpPDU{
					createPDU("1.3.6.1.4.1.9.9.305.1.1.2.1.1.1", gosnmp.OpaqueFloat, float32(0.75)),
					createPDU("1.3.6.1.4.1.9.9.305.1.1.2.1.2.1", gosnmp.OpaqueFloat, float32(0.82)),
					createIntegerPDU("1.3.6.1.4.1.9.9.305.1.1.2.1.0.1", 1),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cpmCPULoadAvg1min",
					Value:      0, // 0.75 truncated to int64
					Tags:       map[string]string{"cpu_index": "1"},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "cpmCPULoadAvg5min",
					Value:      81, // 0.82 * 100 = 82
					Tags:       map[string]string{"cpu_index": "1"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table with non-convertible string values": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
								{
									OID:  "1.3.6.1.2.1.2.2.1.2",
									Name: "ifDescr", // This is a string, should be skipped
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"), // Can't convert to int64
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       nil,
					MetricType: "rate",
					IsTable:    true,
				},
				// ifDescr metric is skipped because string can't be converted to int64
			},
			expectedError: false,
		},

		"cross-table tag from ifXTable": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1",
								Name: "ifXTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.2", 20),
				})
				// Walk ifXTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0"),
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.2", "GigabitEthernet0/1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "GigabitEthernet0/0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      20,
					Tags:       map[string]string{"interface": "GigabitEthernet0/1"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag with missing row in referenced table": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1",
								Name: "ifXTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable - has 3 interfaces
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.2", 20),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.3", 30),
				})
				// Walk ifXTable - only has 2 interfaces (missing index 3)
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0"),
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.2", "GigabitEthernet0/1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "GigabitEthernet0/0"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      20,
					Tags:       map[string]string{"interface": "GigabitEthernet0/1"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      30,
					Tags:       nil, // No cross-table tag found for index 3
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"multiple cross-table tags": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface_desc",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
									// No Table field = same table
								},
								{
									Tag: "interface_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
								{
									Tag: "interface_alias",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.18",
										Name: "ifAlias",
									},
									Table: "ifXTable",
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1",
								Name: "ifXTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.18",
									Name: "ifAlias",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
				})
				// Walk ifXTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0"),
					createStringPDU("1.3.6.1.2.1.31.1.1.1.18.1", "Uplink to Core"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "ifInErrors",
					Value: 10,
					Tags: map[string]string{
						"interface_desc":  "eth0",
						"interface_name":  "GigabitEthernet0/0",
						"interface_alias": "Uplink to Core",
					},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag with extract_value": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface_clean",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:                  "1.3.6.1.2.1.31.1.1.1.1",
										Name:                 "ifName",
										ExtractValueCompiled: mustCompileRegex("^([a-zA-Z0-9]+)"),
									},
									Table: "ifXTable",
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1",
								Name: "ifXTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
				})
				// Walk ifXTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0.100"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface_clean": "GigabitEthernet0"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag with mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.276.1.1.2",
								Name: "cieIfInterfaceTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.9.9.276.1.1.2.1.1",
									Name: "cieIfResetCount",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
									Table: "ifTable",
									Mapping: map[string]string{
										"6":  "ethernet",
										"24": "loopback",
									},
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1",
								Name: "ifXTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.3",
									Name: "ifType",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk cieIfInterfaceTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.9.9.276.1.1.2", []gosnmp.SnmpPDU{
					createCounter64PDU("1.3.6.1.4.1.9.9.276.1.1.2.1.1.1", 5),
				})
				// Walk ifXTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0"),
				})
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 6),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "cieIfResetCount",
					Value: 5,
					Tags: map[string]string{
						"interface": "GigabitEthernet0/0",
						"if_type":   "ethernet",
					},
					MetricType: ddprofiledefinition.ProfileMetricTypeRate,
					IsTable:    true,
				},
				{
					Name:       "ifType",
					MetricType: "gauge",
					IsTable:    true,
					Tags:       nil,
					Value:      6,
				},
			},
			expectedError: false,
		},
		"cross-table tag with index transformation": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.30932.1.10.1.3.110",
								Name: "cpiPduBranchTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.30932.1.10.1.3.110.1.3",
									Name: "cpiPduBranchCurrent",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "pdu_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3",
										Name: "cpiPduName",
									},
									Table: "cpiPduTable",
									IndexTransform: []ddprofiledefinition.MetricIndexTransform{
										{
											Start: 1,
											End:   7,
										},
									},
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.30932.1.10.1.2.10",
								Name: "cpiPduTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3",
									Name: "cpiPduName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk cpiPduBranchTable with complex index: 1.6.0.36.155.53.3.246
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.30932.1.10.1.3.110", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.30932.1.10.1.3.110.1.3.1.6.0.36.155.53.3.246", 15),
				})
				// Walk cpiPduTable with simpler index: 6.0.36.155.53.3.246
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.30932.1.10.1.2.10", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.30932.1.10.1.2.10.1.3.6.0.36.155.53.3.246", "PDU-A1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cpiPduBranchCurrent",
					Value:      15,
					Tags:       map[string]string{"pdu_name": "PDU-A1"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag with complex index transformation": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12345.1.1",
								Name: "customTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12345.1.1.1.1",
									Name: "customMetric",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "device_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.12345.2.1.1.1",
										Name: "deviceName",
									},
									Table: "deviceTable",
									IndexTransform: []ddprofiledefinition.MetricIndexTransform{
										{
											Start: 1,
											End:   2,
										},
										{
											Start: 4,
											End:   6,
										},
									},
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12345.2.1",
								Name: "deviceTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12345.2.1.1.1",
									Name: "deviceName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk customTable with index: 1.2.3.4.5.6.7
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12345.1.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.12345.1.1.1.1.1.2.3.4.5.6.7", 100),
				})
				// Walk deviceTable with transformed index: 2.3.5.6.7
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12345.2.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12345.2.1.1.1.2.3.5.6.7", "Device-123"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "customMetric",
					Value:      100,
					Tags:       map[string]string{"device_name": "Device-123"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag error when index transform fails": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12345.1.1",
								Name: "customTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12345.1.1.1.1",
									Name: "customMetric",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "device_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.12345.2.1.1.1",
										Name: "deviceName",
									},
									Table: "deviceTable",
									IndexTransform: []ddprofiledefinition.MetricIndexTransform{
										{
											Start: 10, // Out of bounds
											End:   15,
										},
									},
								},
							},
						},
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12345.2.1",
								Name: "deviceTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12345.2.1.1.1",
									Name: "deviceName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk customTable with short index: 1.2.3
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12345.1.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.12345.1.1.1.1.1.2.3", 100),
				})
				// Walk deviceTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12345.2.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.4.1.12345.2.1.1.1.1", "Device-1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "customMetric",
					Value:      100,
					Tags:       nil, // No tag because index transform failed
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table tag from table without metrics": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1",
								Name: "panEntityFRUModuleTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1",
									Name: "panEntryFRUModulePowerUsed",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "ent_descr",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.47.1.1.1.1.2",
										Name: "entPhysicalDescr",
									},
									Table: "entPhysicalTable",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk panEntityFRUModuleTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.25461.1.1.7.1.2.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 100),
					createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 150),
				})
				// Walk entPhysicalDescr column only (not the whole entPhysicalTable)
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.47.1.1.1.1.2", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.1", "Power Supply 1"),
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.2", "Power Supply 2"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "panEntryFRUModulePowerUsed",
					Value:      100,
					Tags:       map[string]string{"ent_descr": "Power Supply 1"},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "panEntryFRUModulePowerUsed",
					Value:      150,
					Tags:       map[string]string{"ent_descr": "Power Supply 2"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"cross-table multiple tags from same table without metrics": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1",
								Name: "panEntityFRUModuleTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1",
									Name: "panEntryFRUModulePowerUsed",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "ent_descr",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.47.1.1.1.1.2",
										Name: "entPhysicalDescr",
									},
									Table: "entPhysicalTable",
								},
								{
									Tag: "ent_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.47.1.1.1.1.3",
										Name: "entPhysicalVendorType",
									},
									Table: "entPhysicalTable",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk panEntityFRUModuleTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.25461.1.1.7.1.2.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 100),
					createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 150),
				})

				// With preprocessing, should walk the common prefix for entPhysicalTable
				// Common prefix of 1.3.6.1.2.1.47.1.1.1.1.2 and 1.3.6.1.2.1.47.1.1.1.1.3
				// is 1.3.6.1.2.1.47.1.1.1.1
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.47.1.1.1.1", []gosnmp.SnmpPDU{
					// entPhysicalDescr column
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.1", "Power Supply 1"),
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.2", "Power Supply 2"),
					// entPhysicalVendorType column
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.3.1", "PowerSupplyModule"),
					createStringPDU("1.3.6.1.2.1.47.1.1.1.1.3.2", "PowerSupplyModule"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "panEntryFRUModulePowerUsed",
					Value: 100,
					Tags: map[string]string{
						"ent_descr": "Power Supply 1",
						"ent_type":  "PowerSupplyModule",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:  "panEntryFRUModulePowerUsed",
					Value: 150,
					Tags: map[string]string{
						"ent_descr": "Power Supply 2",
						"ent_type":  "PowerSupplyModule",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"tag precedence - cross-table before same-table": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.14",
									Name: "ifInErrors",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
									// Same table tag
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.1", 10),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0-description"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.2", 20),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1-description"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.14.3", 30),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.3", "eth2-description"),
				})
				// Walk ifName column from ifXTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigabitEthernet0/0"),
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.2", "GigabitEthernet0/1"),
					// No entry for index 3 - missing cross-table data
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInErrors",
					Value:      10,
					Tags:       map[string]string{"interface": "GigabitEthernet0/0"}, // Uses ifName (cross-table)
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      20,
					Tags:       map[string]string{"interface": "GigabitEthernet0/1"}, // Uses ifName (cross-table)
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInErrors",
					Value:      30,
					Tags:       map[string]string{"interface": "eth2-description"}, // Falls back to ifDescr (same-table)
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"tag precedence with same name - respects profile order": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
								{
									Tag: "if_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.3",
										Name: "ifType",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					// Interface 1 - has both ifName and ifDescr
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0-description"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.1", 6),
					// Interface 2 - has only ifDescr
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1-description"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.3.2", 6),
				})
				// Walk ifName column
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigE0/0"),
					// No entry for index 2 - simulating missing ifName
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "ifInOctets",
					Value: 1000,
					Tags: map[string]string{
						"interface": "GigE0/0", // Uses ifName (first in order)
						"if_type":   "6",
					},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:  "ifInOctets",
					Value: 2000,
					Tags: map[string]string{
						"interface": "eth1-description", // Falls back to ifDescr
						"if_type":   "6",
					},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"tag precedence with same name - swapped order": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
									// Same table tag FIRST this time
								},
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
									// Cross-table tag SECOND
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0-description"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", ""), // Empty ifDescr
				})
				// Walk ifName column
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigE0/0"),
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.2", "GigE0/1"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface": "eth0-description"}, // Uses ifDescr (first in order)
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface": "GigE0/1"}, // Falls back to ifName when ifDescr is empty
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"tag precedence with index fallback": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.31.1.1.1.1",
										Name: "ifName",
									},
									Table: "ifXTable",
								},
								{
									Tag: "interface_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
								{
									Index: 1,
									Tag:   "interface_name",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				// Walk ifTable
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					// Interface 1 - has both ifName and ifDescr
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					// Interface 2 - has only ifDescr
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
					// Interface 3 - has empty ifDescr
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.3", 3000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.3", ""),
				})
				// Walk ifName column
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", []gosnmp.SnmpPDU{
					createStringPDU("1.3.6.1.2.1.31.1.1.1.1.1", "GigE0/0"),
					// No entry for index 2
					// No entry for index 3
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface_name": "GigE0/0"}, // Uses ifName
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface_name": "eth1"}, // Falls back to ifDescr
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInOctets",
					Value:      3000,
					Tags:       map[string]string{"interface_name": "3"}, // Falls back to index
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},

		"basic index tag": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.4.31.1",
								Name: "ipSystemStatsTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.4.31.1.1.4",
									Name: "ipSystemStatsHCInReceives",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1,
									Tag:   "ip_version",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.4.31.1", []gosnmp.SnmpPDU{
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.1", 1000), // IPv4
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.2", 2000), // IPv6
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      1000,
					Tags:       map[string]string{"ip_version": "1"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      2000,
					Tags:       map[string]string{"ip_version": "2"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"multiple index positions": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.9.9.147.1.2.2.2",
								Name: "cfwConnectionStatTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5",
									Name: "cfwConnectionStatValue",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1,
									Tag:   "service_type",
								},
								{
									Index: 2,
									Tag:   "stat_type",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.9.9.147.1.2.2.2", []gosnmp.SnmpPDU{
					createCounter64PDU("1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.20.2", 4087850099),
					createCounter64PDU("1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.21.3", 1234567890),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "cfwConnectionStatValue",
					Value:      4087850099,
					Tags:       map[string]string{"service_type": "20", "stat_type": "2"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "cfwConnectionStatValue",
					Value:      1234567890,
					Tags:       map[string]string{"service_type": "21", "stat_type": "3"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"index tag with mapping": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.4.31.1",
								Name: "ipSystemStatsTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.4.31.1.1.4",
									Name: "ipSystemStatsHCInReceives",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1,
									Tag:   "ip_version",
									Mapping: map[string]string{
										"1": "ipv4",
										"2": "ipv6",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.4.31.1", []gosnmp.SnmpPDU{
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.1", 1000),
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.2", 2000),
					createCounter64PDU("1.3.6.1.2.1.4.31.1.1.4.0", 10), // Unknown
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      1000,
					Tags:       map[string]string{"ip_version": "ipv4"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      2000,
					Tags:       map[string]string{"ip_version": "ipv6"},
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ipSystemStatsHCInReceives",
					Value:      10,
					Tags:       map[string]string{"ip_version": "0"}, // No mapping for 0
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"index tag with missing position": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1,
									Tag:   "interface_index",
								},
								{
									Index: 2,
									Tag:   "sub_interface", // Won't exist for single index
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),   // Single index
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2.3", 2000), // Multi-part index
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface_index": "1"}, // No sub_interface tag
					MetricType: "rate",
					IsTable:    true,
				},
				{
					Name:       "ifInOctets",
					Value:      2000,
					Tags:       map[string]string{"interface_index": "2", "sub_interface": "3"},
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"index tag with default tag name": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.4.24.4",
								Name: "ipMRouteTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.4.24.4.1.3",
									Name: "ipMRouteInIfIndex",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1, // No Tag field, should use "index1"
								},
								{
									Index: 2, // No Tag field, should use "index2"
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.4.24.4", []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.4.24.4.1.3.192.168.1.0.255.255.255.0", 1),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ipMRouteInIfIndex",
					Value:      1,
					Tags:       map[string]string{"index1": "192", "index2": "168"},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"complex multi-part index": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.12345.1.1",
								Name: "customTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.12345.1.1.1.1",
									Name: "customMetric",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 1,
									Tag:   "device_id",
								},
								{
									Index: 3,
									Tag:   "sensor_id",
								},
								{
									Index: 5,
									Tag:   "reading_type",
								},
								{
									Index: 10,
									Tag:   "last_octet", // Should not exist
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.12345.1.1", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.4.1.12345.1.1.1.1.10.20.30.40.50", 100),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "customMetric",
					Value: 100,
					Tags: map[string]string{
						"device_id":    "10",
						"sensor_id":    "30",
						"reading_type": "50",
						// No last_octet tag as position 10 doesn't exist
					},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"index tag with zero position": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 0, // Invalid, should be ignored
									Tag:   "invalid_tag",
								},
								{
									Index: 1,
									Tag:   "interface_index",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      1000,
					Tags:       map[string]string{"interface_index": "1"}, // No invalid_tag
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"index overflow handling": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Index: 999, // Way out of bounds
									Tag:   "impossible_tag",
								},
								{
									Index: 1,
									Tag:   "interface_index",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.5", 5000),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "ifInOctets",
					Value:      5000,
					Tags:       map[string]string{"interface_index": "5"}, // No impossible_tag
					MetricType: "rate",
					IsTable:    true,
				},
			},
			expectedError: false,
		},

		"table metric with sensor type transformation": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.14988.1.1.3.100",
								Name: "mtxrHlTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.3",
									Name: "mtxrHlSensorValue",
									Transform: `
{{- $sensorType := index .Metric.Tags "sensor_type" | default "" -}}
{{- if eq $sensorType "1" -}}
  {{- setName .Metric (printf "%s_temperature" .Metric.Name) -}}
  {{- setUnit .Metric "celsius" -}}
  {{- setFamily .Metric "Health/Temperature" -}}
{{- else if eq $sensorType "3" -}}
  {{- setName .Metric (printf "%s_voltage" .Metric.Name) -}}
  {{- setUnit .Metric "volts" -}}
  {{- setValue .Metric (int64 (div (float64 .Metric.Value) 10.0)) -}}
  {{- setFamily .Metric "Health/Power" -}}
{{- end -}}
{{- deleteTag .Metric "sensor_type" -}}`,
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "sensor_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.2",
										Name: "mtxrHlSensorName",
									},
								},
								{
									Tag: "sensor_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.4",
										Name: "mtxrHlSensorType",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.14988.1.1.3.100", []gosnmp.SnmpPDU{
					// Temperature sensor (type 1)
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.3.1", 25),
					createStringPDU("1.3.6.1.4.1.14988.1.1.3.100.1.2.1", "cpu-temperature"),
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.4.1", 1),
					// Voltage sensor (type 3)
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.3.2", 120), // 12.0V in decivolts
					createStringPDU("1.3.6.1.4.1.14988.1.1.3.100.1.2.2", "input-voltage"),
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.4.2", 3),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "mtxrHlSensorValue_temperature",
					Value:      25,
					Tags:       map[string]string{"sensor_name": "cpu-temperature"},
					Unit:       "celsius",
					Family:     "Health/Temperature",
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "mtxrHlSensorValue_voltage",
					Value:      12, // Converted from decivolts
					Tags:       map[string]string{"sensor_name": "input-voltage"},
					Unit:       "volts",
					Family:     "Health/Power",
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"mikrotik sensor table real-world example": {
			profile: &ddsnmp.Profile{
				SourceFile: "mikrotik-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.4.1.14988.1.1.3.100",
								Name: "mtxrHlTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.3",
									Name: "mtxrHlSensorValue",
									Transform: `
{{- $config := get (dict 
    "1" (dict "name" "temperature" "unit" "celsius" "family" "Health/Temperature")
    "2" (dict "name" "fan_speed" "unit" "rpm" "family" "Health/Cooling")
    "3" (dict "name" "voltage" "unit" "volts" "family" "Health/Power" "divisor" 10.0)
    "6" (dict "name" "sensor_status" "family" "Health/Status" 
         "mapping" (i64map 0 "not_ok" 1 "ok"))
) (index .Metric.Tags "sensor_type" | default "") -}}

{{- if $config -}}
  {{- setName .Metric (printf "%s_%s" .Metric.Name (get $config "name")) -}}
  {{- setFamily .Metric (get $config "family") -}}
  {{- with get $config "unit" -}}{{- setUnit $.Metric . -}}{{- end -}}
  {{- with get $config "divisor" -}}{{- setValue $.Metric (int64 (div (float64 $.Metric.Value) .)) -}}{{- end -}}
  {{- with get $config "mapping" -}}{{- setMappings $.Metric . -}}{{- end -}}
{{- end -}}

{{- deleteTag .Metric "sensor_type" -}}`,
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "sensor_name",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.2",
										Name: "mtxrHlSensorName",
									},
								},
								{
									Tag: "sensor_type",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.4.1.14988.1.1.3.100.1.4",
										Name: "mtxrHlSensorType",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.4.1.14988.1.1.3.100", []gosnmp.SnmpPDU{
					// Temperature sensor
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.3.1", 45),
					createStringPDU("1.3.6.1.4.1.14988.1.1.3.100.1.2.1", "cpu-temperature"),
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.4.1", 1),
					// PSU status sensor
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.3.2", 1),
					createStringPDU("1.3.6.1.4.1.14988.1.1.3.100.1.2.2", "psu1-state"),
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.4.2", 6),
					// Voltage sensor
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.3.3", 240), // 24.0V
					createStringPDU("1.3.6.1.4.1.14988.1.1.3.100.1.2.3", "psu-voltage"),
					createIntegerPDU("1.3.6.1.4.1.14988.1.1.3.100.1.4.3", 3),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "mtxrHlSensorValue_temperature",
					Value:      45,
					Tags:       map[string]string{"sensor_name": "cpu-temperature"},
					Unit:       "celsius",
					Family:     "Health/Temperature",
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:   "mtxrHlSensorValue_sensor_status",
					Value:  1,
					Tags:   map[string]string{"sensor_name": "psu1-state"},
					Family: "Health/Status",
					Mappings: map[int64]string{
						0: "not_ok",
						1: "ok",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "mtxrHlSensorValue_voltage",
					Value:      24,
					Tags:       map[string]string{"sensor_name": "psu-voltage"},
					Unit:       "volts",
					Family:     "Health/Power",
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table transformation with dynamic tags": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.10",
									Name: "ifInOctets",
									Transform: `
{{- $ifName := index .Metric.Tags "interface" | lower -}}
{{- if contains "eth" $ifName -}}
  {{- setFamily .Metric "Interfaces/Ethernet" -}}
  {{- setTag .Metric "interface_type" "ethernet" -}}
{{- else if contains "lo" $ifName -}}
  {{- setFamily .Metric "Interfaces/Loopback" -}}
  {{- setTag .Metric "interface_type" "loopback" -}}
{{- else -}}
  {{- setFamily .Metric "Interfaces/Other" -}}
  {{- setTag .Metric "interface_type" "other" -}}
{{- end -}}
{{- setDesc .Metric (printf "Traffic on %s interface" (index .Metric.Tags "interface")) -}}`,
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "lo0"),
					createCounter32PDU("1.3.6.1.2.1.2.2.1.10.3", 3000),
					createStringPDU("1.3.6.1.2.1.2.2.1.2.3", "tun0"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:        "ifInOctets",
					Value:       1000,
					Tags:        map[string]string{"interface": "eth0", "interface_type": "ethernet"},
					Family:      "Interfaces/Ethernet",
					Description: "Traffic on eth0 interface",
					MetricType:  "rate",
					IsTable:     true,
				},
				{
					Name:        "ifInOctets",
					Value:       2000,
					Tags:        map[string]string{"interface": "lo0", "interface_type": "loopback"},
					Family:      "Interfaces/Loopback",
					Description: "Traffic on lo0 interface",
					MetricType:  "rate",
					IsTable:     true,
				},
				{
					Name:        "ifInOctets",
					Value:       3000,
					Tags:        map[string]string{"interface": "tun0", "interface_type": "other"},
					Family:      "Interfaces/Other",
					Description: "Traffic on tun0 interface",
					MetricType:  "rate",
					IsTable:     true,
				},
			},
			expectedError: false,
		},
		"table transformation with conditional metrics": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2",
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
									Transform: `
{{- $status := .Metric.Value -}}
{{- if eq $status 1 -}}
  {{- setValue .Metric 1 -}}
{{- else -}}
  {{- setValue .Metric 0 -}}
{{- end -}}
{{- setMappings .Metric (i64map 0 "down" 1 "up") -}}
{{- setName .Metric "interface_status" -}}`,
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "interface",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.2.2.1.2",
										Name: "ifDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.1", 1), // up
					createStringPDU("1.3.6.1.2.1.2.2.1.2.1", "eth0"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.2", 2), // down
					createStringPDU("1.3.6.1.2.1.2.2.1.2.2", "eth1"),
					createIntegerPDU("1.3.6.1.2.1.2.2.1.8.3", 3), // testing
					createStringPDU("1.3.6.1.2.1.2.2.1.2.3", "eth2"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:  "interface_status",
					Value: 1,
					Tags:  map[string]string{"interface": "eth0"},
					Mappings: map[int64]string{
						0: "down",
						1: "up",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:  "interface_status",
					Value: 0,
					Tags:  map[string]string{"interface": "eth1"},
					Mappings: map[int64]string{
						0: "down",
						1: "up",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:  "interface_status",
					Value: 0,
					Tags:  map[string]string{"interface": "eth2"},
					Mappings: map[int64]string{
						0: "down",
						1: "up",
					},
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
		"table transformation with sprig string functions": {
			profile: &ddsnmp.Profile{
				SourceFile: "test-profile.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{
						{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.3.6.1.2.1.25.2.3",
								Name: "hrStorageTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{
									OID:  "1.3.6.1.2.1.25.2.3.1.5",
									Name: "hrStorageSize",
									Transform: `
{{- $descr := index .Metric.Tags "storage_descr" -}}
{{- if hasPrefix "/" $descr -}}
  {{- setFamily .Metric "Storage/FileSystem" -}}
  {{- setTag .Metric "mount_point" $descr -}}
  {{- setName .Metric "filesystem_size" -}}
{{- else if contains "Memory" $descr -}}
  {{- setFamily .Metric "Storage/Memory" -}}
  {{- setName .Metric "memory_size" -}}
{{- else -}}
  {{- setFamily .Metric "Storage/Other" -}}
{{- end -}}`,
								},
							},
							MetricTags: []ddprofiledefinition.MetricTagConfig{
								{
									Tag: "storage_descr",
									Symbol: ddprofiledefinition.SymbolConfigCompat{
										OID:  "1.3.6.1.2.1.25.2.3.1.3",
										Name: "hrStorageDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				expectSNMPWalk(m, gosnmp.Version2c, "1.3.6.1.2.1.25.2.3", []gosnmp.SnmpPDU{
					createGauge32PDU("1.3.6.1.2.1.25.2.3.1.5.1", 1000000),
					createStringPDU("1.3.6.1.2.1.25.2.3.1.3.1", "/"),
					createGauge32PDU("1.3.6.1.2.1.25.2.3.1.5.2", 500000),
					createStringPDU("1.3.6.1.2.1.25.2.3.1.3.2", "/var"),
					createGauge32PDU("1.3.6.1.2.1.25.2.3.1.5.3", 2000000),
					createStringPDU("1.3.6.1.2.1.25.2.3.1.3.3", "Physical Memory"),
					createGauge32PDU("1.3.6.1.2.1.25.2.3.1.5.4", 100000),
					createStringPDU("1.3.6.1.2.1.25.2.3.1.3.4", "Swap Space"),
				})
			},
			expectedResult: []ddsnmp.Metric{
				{
					Name:       "filesystem_size",
					Value:      1000000,
					Tags:       map[string]string{"storage_descr": "/", "mount_point": "/"},
					Family:     "Storage/FileSystem",
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "filesystem_size",
					Value:      500000,
					Tags:       map[string]string{"storage_descr": "/var", "mount_point": "/var"},
					Family:     "Storage/FileSystem",
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "memory_size",
					Value:      2000000,
					Tags:       map[string]string{"storage_descr": "Physical Memory"},
					Family:     "Storage/Memory",
					MetricType: "gauge",
					IsTable:    true,
				},
				{
					Name:       "hrStorageSize",
					Value:      100000,
					Tags:       map[string]string{"storage_descr": "Swap Space"},
					Family:     "Storage/Other",
					MetricType: "gauge",
					IsTable:    true,
				},
			},
			expectedError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			tc.setupMock(mockHandler)

			handleCrossTableTagsWithoutMetrics(tc.profile)
			if err := ddsnmp.CompileTransforms(tc.profile); err != nil {
				if tc.expectedError && tc.errorContains != "" && strings.Contains(err.Error(), tc.errorContains) {
					return // Expected error during compilation
				}
				t.Fatalf("Failed to compile transforms: %v", err)
			}

			missingOIDs := make(map[string]bool)
			tableCache := newTableCache(0, 0) // Cache disabled
			collector := newTableCollector(mockHandler, missingOIDs, tableCache, logger.New())

			result, err := collector.Collect(tc.profile)

			// TODO: the Table field is now compared as part of the metric; ensure expectedResult includes correct Table values
			for i := range result {
				result[i].Table = ""
			}

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assertTableMetricsEqual(t, tc.expectedResult, result)

			// Check missing OIDs if specified
			if tc.checkMissing != nil {
				for oid, shouldBeMissing := range tc.checkMissing {
					assert.Equal(t, shouldBeMissing, missingOIDs[oid], "OID %s missing status", oid)
				}
			}
		})
	}
}

func TestCollector_Collect_TableCaching(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ddsnmp.ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool
		cacheTTL       time.Duration
		collectCount   int // Number of times to call Collect()
		sleepBetween   time.Duration
	}{
		"table cache basic operation": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()

				// First collection: Full walk
				m.EXPECT().Version().Return(gosnmp.Version2c).Times(1)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.16.1",
							Type:  gosnmp.Counter32,
							Value: uint(500),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2",
							Type:  gosnmp.Counter32,
							Value: uint(2000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.16.2",
							Type:  gosnmp.Counter32,
							Value: uint(1500),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("eth1"),
						},
					}, nil,
				).Times(1)

				// Second collection: Only GET metrics (not tags)
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.2.2.1.10.1",
					"1.3.6.1.2.1.2.2.1.16.1",
					"1.3.6.1.2.1.2.2.1.10.2",
					"1.3.6.1.2.1.2.2.1.16.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.1",
								Type:  gosnmp.Counter32,
								Value: uint(1100), // Values changed
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.16.1",
								Type:  gosnmp.Counter32,
								Value: uint(600),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.2",
								Type:  gosnmp.Counter32,
								Value: uint(2200),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.16.2",
								Type:  gosnmp.Counter32,
								Value: uint(1700),
							},
						},
					}, nil,
				).Times(1)

				// Third collection: Still using cache
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.2.2.1.10.1",
					"1.3.6.1.2.1.2.2.1.16.1",
					"1.3.6.1.2.1.2.2.1.10.2",
					"1.3.6.1.2.1.2.2.1.16.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.1",
								Type:  gosnmp.Counter32,
								Value: uint(1200),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.16.1",
								Type:  gosnmp.Counter32,
								Value: uint(700),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.2",
								Type:  gosnmp.Counter32,
								Value: uint(2400),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.16.2",
								Type:  gosnmp.Counter32,
								Value: uint(1900),
							},
						},
					}, nil,
				).Times(1)
			},
			expectedResult: []*ddsnmp.ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []ddsnmp.Metric{
						{
							Name:       "ifInOctets",
							Value:      1200, // Latest value
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifOutOctets",
							Value:      700,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      2400,
							Tags:       map[string]string{"interface": "eth1"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifOutOctets",
							Value:      1900,
							Tags:       map[string]string{"interface": "eth1"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   true,
			cacheTTL:      30 * time.Second,
			collectCount:  3,
			sleepBetween:  10 * time.Millisecond,
		},
		"table cache expiration": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// First collection: Full walk
				gomock.InOrder(
					m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
						[]gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.1",
								Type:  gosnmp.Counter32,
								Value: uint(1000),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.2.1",
								Type:  gosnmp.OctetString,
								Value: []byte("eth0"),
							},
						}, nil,
					),

					// Second collection: Using cache
					m.EXPECT().Get([]string{"1.3.6.1.2.1.2.2.1.10.1"}).Return(
						&gosnmp.SnmpPacket{
							Variables: []gosnmp.SnmpPDU{
								{
									Name:  "1.3.6.1.2.1.2.2.1.10.1",
									Type:  gosnmp.Counter32,
									Value: uint(1100),
								},
							},
						}, nil,
					),

					// Third collection: After expiration, full walk again
					m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
						[]gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.1",
								Type:  gosnmp.Counter32,
								Value: uint(1200),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.2.1",
								Type:  gosnmp.OctetString,
								Value: []byte("eth0"),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.2",
								Type:  gosnmp.Counter32,
								Value: uint(2000),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.2.2",
								Type:  gosnmp.OctetString,
								Value: []byte("eth1"),
							},
						}, nil,
					),
				)
			},
			expectedResult: []*ddsnmp.ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []ddsnmp.Metric{
						{
							Name:       "ifInOctets",
							Value:      1200,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      2000,
							Tags:       map[string]string{"interface": "eth1"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   true,
			cacheTTL:      100 * time.Millisecond,
			collectCount:  3,
			sleepBetween:  60 * time.Millisecond, // Sleep less than TTL for second collection, but total > TTL for third
		},
		"table cache with multiple configs same table": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.14",
										Name: "ifInErrors",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.20",
										Name: "ifOutErrors",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// First collection: Walk table once
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Interface 1
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.16.1",
							Type:  gosnmp.Counter32,
							Value: uint(500),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.20.1",
							Type:  gosnmp.Counter32,
							Value: uint(5),
						},
					}, nil,
				).Times(1)

				// Second collection: Each config uses cache separately
				// First config GET
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.2.2.1.10.1",
					"1.3.6.1.2.1.2.2.1.16.1",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.10.1",
								Type:  gosnmp.Counter32,
								Value: uint(1100),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.16.1",
								Type:  gosnmp.Counter32,
								Value: uint(600),
							},
						},
					}, nil,
				).Times(1)

				// Second config GET
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.2.2.1.14.1",
					"1.3.6.1.2.1.2.2.1.20.1",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.14.1",
								Type:  gosnmp.Counter32,
								Value: uint(12),
							},
							{
								Name:  "1.3.6.1.2.1.2.2.1.20.1",
								Type:  gosnmp.Counter32,
								Value: uint(6),
							},
						},
					}, nil,
				).Times(1)
			},
			expectedResult: []*ddsnmp.ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []ddsnmp.Metric{
						// From first config
						{
							Name:       "ifInOctets",
							Value:      1100,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifOutOctets",
							Value:      600,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						// From second config
						{
							Name:       "ifInErrors",
							Value:      12,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifOutErrors",
							Value:      6,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   true,
			cacheTTL:      30 * time.Second,
			collectCount:  2,
			sleepBetween:  10 * time.Millisecond,
		},
		"table cache with cross-table-only columns": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1",
									Name: "panEntityFRUModuleTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1",
										Name: "panEntryFRUModulePowerUsed",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "ent_descr",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.47.1.1.1.1.2",
											Name: "entPhysicalDescr",
										},
										Table: "entPhysicalTable",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// First collection: Walk both table and cross-table column

				// Walk main table
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.25461.1.1.7.1.2.1").Return(
					[]gosnmp.SnmpPDU{
						createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 100),
						createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 150),
					}, nil,
				).Times(1)

				// Walk cross-table column
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.47.1.1.1.1.2").Return(
					[]gosnmp.SnmpPDU{
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.1", "Power Supply 1"),
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.2", "Power Supply 2"),
					}, nil,
				).Times(1)

				// Second collection: GET metrics only, cross-table column should be cached
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1",
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 110),
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 160),
						},
					}, nil,
				).Times(1)

				// Third collection: Still using cache
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1",
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 120),
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 170),
						},
					}, nil,
				).Times(1)

			},
			expectedResult: []*ddsnmp.ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []ddsnmp.Metric{
						{
							Name:       "panEntryFRUModulePowerUsed",
							Value:      120, // Latest value
							Tags:       map[string]string{"ent_descr": "Power Supply 1"},
							MetricType: "gauge",
							IsTable:    true,
						},
						{
							Name:       "panEntryFRUModulePowerUsed",
							Value:      170,
							Tags:       map[string]string{"ent_descr": "Power Supply 2"},
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   true,
			cacheTTL:      30 * time.Second,
			collectCount:  3,
			sleepBetween:  10 * time.Millisecond,
		},
		"table cache with multiple cross-table tags from same table": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1",
									Name: "panEntityFRUModuleTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1",
										Name: "panEntryFRUModulePowerUsed",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "ent_descr",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.47.1.1.1.1.2",
											Name: "entPhysicalDescr",
										},
										Table: "entPhysicalTable",
									},
									{
										Tag: "ent_type",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.47.1.1.1.1.3",
											Name: "entPhysicalVendorType",
										},
										Table: "entPhysicalTable",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// First collection: Walk both tables (order doesn't matter)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.25461.1.1.7.1.2.1").Return(
					[]gosnmp.SnmpPDU{
						createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 100),
						createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 150),
					}, nil,
				).Times(1)

				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.47.1.1.1.1").Return(
					[]gosnmp.SnmpPDU{
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.1", "Power Supply 1"),
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.2.2", "Power Supply 2"),
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.3.1", "Type A"),
						createStringPDU("1.3.6.1.2.1.47.1.1.1.1.3.2", "Type B"),
					}, nil,
				).Times(1)

				// Second collection: GET metrics only
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1",
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 110),
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 160),
						},
					}, nil,
				).Times(1)

				// Third collection: Still using cache
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1",
					"1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.1", 120),
							createGauge32PDU("1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1.2", 170),
						},
					}, nil,
				).Times(1)
			},
			expectedResult: []*ddsnmp.ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []ddsnmp.Metric{
						{
							Name:  "panEntryFRUModulePowerUsed",
							Value: 120, // Latest value
							Tags: map[string]string{
								"ent_descr": "Power Supply 1",
								"ent_type":  "Type A",
							},
							MetricType: "gauge",
							IsTable:    true,
						},
						{
							Name:  "panEntryFRUModulePowerUsed",
							Value: 170,
							Tags: map[string]string{
								"ent_descr": "Power Supply 2",
								"ent_type":  "Type B",
							},
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   true,
			cacheTTL:      30 * time.Second,
			collectCount:  3,
			sleepBetween:  10 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHandler := snmpmock.NewMockHandler(ctrl)
			tc.setupMock(mockHandler)

			collector := New(mockHandler, tc.profiles, logger.New())
			collector.DoTableMetrics = true

			// Configure cache based on test requirements
			if tc.enableCache {
				collector.tableCache.setTTL(tc.cacheTTL, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			var result []*ddsnmp.ProfileMetrics
			var err error

			// Perform multiple collections to test caching behavior
			for i := 0; i < tc.collectCount; i++ {
				if i > 0 && tc.sleepBetween > 0 {
					time.Sleep(tc.sleepBetween)
				}

				result, err = collector.Collect()

				// For intermediate collections, just verify no error
				if i < tc.collectCount-1 {
					assert.NoError(t, err)
				}
			}

			// Clear circular references in final result
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
					// TODO: the Table field is now compared as part of the metric; ensure expectedResult includes correct Table values
					profile.Metrics[i].Table = ""
				}
			}

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tc.expectedResult != nil {
				require.Equal(t, len(tc.expectedResult), len(result))
				for i := range tc.expectedResult {
					assert.Equal(t, tc.expectedResult[i].DeviceMetadata, result[i].DeviceMetadata)
					assert.Equal(t, tc.expectedResult[i].Tags, result[i].Tags)
					assert.ElementsMatch(t, tc.expectedResult[i].Metrics, result[i].Metrics)
				}
			} else {
				assert.Nil(t, result)
			}
		})
	}
}
