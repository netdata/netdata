// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"regexp"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmpmock "github.com/gosnmp/gosnmp/mocks"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		name           string
		profiles       []*ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult []*ProfileMetrics
		expectedError  bool
		errorContains  string
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
			expectedError: false, // Changed to false - partial success is not an error
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
		"table metrics with same-table tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "IF-MIB",
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
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Row 1 - index 1
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
							Value: uint(2000),
						},
						// Row 2 - index 2
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.2",
							Type:  gosnmp.OctetString,
							Value: []byte("eth1"),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2",
							Type:  gosnmp.Counter32,
							Value: uint(3000),
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.16.2",
							Type:  gosnmp.Counter32,
							Value: uint(4000),
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
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ifOutOctets",
							Value:      2000,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      3000,
							Tags:       map[string]string{"interface": "eth1"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ifOutOctets",
							Value:      4000,
							Tags:       map[string]string{"interface": "eth1"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"table metrics with tag mapping": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "IF-MIB",
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
										Tag: "if_type",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.3",
											Name: "ifType",
										},
										Mapping: map[string]string{
											"1": "other",
											"2": "regular1822",
											"6": "ethernetCsmacd",
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
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Row 1
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.1",
							Type:  gosnmp.Integer,
							Value: 6, // ethernetCsmacd
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
						},
						// Row 2
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.2",
							Type:  gosnmp.Integer,
							Value: 1, // other
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2",
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
							Tags:       map[string]string{"if_type": "ethernetCsmacd"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      2000,
							Tags:       map[string]string{"if_type": "other"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"table metrics with pattern matching tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:                  "1.3.6.1.4.1.1000.1.1.2",
											Name:                 "myDescription",
											ExtractValueCompiled: mustCompileRegex(`Interface (\w+)`),
										},
										Tag: "port",
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
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("Interface Gi0/1"),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"port": "Gi0"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"table metrics with static tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								StaticTags: []string{"table_type:performance", "source:snmp"},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.1.1.2",
											Name: "ifName",
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
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.2.1",
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
							Name:  "myMetric",
							Value: 100,
							StaticTags: map[string]string{
								"table_type": "performance",
								"source":     "snmp",
							},
							Tags: map[string]string{
								"interface": "eth0",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"table metrics with missing tag values": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "IF-MIB",
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
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Row 1 - has both metric and tag
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
						// Row 2 - missing tag value
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.2",
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
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      2000,
							Tags:       nil, // No interface tag because it's missing
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},

		"cross-table tags with same index": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "CISCO-IF-EXTENSION-MIB",
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
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.31.1.1.1.1",
											Name: "ifName",
										},
										Table: "ifXTable",
										Tag:   "interface",
									},
								},
							},
							{
								MIB: "IF-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1",
									Name: "ifXTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed - this table is only used for cross-table tags
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk cieIfInterfaceTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.9.9.276.1.1.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.9.9.276.1.1.2.1.1.1",
							Type:  gosnmp.Counter32,
							Value: uint(10),
						},
						{
							Name:  "1.3.6.1.4.1.9.9.276.1.1.2.1.1.2",
							Type:  gosnmp.Counter32,
							Value: uint(20),
						},
					}, nil,
				)

				// Walk ifXTable
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						// ifName values that will be used as tags
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/1"),
						},
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/2"),
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
							Name:       "cieIfResetCount",
							Value:      10,
							Tags:       map[string]string{"interface": "GigabitEthernet0/1"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "cieIfResetCount",
							Value:      20,
							Tags:       map[string]string{"interface": "GigabitEthernet0/2"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with missing referenced table": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.31.1.1.1.1",
											Name: "ifName",
										},
										Table: "ifXTable", // This table is not defined
										Tag:   "interface",
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

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       nil, // No cross-table tag because referenced table is missing
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with missing value in referenced table": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
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
									},
								},
							},
							{
								MIB: "IF-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1",
									Name: "ifXTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed - this table is only used for cross-table tags
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable - has rows 1 and 2
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.2",
							Type:  gosnmp.Gauge32,
							Value: uint(200),
						},
					}, nil,
				)

				// Walk ifXTable - only has ifName for row 1, missing row 2
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("eth0"),
						},
						// Missing ifName for index 2
					}, nil,
				)
			},
			expectedResult: []*ProfileMetrics{
				{
					Source:         "test-profile.yaml",
					DeviceMetadata: nil,
					Metrics: []Metric{
						{
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"interface": "eth0"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
						{
							Name:       "myMetric",
							Value:      200,
							Tags:       nil, // No tag because ifName is missing for this index
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with mapping": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.3",
											Name: "ifType",
										},
										Table: "ifTable",
										Tag:   "if_type",
										Mapping: map[string]string{
											"6":  "ethernet",
											"71": "wifi",
										},
									},
								},
							},
							{
								MIB: "IF-MIB",
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
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.2",
							Type:  gosnmp.Gauge32,
							Value: uint(200),
						},
					}, nil,
				)

				// Walk ifTable
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.1",
							Type:  gosnmp.Integer,
							Value: 6, // ethernet
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.3.2",
							Type:  gosnmp.Integer,
							Value: 71, // wifi
						},
						{
							Name:  "1.3.6.1.2.1.2.2.1.10.1",
							Type:  gosnmp.Counter32,
							Value: uint(1000),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"if_type": "ethernet"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
						{
							Name:       "myMetric",
							Value:      200,
							Tags:       map[string]string{"if_type": "wifi"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
						{
							Name:       "ifInOctets",
							Value:      1000,
							Tags:       nil,
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"multiple cross-table tags from different tables": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
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
									},
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
										Table: "ifTable",
										Tag:   "description",
									},
								},
							},
							{
								MIB: "IF-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1",
									Name: "ifXTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed - this table is only used for cross-table tags
								},
							},
							{
								MIB: "IF-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed - this table is only used for cross-table tags
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
					}, nil,
				)

				// Walk ifXTable
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.31.1.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigabitEthernet0/1"),
						},
					}, nil,
				)

				// Walk ifTable
				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.2.1.2.2.1.2.1",
							Type:  gosnmp.OctetString,
							Value: []byte("GigE0/1"),
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
							Name:  "myMetric",
							Value: 100,
							Tags: map[string]string{
								"interface":   "GigabitEthernet0/1",
								"description": "GigE0/1",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},

		"index-based tags with single position": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "IP-MIB",
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
										Tag:   "ipversion",
										Mapping: map[string]string{
											"0":  "unknown",
											"1":  "ipv4",
											"2":  "ipv6",
											"3":  "ipv4z",
											"4":  "ipv6z",
											"16": "dns",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.4.31.1").Return(
					[]gosnmp.SnmpPDU{
						// IPv4 row
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.1",
							Type:  gosnmp.Counter64,
							Value: uint64(1000),
						},
						// IPv6 row
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.2",
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
							Tags:       map[string]string{"ipversion": "ipv4"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:       "ipSystemStatsHCInReceives",
							Value:      2000,
							Tags:       map[string]string{"ipversion": "ipv6"},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with multiple positions": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "CISCO-FIREWALL-MIB",
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
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.9.9.147.1.2.2.2").Return(
					[]gosnmp.SnmpPDU{
						// Index: 20.2 (service_type=20, stat_type=2)
						{
							Name:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.20.2",
							Type:  gosnmp.Counter64,
							Value: uint64(4087850099),
						},
						// Index: 21.3 (service_type=21, stat_type=3)
						{
							Name:  "1.3.6.1.4.1.9.9.147.1.2.2.2.1.5.21.3",
							Type:  gosnmp.Counter64,
							Value: uint64(5000000),
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
							Name:  "cfwConnectionStatValue",
							Value: 4087850099,
							Tags: map[string]string{
								"service_type": "20",
								"stat_type":    "2",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
						{
							Name:  "cfwConnectionStatValue",
							Value: 5000000,
							Tags: map[string]string{
								"service_type": "21",
								"stat_type":    "3",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with missing mapping value": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "IP-MIB",
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
										Tag:   "ipversion",
										Mapping: map[string]string{
											"1": "ipv4",
											"2": "ipv6",
											// Missing mapping for "99"
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

				m.EXPECT().BulkWalkAll("1.3.6.1.2.1.4.31.1").Return(
					[]gosnmp.SnmpPDU{
						// Index 99 - no mapping defined
						{
							Name:  "1.3.6.1.2.1.4.31.1.1.4.99",
							Type:  gosnmp.Counter64,
							Value: uint64(3000),
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
							Value:      3000,
							Tags:       map[string]string{"ipversion": "99"}, // Raw value when no mapping exists
							MetricType: ddprofiledefinition.ProfileMetricTypeRate,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with complex multi-part index": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myComplexTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 1,
										Tag:   "first",
									},
									{
										Index: 3,
										Tag:   "third",
									},
									{
										Index: 5,
										Tag:   "fifth",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						// Complex index: 10.20.30.40.50
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.10.20.30.40.50",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
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
							Name:  "myMetric",
							Value: 100,
							Tags: map[string]string{
								"first": "10",
								"third": "30",
								"fifth": "50",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with out-of-range position": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 1,
										Tag:   "first",
									},
									{
										Index: 5, // This position doesn't exist in index "1.2"
										Tag:   "fifth",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						// Simple index: 1.2
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1.2",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
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
							Name:  "myMetric",
							Value: 100,
							Tags: map[string]string{
								"first": "1",
								// "fifth" tag is not present because position 5 doesn't exist
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags combined with symbol tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 1,
										Tag:   "index_tag",
									},
									{
										Tag: "name_tag",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.1.1.2",
											Name: "myName",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.5",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.2.5",
							Type:  gosnmp.OctetString,
							Value: []byte("device-5"),
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
							Name:  "myMetric",
							Value: 100,
							Tags: map[string]string{
								"index_tag": "5",
								"name_tag":  "device-5",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with default tag name": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 2,
										// No tag name specified, should default to "index2"
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.10.20",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"index2": "20"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"index-based tags with single component index": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 1,
										Tag:   "id",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						// Single component index: just "42"
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.42",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"id": "42"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},

		"cross-table tags with index transformation": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "CPI-UNITY-MIB",
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
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 1, End: 7},
										},
										Tag: "pdu_name",
									},
								},
							},
							{
								MIB: "CPI-UNITY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.30932.1.10.1.2.10",
									Name: "cpiPduTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed - only used for cross-table reference
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk cpiPduBranchTable
				// Index structure: <branch_id>.<mac_address>
				// Example: 1.6.0.36.155.53.3.246
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.30932.1.10.1.3.110").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.3.110.1.3.1.6.0.36.155.53.3.246",
							Type:  gosnmp.Gauge32,
							Value: uint(150), // 1.5 Amps
						},
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.3.110.1.3.2.6.0.36.155.53.3.247",
							Type:  gosnmp.Gauge32,
							Value: uint(200), // 2.0 Amps
						},
					}, nil,
				)

				// Walk cpiPduTable
				// Index structure: <mac_address> only
				// Example: 6.0.36.155.53.3.246
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.30932.1.10.1.2.10").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3.6.0.36.155.53.3.246",
							Type:  gosnmp.OctetString,
							Value: []byte("PDU-A"),
						},
						{
							Name:  "1.3.6.1.4.1.30932.1.10.1.2.10.1.3.6.0.36.155.53.3.247",
							Type:  gosnmp.OctetString,
							Value: []byte("PDU-B"),
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
							Tags:       map[string]string{"pdu_name": "PDU-A"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
						{
							Name:       "cpiPduBranchCurrent",
							Value:      200,
							Tags:       map[string]string{"pdu_name": "PDU-B"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with multiple index transformations": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myComplexTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.2.1.1",
											Name: "refName",
										},
										Table: "refTable",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 0, End: 1},
											{Start: 3, End: 5},
										},
										Tag: "ref_name",
									},
								},
							},
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.2",
									Name: "refTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myComplexTable
				// Index: 1.2.3.4.5.6.7
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1.2.3.4.5.6.7",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
					}, nil,
				)

				// Walk refTable
				// Expected transformed index: 1.2.4.5.6 (positions 1-2 and 4-6)
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.1.2.4.5.6",
							Type:  gosnmp.OctetString,
							Value: []byte("Complex-Ref"),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"ref_name": "Complex-Ref"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with index transformation no match": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.2.1.1",
											Name: "refName",
										},
										Table: "refTable",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 2, End: 4},
										},
										Tag: "ref_name",
									},
								},
							},
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.2",
									Name: "refTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1.2.3",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
					}, nil,
				)

				// Walk refTable - but it doesn't have the transformed index
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.9.9.9", // Different index
							Type:  gosnmp.OctetString,
							Value: []byte("Other-Ref"),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       nil, // No tag because transformed index not found
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with invalid index transformation": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.2.1.1",
											Name: "refName",
										},
										Table: "refTable",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 5, End: 10}, // Out of bounds
										},
										Tag: "ref_name",
									},
								},
							},
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.2",
									Name: "refTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable with short index
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1.2",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
					}, nil,
				)

				// Walk refTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.1.2",
							Type:  gosnmp.OctetString,
							Value: []byte("Ref"),
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
							Name:       "myMetric",
							Value:      100,
							Tags:       nil, // No tag because transformation failed
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags with transformation and mapping": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.2.1.1",
											Name: "refType",
										},
										Table: "refTable",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 1, End: 2},
										},
										Tag: "ref_type",
										Mapping: map[string]string{
											"1": "primary",
											"2": "secondary",
											"3": "backup",
										},
									},
								},
							},
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.2",
									Name: "refTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.5.10.20",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.6.20.30",
							Type:  gosnmp.Gauge32,
							Value: uint(200),
						},
					}, nil,
				)

				// Walk refTable with transformed indexes
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.10.20", // Matches first row (positions 2-3)
							Type:  gosnmp.Integer,
							Value: 1, // Will map to "primary"
						},
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.20.30", // Matches second row (positions 2-3)
							Type:  gosnmp.Integer,
							Value: 3, // Will map to "backup"
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
							Name:       "myMetric",
							Value:      100,
							Tags:       map[string]string{"ref_type": "primary"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
						{
							Name:       "myMetric",
							Value:      200,
							Tags:       map[string]string{"ref_type": "backup"},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"cross-table tags mixed with index-based tags": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.1",
									Name: "myTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.1000.1.1.1",
										Name: "myMetric",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Index: 1,
										Tag:   "branch_id",
									},
									{
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.1000.2.1.1",
											Name: "pduName",
										},
										Table: "pduTable",
										IndexTransform: []ddprofiledefinition.MetricIndexTransform{
											{Start: 1, End: 7},
										},
										Tag: "pdu_name",
									},
								},
							},
							{
								MIB: "MY-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.1000.2",
									Name: "pduTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									// No symbols needed
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

				// Walk myTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.1").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.1.1.1.1.6.0.36.155.53.3.246",
							Type:  gosnmp.Gauge32,
							Value: uint(100),
						},
					}, nil,
				)

				// Walk pduTable
				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.1000.2").Return(
					[]gosnmp.SnmpPDU{
						{
							Name:  "1.3.6.1.4.1.1000.2.1.1.6.0.36.155.53.3.246",
							Type:  gosnmp.OctetString,
							Value: []byte("Main-PDU"),
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
							Name:  "myMetric",
							Value: 100,
							Tags: map[string]string{
								"branch_id": "1",
								"pdu_name":  "Main-PDU",
							},
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},

		"scalar metric with OpaqueFloat": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.4.2.12.1.0",
									Name: "temperature",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.2.12.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.2.12.1.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(29.5),
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
							Value:      29, // Truncated from 29.5
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"scalar metric with OpaqueDouble": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.4.4.1.1.0",
									Name: "voltage",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.4.1.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.4.1.1.0",
								Type:  gosnmp.OpaqueDouble,
								Value: float64(232.75),
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
							Name:       "voltage",
							Value:      232, // Truncated from 232.75
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"scalar metric with OpaqueFloat and scale factor": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:         "1.3.6.1.4.1.6574.4.2.12.1.0",
									Name:        "temperatureMilliDegrees",
									ScaleFactor: 1000, // Convert to milli-degrees to preserve precision
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.2.12.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.2.12.1.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(29.567),
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
							Name:       "temperatureMilliDegrees",
							Value:      29566, // 29.567 * 1000
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"table metric with OpaqueFloat": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								MIB: "SYNOLOGY-SYSTEM-MIB",
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.1",
									Name: "temperatureTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.4.1.6574.1.2",
										Name: "temperature",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "sensor",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.4.1.6574.1.1",
											Name: "temperatureIndex",
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

				m.EXPECT().BulkWalkAll("1.3.6.1.4.1.6574.1").Return(
					[]gosnmp.SnmpPDU{
						// Row 1
						{
							Name:  "1.3.6.1.4.1.6574.1.1.1",
							Type:  gosnmp.Integer,
							Value: 1,
						},
						{
							Name:  "1.3.6.1.4.1.6574.1.2.1",
							Type:  gosnmp.OpaqueFloat,
							Value: float32(65.5),
						},
						// Row 2
						{
							Name:  "1.3.6.1.4.1.6574.1.1.2",
							Type:  gosnmp.Integer,
							Value: 2,
						},
						{
							Name:  "1.3.6.1.4.1.6574.1.2.2",
							Type:  gosnmp.OpaqueFloat,
							Value: float32(71.25),
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
							Value:      65, // Truncated from 65.5
							Tags:       map[string]string{"sensor": "1"},
							MetricType: "gauge",
							IsTable:    true,
						},
						{
							Name:       "temperature",
							Value:      71, // Truncated from 71.25
							Tags:       map[string]string{"sensor": "2"},
							MetricType: "gauge",
							IsTable:    true,
						},
					},
				},
			},
			expectedError: false,
		},
		"OpaqueFloat with unexpected type": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.4.2.12.1.0",
									Name: "temperature",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.2.12.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.2.12.1.0",
								Type:  gosnmp.OpaqueFloat,
								Value: "29.5", // Wrong type - should be float32
							},
						},
					}, nil,
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "OpaqueFloat has unexpected type",
		},
		"OpaqueFloat with negative value": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.4.2.12.1.0",
									Name: "temperature",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.2.12.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.2.12.1.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(-15.5),
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
							Value:      -15, // Truncated from -15.5
							MetricType: "gauge",
						},
					},
				},
			},
			expectedError: false,
		},
		"OpaqueFloat with very large value": {
			profiles: []*ddsnmp.Profile{
				{
					SourceFile: "test-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.6574.4.4.2.2.0",
									Name: "power",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.6574.4.4.2.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.6574.4.4.2.2.0",
								Type:  gosnmp.OpaqueFloat,
								Value: float32(1234567.89),
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
							Name:       "power",
							Value:      1234567, // Truncated from 1234567.89
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
			collector.tableCache.setTTL(0, 0)

			result, err := collector.Collect()

			// The Metric struct has a Profile field that contains a pointer to ProfileMetrics,
			// which itself contains the Metrics slice.
			// This creates a circular reference that makes ElementsMatch fail.
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
					assert.ElementsMatch(t, tc.expectedResult[i].Metrics, result[i].Metrics)
				}
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

//func TestCollector_TableCache(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockHandler := snmpmock.NewMockHandler(ctrl)
//	mockHandler.EXPECT().MaxOids().Return(10).AnyTimes()
//	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
//
//	// First collection - expect table walk
//	mockHandler.EXPECT().BulkWalkAll("1.3.6.1.2.1.2.2").Return(
//		[]gosnmp.SnmpPDU{
//			// Row 1
//			{Name: "1.3.6.1.2.1.2.2.1.2.1", Type: gosnmp.OctetString, Value: []byte("eth0")},
//			{Name: "1.3.6.1.2.1.2.2.1.3.1", Type: gosnmp.Integer, Value: 6},
//			{Name: "1.3.6.1.2.1.2.2.1.8.1", Type: gosnmp.Integer, Value: 1},
//			{Name: "1.3.6.1.2.1.2.2.1.10.1", Type: gosnmp.Counter32, Value: uint(1000)},
//			{Name: "1.3.6.1.2.1.2.2.1.16.1", Type: gosnmp.Counter32, Value: uint(2000)},
//			// Row 2
//			{Name: "1.3.6.1.2.1.2.2.1.2.2", Type: gosnmp.OctetString, Value: []byte("lo0")},
//			{Name: "1.3.6.1.2.1.2.2.1.3.2", Type: gosnmp.Integer, Value: 1},
//			{Name: "1.3.6.1.2.1.2.2.1.8.2", Type: gosnmp.Integer, Value: 1},
//			{Name: "1.3.6.1.2.1.2.2.1.10.2", Type: gosnmp.Counter32, Value: uint(500)},
//			{Name: "1.3.6.1.2.1.2.2.1.16.2", Type: gosnmp.Counter32, Value: uint(500)},
//		}, nil,
//	).Times(1) // Walk only once
//
//	// Second collection - expect GETs for each config
//	// First config GETs its columns
//	mockHandler.EXPECT().Get(gomock.InAnyOrder([]string{
//		"1.3.6.1.2.1.2.2.1.10.1", // ifInOctets.1
//		"1.3.6.1.2.1.2.2.1.2.1",  // ifDescr.1 (tag)
//		"1.3.6.1.2.1.2.2.1.10.2", // ifInOctets.2
//		"1.3.6.1.2.1.2.2.1.2.2",  // ifDescr.2 (tag)
//	})).Return(
//		&gosnmp.SnmpPacket{
//			Variables: []gosnmp.SnmpPDU{
//				{Name: "1.3.6.1.2.1.2.2.1.10.1", Type: gosnmp.Counter32, Value: uint(1500)},
//				{Name: "1.3.6.1.2.1.2.2.1.2.1", Type: gosnmp.OctetString, Value: []byte("eth0")},
//				{Name: "1.3.6.1.2.1.2.2.1.10.2", Type: gosnmp.Counter32, Value: uint(600)},
//				{Name: "1.3.6.1.2.1.2.2.1.2.2", Type: gosnmp.OctetString, Value: []byte("lo0")},
//			},
//		}, nil,
//	).Times(1)
//
//	// Second config GETs its columns
//	mockHandler.EXPECT().Get(gomock.InAnyOrder([]string{
//		"1.3.6.1.2.1.2.2.1.16.1", // ifOutOctets.1
//		"1.3.6.1.2.1.2.2.1.3.1",  // ifType.1 (tag)
//		"1.3.6.1.2.1.2.2.1.16.2", // ifOutOctets.2
//		"1.3.6.1.2.1.2.2.1.3.2",  // ifType.2 (tag)
//	})).Return(
//		&gosnmp.SnmpPacket{
//			Variables: []gosnmp.SnmpPDU{
//				{Name: "1.3.6.1.2.1.2.2.1.16.1", Type: gosnmp.Counter32, Value: uint(2500)},
//				{Name: "1.3.6.1.2.1.2.2.1.3.1", Type: gosnmp.Integer, Value: 6},
//				{Name: "1.3.6.1.2.1.2.2.1.16.2", Type: gosnmp.Counter32, Value: uint(600)},
//				{Name: "1.3.6.1.2.1.2.2.1.3.2", Type: gosnmp.Integer, Value: 1},
//			},
//		}, nil,
//	).Times(1)
//
//	// Third config GETs its columns
//	mockHandler.EXPECT().Get(gomock.InAnyOrder([]string{
//		"1.3.6.1.2.1.2.2.1.8.1", // ifOperStatus.1
//		"1.3.6.1.2.1.2.2.1.8.2", // ifOperStatus.2
//	})).Return(
//		&gosnmp.SnmpPacket{
//			Variables: []gosnmp.SnmpPDU{
//				{Name: "1.3.6.1.2.1.2.2.1.8.1", Type: gosnmp.Integer, Value: 1},
//				{Name: "1.3.6.1.2.1.2.2.1.8.2", Type: gosnmp.Integer, Value: 1},
//			},
//		}, nil,
//	).Times(1)
//
//	// Create profile with multiple configs for same table
//	profile := &ddsnmp.Profile{
//		SourceFile: "test-profile.yaml",
//		Definition: &ddprofiledefinition.ProfileDefinition{
//			Metrics: []ddprofiledefinition.MetricsConfig{
//				{
//					MIB: "IF-MIB",
//					Table: ddprofiledefinition.SymbolConfig{
//						OID:  "1.3.6.1.2.1.2.2",
//						Name: "ifTable",
//					},
//					Symbols: []ddprofiledefinition.SymbolConfig{
//						{
//							OID:  "1.3.6.1.2.1.2.2.1.10",
//							Name: "ifInOctets",
//						},
//					},
//					MetricTags: []ddprofiledefinition.MetricTagConfig{
//						{
//							Tag: "interface",
//							Symbol: ddprofiledefinition.SymbolConfigCompat{
//								OID:  "1.3.6.1.2.1.2.2.1.2",
//								Name: "ifDescr",
//							},
//						},
//					},
//				},
//				{
//					MIB: "IF-MIB",
//					Table: ddprofiledefinition.SymbolConfig{
//						OID:  "1.3.6.1.2.1.2.2",
//						Name: "ifTable",
//					},
//					Symbols: []ddprofiledefinition.SymbolConfig{
//						{
//							OID:  "1.3.6.1.2.1.2.2.1.16",
//							Name: "ifOutOctets",
//						},
//					},
//					MetricTags: []ddprofiledefinition.MetricTagConfig{
//						{
//							Tag: "if_type",
//							Symbol: ddprofiledefinition.SymbolConfigCompat{
//								OID:  "1.3.6.1.2.1.2.2.1.3",
//								Name: "ifType",
//							},
//							Mapping: map[string]string{
//								"6": "ethernet",
//								"1": "other",
//							},
//						},
//					},
//				},
//				{
//					MIB: "IF-MIB",
//					Table: ddprofiledefinition.SymbolConfig{
//						OID:  "1.3.6.1.2.1.2.2",
//						Name: "ifTable",
//					},
//					Symbols: []ddprofiledefinition.SymbolConfig{
//						{
//							OID:  "1.3.6.1.2.1.2.2.1.8",
//							Name: "ifOperStatus",
//							Mapping: map[string]string{
//								"1": "up",
//								"2": "down",
//							},
//						},
//					},
//					StaticTags: []string{"source:cache_test"},
//				},
//			},
//		},
//	}
//
//	// Create collector with cache enabled
//	collector := New(mockHandler, []*ddsnmp.Profile{profile}, logger.New())
//	collector.doTableMetrics = true
//	collector.tableCache.setTTL(5*time.Minute, 0.1)
//
//	// First collection - should walk tables
//	result1, err := collector.Collect()
//	require.NoError(t, err)
//	require.Len(t, result1, 1)
//	require.Len(t, result1[0].Metrics, 6) // 2 rows x 3 metrics
//
//	// Verify first collection metrics
//	metrics1 := result1[0].Metrics
//	assert.Contains(t, metrics1, Metric{
//		Name:       "ifInOctets",
//		Value:      1000,
//		Tags:       map[string]string{"interface": "eth0"},
//		MetricType: "rate",
//		IsTable:    true,
//	})
//
//	// Second collection - should use cache
//	result2, err := collector.Collect()
//	require.NoError(t, err)
//	require.Len(t, result2, 1)
//	require.Len(t, result2[0].Metrics, 6) // 2 rows x 3 metrics
//
//	// Verify second collection has updated values from cache
//	metrics2 := result2[0].Metrics
//	var foundUpdatedMetric bool
//	for _, m := range metrics2 {
//		if m.Name == "ifInOctets" && m.Tags["interface"] == "eth0" {
//			assert.Equal(t, int64(1500), m.Value) // Updated value from cache
//			foundUpdatedMetric = true
//		}
//	}
//	assert.True(t, foundUpdatedMetric, "Should find updated metric from cache")
//
//	// Verify all expectations were met
//	ctrl.Finish()
//}

func mustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}
