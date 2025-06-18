// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmpmock "github.com/gosnmp/gosnmp/mocks"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_ScalarMetrics(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool // Whether to enable cache for this test
	}{
		"successful collection with scalar metrics only": {
			profiles: []*ddsnmp.Profile{
				{
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
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{"1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0"})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
							{
								Name:  "1.3.6.1.2.1.1.5.0",
								Type:  gosnmp.OctetString,
								Value: []byte("test-host"),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
						},
						// sysName will be skipped because it can't be converted to int64
					},
				},
			},
			expectedError: false,
		},
		"successful collection with global tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						MetricTags: []ddprofiledefinition.MetricTagConfig{
							{
								Tag: "device_vendor",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// First call for global tags
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("Cisco IOS"),
							},
						},
					}, nil,
				)
				// Second call for metrics
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Tags:           map[string]string{"device_vendor": "Cisco IOS"},
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"successful collection with device metadata": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metadata: ddprofiledefinition.MetadataConfig{
							"device": ddprofiledefinition.MetadataResourceConfig{
								Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
									"vendor": {
										Value: "dell",
									},
									"serial_number": {
										Symbol: ddprofiledefinition.SymbolConfig{
											OID:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
											Name: "chassisSerialNumber",
										},
									},
								},
							},
						},
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// First call for device metadata
				m.EXPECT().Get([]string{"1.3.6.1.4.1.674.10892.5.1.3.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
								Type:  gosnmp.OctetString,
								Value: []byte("ABC123"),
							},
						},
					}, nil,
				)
				// Second call for metrics
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source: "test-profile.yaml",
					DeviceMetadata: map[string]string{
						"vendor":        "dell",
						"serial_number": "ABC123",
					},
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"metric with scale factor": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.2"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.2",
								Type:  gosnmp.Gauge32,
								Value: uint(1024),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "memoryKilobytes",
							Value:      1024000, // 1024 * 1000
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"metric with extract_value": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.8"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.8",
								Type:  gosnmp.OctetString,
								Value: []byte("25C"),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "temperature",
							Value:      25,
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"global tags with mapping": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						MetricTags: []ddprofiledefinition.MetricTagConfig{
							{
								Tag: "device_type",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.1.2.0",
									Name: "sysObjectID",
								},
								Mapping: map[string]string{
									"1.3.6.1.4.1.9.1.1": "router",
									"1.3.6.1.4.1.9.1.2": "switch",
								},
							},
						},
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// First call for global tags
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.2.0",
								Type:  gosnmp.ObjectIdentifier,
								Value: "1.3.6.1.4.1.9.1.1",
							},
						},
					}, nil,
				)
				// Second call for metrics
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Tags:           map[string]string{"device_type": "router"},
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"OID not found - returns empty metrics": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.NoSuchObject,
								Value: nil,
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					DeviceMetadata: nil,
					Metrics:        []Metric{},
				},
			},
			expectedError: false,
		},
		"SNMP error": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					(*gosnmp.SnmpPacket)(nil),
					errors.New("SNMP timeout"),
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "SNMP timeout",
		},
		"empty profile - no metrics defined": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "empty-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
			},
			expectedResult: []*ProfileMetrics{
				{
					DeviceMetadata: nil,
					Metrics:        []Metric{},
				},
			},
			expectedError: false,
		},
		"multiple profiles - one fails": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// First profile succeeds
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
						},
					}, nil,
				)
				// Second profile fails
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					(*gosnmp.SnmpPacket)(nil),
					errors.New("connection refused"),
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "profile1.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false, // Should return partial results
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
				collector.tableCache.setTTL(30*time.Second, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_ValueMappings(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
	}{
		"metric with string to int mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.2"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.2",
								Type:  gosnmp.OctetString,
								Value: []byte("WARNING"),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"metric with int to string mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.2.2.1.8"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.8",
								Type:  gosnmp.Integer,
								Value: 2, // down
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"metric with extract_value and int to string mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.8"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.8",
								Type:  gosnmp.OctetString,
								Value: []byte("Fan2"),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"metric with int to int mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.2.2.1.7"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.7",
								Type:  gosnmp.Integer,
								Value: 2, // down
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"metric with partial string to int mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.2"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.2",
								Type:  gosnmp.OctetString,
								Value: []byte("CRITICAL"),
							},
						},
					}, nil,
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "strconv.ParseInt",
		},
		"metric with mixed mapping values": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.12124.1.1.2"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.12124.1.1.2",
								Type:  gosnmp.OctetString,
								Value: []byte("ERROR"),
							},
						},
					}, nil,
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "strconv.ParseInt",
		},
		"metric with no mapping": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.TimeTicks,
								Value: uint32(123456),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "sysUpTime",
							Value:      123456,
							MetricType: "gauge",
							Mappings:   nil, // No mappings
						},
					},
				},
			},
			expectedError: false,
		},
		"metric with numeric value and int to string mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.2.2.1.7"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.7",
								Type:  gosnmp.Integer,
								Value: 1, // up
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"metric with string value and string to int mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.318.1.1.1.2.2.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.318.1.1.1.2.2.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("batteryLow"),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
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
			collector.tableCache.setTTL(0, 0) // Disable cache

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_TableMetricsBasic(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool
	}{
		"basic table with single metric": {
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
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2",
							Type:  gosnmp.Counter32,
							Value: uint(2000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("eth1"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with multiple metrics": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Row 1
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
							Name:  "1.3.6.1.2.1.2.2.1.8.1",
							Type:  gosnmp.Integer,
							Value: 1, // up
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						// Row 2
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
							Name:  "1.3.6.1.2.1.2.2.1.8.2",
							Type:  gosnmp.Integer,
							Value: 2, // down
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("eth1"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with tag mapping": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("lo0"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.1",
							Type:  gosnmp.Integer,
							Value: 24, // softwareLoopback
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.2",
							Type:  gosnmp.Counter32,
							Value: uint(5),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.2",
							Type:  gosnmp.Integer,
							Value: 6, // ethernetCsmacd
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with missing rows": {
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
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Row 1 - complete
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
						// Row 2 - missing ifOutOctets
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
						// Row 3 - missing tag (ifDescr)
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.3",
							Type:  gosnmp.Counter32,
							Value: uint(3000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.16.3",
							Type:  gosnmp.Counter32,
							Value: uint(1500),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with static tags": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
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
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with extract_value in tag": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0 - Primary Network Interface"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.2",
							Type:  gosnmp.Counter32,
							Value: uint(5),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("lo0 - Loopback"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"table with multiple tag columns": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
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
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.1",
							Type:  gosnmp.Integer,
							Value: 6,
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.7.1",
							Type:  gosnmp.Integer,
							Value: 1, // up
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"empty table": {
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
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics:        []Metric{},
				},
			},
			expectedError: false,
		},
		"table walk error": {
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
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					nil,
					errors.New("timeout walking table"),
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "timeout walking table",
		},
		"table with scale factor": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.9.9.109.1.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.9.9.109.1.1.1.1.12.1",
							Type:  gosnmp.Gauge32,
							Value: uint(2048), // 2048 KB
						},
						{
							Name:  "1.3.6.1.4.1.9.9.109.1.1.1.1.2.1",
							Type:  gosnmp.Integer,
							Value: 1,
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "cpmCPUMemoryUsed",
							Value:      2097152, // 2048 * 1024
							Tags:       map[string]string{"cpu_id": "1"},
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"table with snmpv1 (using WalkAll instead of BulkWalkAll)": {
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
				m.EXPECT().Version().Return(gosnmp.Version1)
				m.EXPECT().WalkAll("1.3.6.1.2.1.2.2").Return(
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
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "ifInOctets",
							Value:      1000,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
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
				collector.tableCache.setTTL(30*time.Second, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_CrossTableTags(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool
	}{
		"cross-table tag from ifXTable": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// First walk for ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.2",
							Type:  gosnmp.Counter32,
							Value: uint(20),
						},
					}, nil,
				)
				// Second walk for ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/1"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"cross-table tag with missing values": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable - has 3 interfaces
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.2",
							Type:  gosnmp.Counter32,
							Value: uint(20),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.3",
							Type:  gosnmp.Counter32,
							Value: uint(30),
						},
					}, nil,
				)
				// Walk ifXTable - only has 2 interfaces (missing index 3)
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/1"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
							Tags:       nil, // No cross-table tag found
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"multiple cross-table tags": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
							Type:  gosnmp.OctetString,
							Value: []byte("Uplink to Core"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"cross-table tag with mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
										Tag: "oper_status",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.8",
											Name: "ifOperStatus",
										},
										Table: "ifTable",
										Mapping: map[string]string{
											"1": "up",
											"2": "down",
											"3": "testing",
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
										OID:  "1.3.6.1.2.1.2.2.1.8",
										Name: "ifOperStatus",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk cieIfInterfaceTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.9.9.276.1.1.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.9.9.276.1.1.2.1.1.1",
							Type:  gosnmp.Counter32,
							Value: uint(5),
						},
						{
							Name:  "1.3.6.1.4.1.9.9.276.1.1.2.1.1.2",
							Type:  gosnmp.Counter32,
							Value: uint(3),
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/1"),
						},
					}, nil,
				)
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.8.1",
							Type:  gosnmp.Integer,
							Value: 1, // up
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.8.2",
							Type:  gosnmp.Integer,
							Value: 2, // down
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:  "cieIfResetCount",
							Value: 5,
							Tags: map[string]string{
								"interface":   "GigabitEthernet0/0",
								"oper_status": "up",
							},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:  "cieIfResetCount",
							Value: 3,
							Tags: map[string]string{
								"interface":   "GigabitEthernet0/1",
								"oper_status": "down",
							},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifOperStatus",
							Value:      1,
							MetricType: "gauge",
							IsTable:    true,
						},
						{
							Name:       "ifOperStatus",
							Value:      2,
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tag when referenced table not walked": {
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
							// Note: ifXTable is NOT in the metrics list
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Only walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.2",
							Type:  gosnmp.Counter32,
							Value: uint(20),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "ifInErrors",
							Value:      10,
							Tags:       nil, // No cross-table tag because ifXTable wasn't walked
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifInErrors",
							Value:      20,
							Tags:       nil,
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tag with extract_value": {
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
											OID:                  "1.3.6.1.2.1.31.1.1.1.18",
											Name:                 "ifAlias",
											ExtractValueCompiled: mustCompileRegex(`\[(\w+)\]`), // Extract value in brackets
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
										OID:  "1.3.6.1.2.1.31.1.1.1.18",
										Name: "ifAlias",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
							Type:  gosnmp.OctetString,
							Value: []byte("Connection to [CORE-SW1] port 24"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "ifInOctets",
							Value:      1000,
							Tags:       nil,
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"same and cross-table tags combined": {
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
										OID:  "1.3.6.1.2.1.2.2.1.14",
										Name: "ifInErrors",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "if_desc",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
										// No Table field = same table
									},
									{
										Tag: "if_name",
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
										// No Table field = same table
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
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("Ethernet Interface"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.1",
							Type:  gosnmp.Integer,
							Value: 6,
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:  "ifInErrors",
							Value: 10,
							Tags: map[string]string{
								"if_desc": "Ethernet Interface",
								"if_name": "GigabitEthernet0/0",
								"if_type": "ethernet",
							},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table with caching disabled": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.14.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "ifInErrors",
							Value:      10,
							Tags:       map[string]string{"interface": "GigabitEthernet0/0"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
			enableCache:   false, // Explicitly disable cache
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
				collector.tableCache.setTTL(30*time.Second, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_IndexBasedAndTransforms(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool
	}{
		// Index-based Tags Tests
		"basic index tag": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.4.31.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.1", // IPv4
							Type:  gosnmp.Counter64,
							Value: uint64(1000),
						},
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.2", // IPv6
							Type:  gosnmp.Counter64,
							Value: uint64(2000),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"multiple index positions": {
			profiles: []*ddsnmp.Profile{
				{
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.9.9.147.1.2.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.20.2", // service_type=20, stat_type=2
							Type:  gosnmp.Counter64,
							Value: uint64(100),
						},
						{
							Name:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.21.3", // service_type=21, stat_type=3
							Type:  gosnmp.Counter64,
							Value: uint64(200),
						},
						{
							Name:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.20.3", // service_type=20, stat_type=3
							Type:  gosnmp.Counter64,
							Value: uint64(300),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "cfwConnectionStatValue",
							Value:      100,
							Tags:       map[string]string{"service_type": "20", "stat_type": "2"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "cfwConnectionStatValue",
							Value:      200,
							Tags:       map[string]string{"service_type": "21", "stat_type": "3"},
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "cfwConnectionStatValue",
							Value:      300,
							Tags:       map[string]string{"service_type": "20", "stat_type": "3"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index tag with mapping": {
			profiles: []*ddsnmp.Profile{
				{
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
											"0": "unknown",
											"1": "ipv4",
											"2": "ipv6",
											"3": "ipv4z",
											"4": "ipv6z",
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
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.4.31.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.1", // IPv4
							Type:  gosnmp.Counter64,
							Value: uint64(1000),
						},
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.2", // IPv6
							Type:  gosnmp.Counter64,
							Value: uint64(2000),
						},
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.0", // Unknown
							Type:  gosnmp.Counter64,
							Value: uint64(10),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
							Tags:       map[string]string{"ip_version": "unknown"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index tag with missing position": {
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1", // Single index
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2.3", // Multi-part index
							Type:  gosnmp.Counter32,
							Value: uint(2000),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},

		// Index Transformation Tests
		"basic index transform": {
			profiles: []*ddsnmp.Profile{
				{
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
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3",
											Name: "cpiPduName",
										},
										Table: "cpiPduTable",
										Tag:   "pdu_name",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 1, End: 7}, // Extract MAC address portion
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
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk cpiPduBranchTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.30932.1.10.1.3.110").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.3.110.1.3.1.6.0.36.155.53.3.246", // Branch ID + MAC
							Type:  gosnmp.Gauge32,
							Value: uint(150),
						},
					}, nil,
				)
				// Walk cpiPduTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.30932.1.10.1.2.10").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3.6.0.36.155.53.3.246", // Just MAC
							Type:  gosnmp.OctetString,
							Value: []byte("PDU-01"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "cpiPduBranchCurrent",
							Value:      150,
							Tags:       map[string]string{"pdu_name": "PDU-01"},
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"multiple index transform ranges": {
			profiles: []*ddsnmp.Profile{
				{
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
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.12345.2.1.1.1",
											Name: "customTag",
										},
										Table: "customRefTable",
										Tag:   "custom_ref",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 1, End: 2}, // First two positions
											{Start: 4, End: 6}, // Positions 5-7
										},
									},
								},
							},
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.12345.2.1",
									Name: "customRefTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.12345.2.1.1.1",
										Name: "customTag",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk customTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.12345.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.12345.1.1.1.1.1.2.3.4.5.6.7", // Index: 1.2.3.4.5.6.7
							Type:  gosnmp.Counter32,
							Value: uint(500),
						},
					}, nil,
				)
				// Walk customRefTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.12345.2.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.12345.2.1.1.1.2.3.5.6.7", // Index: 2.3.5.6.7 (matches transform)
							Type:  gosnmp.OctetString,
							Value: []byte("CustomValue"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "customMetric",
							Value:      500,
							Tags:       map[string]string{"custom_ref": "CustomValue"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index transform with out of bounds": {
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
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.31.1.1.1.1",
											Name: "ifName",
										},
										Table: "ifXTable",
										Tag:   "interface",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 0, End: 5}, // Trying to extract 6 positions from single index
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
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				// Walk ifTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1", // Single index
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1.2.3.4.5.6", // Multi-part index
							Type:  gosnmp.Counter32,
							Value: uint(2000),
						},
					}, nil,
				)
				// Walk ifXTable
				m.EXPECT().Version().Return(gosnmp.Version2c)
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1.2.3.4.5.6", // Matches second row
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/0"),
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "ifInOctets",
							Value:      1000,
							Tags:       nil, // Transform fails - index too short
							MetricType: "rate",
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      2000,
							Tags:       map[string]string{"interface": "GigabitEthernet0/0"},
							MetricType: "rate",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
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
				collector.tableCache.setTTL(30*time.Second, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_OpaqueTypes(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
		enableCache    bool
	}{
		"opaque float metric": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.1.0",
									Name: "cpmCPULoadAvg1min",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.2.0",
									Name: "cpmCPULoadAvg5min",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.3.0",
									Name: "cpmCPULoadAvg15min",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.9.9.305.1.1.1.0",
					"1.3.6.1.4.1.9.9.305.1.1.2.0",
					"1.3.6.1.4.1.9.9.305.1.1.3.0",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.9.9.305.1.1.1.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(0.75),
							},
							{
								Name:  "1.3.6.1.4.1.9.9.305.1.1.2.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(1.23),
							},
							{
								Name:  "1.3.6.1.4.1.9.9.305.1.1.3.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(2.45),
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
		},
		"opaque double metric": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1",
									Name: "cpqHeSysBatteryVoltage",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2",
									Name: "cpqHeSysBatteryCurrent",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3",
									Name: "cpqHeSysBatteryCapacity",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1",
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2",
					"1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.1",
								Type:  gosnmp.OpaqueDouble,
								Value: 12.6,
							},
							{
								Name:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.2",
								Type:  gosnmp.OpaqueDouble,
								Value: 2.4,
							},
							{
								Name:  "1.3.6.1.4.1.232.6.2.6.8.1.4.1.1.6.3",
								Type:  gosnmp.OpaqueDouble,
								Value: 98.5,
							},
						},
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
				},
			},
			expectedError: false,
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
				collector.tableCache.setTTL(30*time.Second, 0)
			} else {
				collector.tableCache.setTTL(0, 0) // Disable cache
			}

			result, err := collector.Collect()

			// Clear circular references
			for _, profile := range result {
				for i := range profile.Metrics {
					profile.Metrics[i].Profile = nil
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

func TestCollector_Collect_TableCaching(t *testing.T) {
	tests := map[string]struct {
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
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
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
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

			var result []*ProfileMetrics
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

func mustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}
