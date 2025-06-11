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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHandler := snmpmock.NewMockHandler(ctrl)
			tc.setupMock(mockHandler)

			collector := New(mockHandler, tc.profiles, logger.New())
			collector.doTableMetrics = true
			collector.tableCache.setTTL(0, 0)

			result, err := collector.Collect()

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

func mustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}
