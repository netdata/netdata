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
					StaticTags: []ddprofiledefinition.StaticMetricTagConfig{},
				},
			},
			setupMock:      func(m *snmpmock.MockHandler) {},
			expectedResult: nil,
			expectedError:  false,
		},
		"static tags only": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					StaticTags: []ddprofiledefinition.StaticMetricTagConfig{
						{Tag: "environment", Value: "production"},
						{Tag: "region", Value: "us-east-1"},
						{Tag: "service", Value: "network"},
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
					StaticTags: []ddprofiledefinition.StaticMetricTagConfig{
						{Tag: "environment", Value: "production"},
						{Tag: "managed", Value: "true"},
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

			result, err := collector.collect(tc.profile)

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
