// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"

	snmpmock "github.com/gosnmp/gosnmp/mocks"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestGlobalTagsCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profile        *ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult map[string]string
		expectedError  bool
		errorContains  string
	}{
		"no tags configured": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{},
					StaticTags: []string{},
				},
			},
			setupMock:      func(m *snmpmock.MockHandler) {},
			expectedResult: nil,
			expectedError:  false,
		},
		"static tags only": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					StaticTags: []string{
						"environment:production",
						"region:us-east-1",
						"service:network",
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {},
			expectedResult: map[string]string{
				"environment": "production",
				"region":      "us-east-1",
				"service":     "network",
			},
			expectedError: false,
		},
		"dynamic tags only": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "device_vendor",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.1.0",
								Name: "sysDescr",
							},
						},
						{
							Tag: "location",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.6.0",
								Name: "sysLocation",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.1.1.0",
					"1.3.6.1.2.1.1.6.0",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("Cisco IOS Software"),
							},
							{
								Name:  "1.3.6.1.2.1.1.6.0",
								Type:  gosnmp.OctetString,
								Value: []byte("DataCenter-1"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"device_vendor": "Cisco IOS Software",
				"location":      "DataCenter-1",
			},
			expectedError: false,
		},
		"mixed static and dynamic tags": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					StaticTags: []string{
						"environment:production",
						"managed:true",
					},
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "hostname",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.5.0",
								Type:  gosnmp.OctetString,
								Value: []byte("router-01"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"environment": "production",
				"managed":     "true",
				"hostname":    "router-01",
			},
			expectedError: false,
		},
		"tag with mapping": {
			profile: &ddsnmp.Profile{
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
								"1.3.6.1.4.1.9.1.3": "firewall",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.2.0",
								Type:  gosnmp.ObjectIdentifier,
								Value: "1.3.6.1.4.1.9.1.2",
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"device_type": "switch",
			},
			expectedError: false,
		},
		"tag with pattern matching": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
							Pattern: mustCompileRegex(`(.*)-(.*)-(.*)`),
							Tags: map[string]string{
								"site":     "$1",
								"role":     "$2",
								"unit_num": "$3",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.5.0",
								Type:  gosnmp.OctetString,
								Value: []byte("NYC-CORE-01"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"site":     "NYC",
				"role":     "CORE",
				"unit_num": "01",
			},
			expectedError: false,
		},
		"missing OID": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "hostname",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.5.0",
								Type:  gosnmp.NoSuchObject,
								Value: nil,
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{},
			expectedError:  false,
		},
		"SNMP error": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "hostname",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					nil,
					errors.New("SNMP timeout"),
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "failed to fetch global tag values",
		},
		"empty tag name falls back to symbol name": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "", // Empty tag name
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.5.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.5.0",
								Type:  gosnmp.OctetString,
								Value: []byte("router-01"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"sysName": "router-01",
			},
			expectedError: false,
		},
		"chunked requests": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "tag1",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.1.0",
								Name: "oid1",
							},
						},
						{
							Tag: "tag2",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.2.0",
								Name: "oid2",
							},
						},
						{
							Tag: "tag3",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.1.3.0",
								Name: "oid3",
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(2).AnyTimes() // Force chunking
				// First chunk
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value1"),
							},
							{
								Name:  "1.3.6.1.2.1.1.2.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value2"),
							},
						},
					}, nil,
				)
				// Second chunk
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value3"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"tag1": "value1",
				"tag2": "value2",
				"tag3": "value3",
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

			missingOIDs := make(map[string]bool)
			collector := newGlobalTagsCollector(mockHandler, missingOIDs, logger.New())

			result, err := collector.Collect(tc.profile)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestDeviceMetadataCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profile        *ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		expectedResult map[string]string
		expectedError  bool
		errorContains  string
	}{
		"no metadata configured": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{},
				},
			},
			setupMock:      func(m *snmpmock.MockHandler) {},
			expectedResult: nil,
			expectedError:  false,
		},
		"static values only": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"vendor": {
									Value: "dell",
								},
								"type": {
									Value: "router",
								},
								"model": {
									Value: "PowerEdge R740",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {},
			expectedResult: map[string]string{
				"vendor": "dell",
				"type":   "router",
				"model":  "PowerEdge R740",
			},
			expectedError: false,
		},
		"dynamic values with single symbol": {
			profile: &ddsnmp.Profile{
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
								"version": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.1.0",
										Name: "sysDescr",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.2.1.1.1.0",
					"1.3.6.1.4.1.674.10892.5.1.3.2.0",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
								Type:  gosnmp.OctetString,
								Value: []byte("ABC123XYZ"),
							},
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("Dell EMC Networking OS10"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"vendor":        "dell",
				"serial_number": "ABC123XYZ",
				"version":       "Dell EMC Networking OS10",
			},
			expectedError: false,
		},
		"multiple symbols fallback": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"serial_number": {
									Symbols: []ddprofiledefinition.SymbolConfig{
										{
											OID:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
											Name: "chassisSerialNumber",
										},
										{
											OID:  "1.3.6.1.4.1.674.10892.5.1.3.3.0",
											Name: "backupSerialNumber",
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
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.674.10892.5.1.3.2.0",
					"1.3.6.1.4.1.674.10892.5.1.3.3.0",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
								Type:  gosnmp.NoSuchObject,
								Value: nil,
							},
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.3.0",
								Type:  gosnmp.OctetString,
								Value: []byte("BACKUP123"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"serial_number": "BACKUP123",
			},
			expectedError: false,
		},
		"value with match_pattern": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"version": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:                  "1.3.6.1.2.1.1.1.0",
										Name:                 "sysDescr",
										MatchPatternCompiled: mustCompileRegex(`Isilon OneFS v(\S+)`),
										MatchValue:           "$1",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("device-name-3 263829375 Isilon OneFS v8.2.0.0"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"version": "8.2.0.0",
			},
			expectedError: false,
		},
		"value with extract_value": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"temperature": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:                  "1.3.6.1.4.1.674.10892.5.4.200.10.1.2.1.3.1",
										Name:                 "temperatureProbeReading",
										ExtractValueCompiled: mustCompileRegex(`(\d+)C`),
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.674.10892.5.4.200.10.1.2.1.3.1"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.4.200.10.1.2.1.3.1",
								Type:  gosnmp.OctetString,
								Value: []byte("25C"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"temperature": "25",
			},
			expectedError: false,
		},
		"value with mapping": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"status": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.4.1.674.10892.5.2.1.0",
										Name: "globalSystemStatus",
										Mapping: map[string]string{
											"1": "other",
											"2": "unknown",
											"3": "ok",
											"4": "nonCritical",
											"5": "critical",
											"6": "nonRecoverable",
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
				m.EXPECT().Get([]string{"1.3.6.1.4.1.674.10892.5.2.1.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.2.1.0",
								Type:  gosnmp.Integer,
								Value: 3,
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"status": "ok",
			},
			expectedError: false,
		},
		"format mac_address": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"mac_address": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:    "1.3.6.1.2.1.2.2.1.6.1",
										Name:   "ifPhysAddress",
										Format: "mac_address",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.2.2.1.6.1"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.2.2.1.6.1",
								Type:  gosnmp.OctetString,
								Value: []byte{0x00, 0x50, 0x56, 0xAB, 0xCD, 0xEF},
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"mac_address": "00:50:56:AB:CD:EF",
			},
			expectedError: false,
		},
		"non-device resource ignored": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"interface": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"name": {
									Value: "eth0",
								},
							},
						},
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"vendor": {
									Value: "cisco",
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {},
			expectedResult: map[string]string{
				"vendor": "cisco",
			},
			expectedError: false,
		},
		"SNMP error": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"serial_number": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
										Name: "chassisSerialNumber",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.674.10892.5.1.3.2.0"}).Return(
					nil,
					errors.New("SNMP timeout"),
				)
			},
			expectedResult: nil,
			expectedError:  true,
			errorContains:  "failed to fetch metadata values",
		},
		"missing OID continues with other fields": {
			profile: &ddsnmp.Profile{
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
								"model": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.4.1.674.10892.5.1.3.1.0",
										Name: "chassisModelName",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get(gomock.InAnyOrder([]string{
					"1.3.6.1.4.1.674.10892.5.1.3.1.0",
					"1.3.6.1.4.1.674.10892.5.1.3.2.0",
				})).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.2.0",
								Type:  gosnmp.NoSuchObject,
								Value: nil,
							},
							{
								Name:  "1.3.6.1.4.1.674.10892.5.1.3.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("PowerEdge R740"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"vendor": "dell",
				"model":  "PowerEdge R740",
			},
			expectedError: false,
		},
		"chunked requests": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: ddprofiledefinition.ListMap[ddprofiledefinition.MetadataField]{
								"field1": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.1.0",
										Name: "oid1",
									},
								},
								"field2": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.2.0",
										Name: "oid2",
									},
								},
								"field3": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.3.0",
										Name: "oid3",
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(2).AnyTimes() // Force chunking
				// First chunk
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.1.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value1"),
							},
							{
								Name:  "1.3.6.1.2.1.1.2.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value2"),
							},
						},
					}, nil,
				)
				// Second chunk
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{
								Name:  "1.3.6.1.2.1.1.3.0",
								Type:  gosnmp.OctetString,
								Value: []byte("value3"),
							},
						},
					}, nil,
				)
			},
			expectedResult: map[string]string{
				"field1": "value1",
				"field2": "value2",
				"field3": "value3",
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

			missingOIDs := make(map[string]bool)
			collector := newDeviceMetadataCollector(mockHandler, missingOIDs, logger.New())

			result, err := collector.Collect(tc.profile)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
