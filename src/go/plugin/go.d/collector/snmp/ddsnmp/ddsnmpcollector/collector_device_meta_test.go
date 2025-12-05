// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestDeviceMetadataCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		profile        *ddsnmp.Profile
		setupMock      func(m *snmpmock.MockHandler)
		sysobjectid    string
		expectedResult map[string]ddsnmp.MetaTag
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
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "dell"},
								"type":   {Value: "router"},
								"model":  {Value: "PowerEdge R740"},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "dell", IsExactMatch: false},
				"type":   {Value: "router", IsExactMatch: false},
				"model":  {Value: "PowerEdge R740", IsExactMatch: false},
			},
			expectedError: false,
		},
		"dynamic values with single symbol": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "dell"},
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
							{Name: "1.3.6.1.4.1.674.10892.5.1.3.2.0", Type: gosnmp.OctetString, Value: []byte("ABC123XYZ")},
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Dell EMC Networking OS10")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor":        {Value: "dell", IsExactMatch: false},
				"serial_number": {Value: "ABC123XYZ", IsExactMatch: false},
				"version":       {Value: "Dell EMC Networking OS10", IsExactMatch: false},
			},
			expectedError: false,
		},
		"multiple symbols fallback": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
								"serial_number": {
									Symbols: []ddprofiledefinition.SymbolConfig{
										{OID: "1.3.6.1.4.1.674.10892.5.1.3.2.0", Name: "chassisSerialNumber"},
										{OID: "1.3.6.1.4.1.674.10892.5.1.3.3.0", Name: "backupSerialNumber"},
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
							{Name: "1.3.6.1.4.1.674.10892.5.1.3.2.0", Type: gosnmp.NoSuchObject, Value: nil},
							{Name: "1.3.6.1.4.1.674.10892.5.1.3.3.0", Type: gosnmp.OctetString, Value: []byte("BACKUP123")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"serial_number": {Value: "BACKUP123", IsExactMatch: false},
			},
			expectedError: false,
		},
		"value with match_pattern": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
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
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("device-name-3 263829375 Isilon OneFS v8.2.0.0")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"version": {Value: "8.2.0.0", IsExactMatch: false},
			},
			expectedError: false,
		},
		"value with extract_value": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
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
							{Name: "1.3.6.1.4.1.674.10892.5.4.200.10.1.2.1.3.1", Type: gosnmp.OctetString, Value: []byte("25C")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"temperature": {Value: "25", IsExactMatch: false},
			},
			expectedError: false,
		},
		"value with mapping": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
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
							{Name: "1.3.6.1.4.1.674.10892.5.2.1.0", Type: gosnmp.Integer, Value: 3},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"status": {Value: "ok", IsExactMatch: false},
			},
			expectedError: false,
		},
		"format mac_address": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
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
							{Name: "1.3.6.1.2.1.2.2.1.6.1", Type: gosnmp.OctetString, Value: []byte{0x00, 0x50, 0x56, 0xAB, 0xCD, 0xEF}},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"mac_address": {Value: "00:50:56:AB:CD:EF", IsExactMatch: false},
			},
			expectedError: false,
		},
		"non-device resource ignored": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"interface": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
								"name": {Value: "eth0"},
							},
						},
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "cisco"},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "cisco", IsExactMatch: false},
			},
			expectedError: false,
		},
		"SNMP error": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
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
				m.EXPECT().Get([]string{"1.3.6.1.4.1.674.10892.5.1.3.2.0"}).Return(nil, errors.New("SNMP timeout"))
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
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "dell"},
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
							{Name: "1.3.6.1.4.1.674.10892.5.1.3.2.0", Type: gosnmp.NoSuchObject, Value: nil},
							{Name: "1.3.6.1.4.1.674.10892.5.1.3.1.0", Type: gosnmp.OctetString, Value: []byte("PowerEdge R740")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "dell", IsExactMatch: false},
				"model":  {Value: "PowerEdge R740", IsExactMatch: false},
			},
			expectedError: false,
		},
		"chunked requests": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": ddprofiledefinition.MetadataResourceConfig{
							Fields: map[string]ddprofiledefinition.MetadataField{
								"field1": {Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.1.0", Name: "oid1"}},
								"field2": {Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.2.0", Name: "oid2"}},
								"field3": {Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "oid3"}},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(2).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.2.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("value1")},
							{Name: "1.3.6.1.2.1.1.2.0", Type: gosnmp.OctetString, Value: []byte("value2")},
						},
					}, nil,
				)
				m.EXPECT().Get([]string{"1.3.6.1.2.1.1.3.0"}).Return(
					&gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{
							{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.OctetString, Value: []byte("value3")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"field1": {Value: "value1", IsExactMatch: false},
				"field2": {Value: "value2", IsExactMatch: false},
				"field3": {Value: "value3", IsExactMatch: false},
			},
			expectedError: false,
		},
		"sysobjectid metadata override - single match": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco"},
								"type":   {Value: "Firewall"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"model":  {Value: "ASA5510"},
								"series": {Value: "ASA5500"},
							},
						},
						{
							SysobjectID: "1.3.6.1.4.1.9.1.670",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"model": {Value: "ASA5520"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco", IsExactMatch: false},
				"type":   {Value: "Firewall", IsExactMatch: false},
				"model":  {Value: "ASA5510", IsExactMatch: true},
				"series": {Value: "ASA5500", IsExactMatch: true},
			},
			expectedError: false,
		},
		"sysobjectid metadata override - multiple matches cascade": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor":   {Value: "Cisco"},
								"platform": {Value: "Default Platform"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.*",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"platform": {Value: "Enterprise"},
								"support":  {Value: "Premium"},
							},
						},
						{
							SysobjectID: "1.3.6.1.4.1.9.1.*",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"type":    {Value: "Firewall"},
								"series":  {Value: "ASA"},
								"support": {Value: "Standard"},
							},
						},
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"model":  {Value: "ASA5510"},
								"series": {Value: "ASA5500"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor":   {Value: "Cisco", IsExactMatch: false},      // base
				"platform": {Value: "Enterprise", IsExactMatch: false}, // wildcard match
				"support":  {Value: "Premium", IsExactMatch: false},    // first wildcard wins
				"type":     {Value: "Firewall", IsExactMatch: false},   // second wildcard
				"series":   {Value: "ASA5500", IsExactMatch: true},     // exact match entry
				"model":    {Value: "ASA5510", IsExactMatch: true},     // exact match entry
			},
			expectedError: false,
		},
		"sysobjectid metadata with dynamic SNMP values": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {Fields: map[string]ddprofiledefinition.MetadataField{"vendor": {Value: "Cisco"}}},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"model": {Value: "ASA5510"},
								"firmware": {Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.109.1.1.1.1.3.1",
									Name: "ciscoImageVersion",
								}},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.9.9.109.1.1.1.1.3.1"}).Return(
					&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
						{Name: "1.3.6.1.4.1.9.9.109.1.1.1.1.3.1", Type: gosnmp.OctetString, Value: []byte("9.2(4)")},
					}}, nil,
				)
			},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor":   {Value: "Cisco", IsExactMatch: false},
				"model":    {Value: "ASA5510", IsExactMatch: true},
				"firmware": {Value: "9.2(4)", IsExactMatch: true},
			},
			expectedError: false,
		},
		"sysobjectid metadata no match": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco"},
								"type":   {Value: "Firewall"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata:    map[string]ddprofiledefinition.MetadataField{"model": {Value: "ASA5510"}},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.700",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco", IsExactMatch: false},
				"type":   {Value: "Firewall", IsExactMatch: false},
			},
			expectedError: false,
		},
		"sysobjectid metadata with invalid regex pattern": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {Fields: map[string]ddprofiledefinition.MetadataField{"vendor": {Value: "Cisco"}}},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.[invalid",
							Metadata:    map[string]ddprofiledefinition.MetadataField{"model": {Value: "ASA5510"}},
						},
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata:    map[string]ddprofiledefinition.MetadataField{"series": {Value: "ASA5500"}},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco", IsExactMatch: false},
				"series": {Value: "ASA5500", IsExactMatch: true},
			},
			expectedError: false,
		},
		"sysobjectid metadata overrides base metadata field": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Generic"},
								"type":   {Value: "Unknown"},
								"model":  {Value: "Generic Model"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco Systems"},
								"model":  {Value: "ASA5510"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco Systems", IsExactMatch: true},
				"type":   {Value: "Unknown", IsExactMatch: false},
				"model":  {Value: "ASA5510", IsExactMatch: true},
			},
			expectedError: false,
		},
		"sysobjectid metadata with SNMP fetch error continues": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {Fields: map[string]ddprofiledefinition.MetadataField{"vendor": {Value: "Cisco"}}},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.669",
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"model": {Value: "ASA5510"},
								"firmware": {Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.109.1.1.1.1.3.1",
									Name: "ciscoImageVersion",
								}},
							},
						},
					},
				},
			},
			setupMock: func(m *snmpmock.MockHandler) {
				m.EXPECT().MaxOids().Return(10).AnyTimes()
				m.EXPECT().Get([]string{"1.3.6.1.4.1.9.9.109.1.1.1.1.3.1"}).Return(nil, errors.New("SNMP timeout"))
			},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco", IsExactMatch: false},
				"model":  {Value: "ASA5510", IsExactMatch: true},
			},
			expectedError: false,
		},
		"os_name with multiple symbols and match_pattern fallback": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco"},
								"os_name": {
									Symbols: []ddprofiledefinition.SymbolConfig{
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco Internetwork Operating System Software`), MatchValue: "IOS"},
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco IOS Software`), MatchValue: "IOS"},
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco NX-OS`), MatchValue: "NXOS"},
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco IOS XR`), MatchValue: "IOSXR"},
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
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Cisco NX-OS(tm) m9100, Software (m9100-s2ek9-mz), Version 4.1(1c), RELEASE SOFTWARE Copyright (c) 2002-2008 by Cisco Systems, Inc. Compiled 11/24/2008 18:00:00")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor":  {Value: "Cisco", IsExactMatch: false},
				"os_name": {Value: "NXOS", IsExactMatch: false},
			},
			expectedError: false,
		},
		"os_name with multiple symbols - no match fallback": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Juniper"},
								"os_name": {
									Symbols: []ddprofiledefinition.SymbolConfig{
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco IOS Software`), MatchValue: "IOS"},
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco NX-OS`), MatchValue: "NXOS"},
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
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Juniper Networks, Inc. mx960 internet router, kernel JUNOS 12.3R3.4")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Juniper", IsExactMatch: false},
			},
			expectedError: false,
		},
		"os_name with single symbol and match_pattern - no match": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Generic"},
								"os_name": {
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:                  "1.3.6.1.2.1.1.1.0",
										Name:                 "sysDescr",
										MatchPatternCompiled: mustCompileRegex(`Cisco IOS Software`),
										MatchValue:           "IOS",
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
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Some other device description")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Generic", IsExactMatch: false},
			},
			expectedError: false,
		},
		"mixed fields with match_pattern and extract_value": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco"},
								"version": {Symbol: ddprofiledefinition.SymbolConfig{
									OID:                  "1.3.6.1.2.1.1.1.0",
									Name:                 "sysDescr",
									ExtractValueCompiled: mustCompileRegex(`Version\s+([a-zA-Z0-9.()\[\]]+)`),
								}},
								"model": {Symbol: ddprofiledefinition.SymbolConfig{
									OID:                  "1.3.6.1.2.1.1.1.0",
									Name:                 "sysDescr",
									ExtractValueCompiled: mustCompileRegex(`Software\s+\(([-a-zA-Z0-9_ ]+)\)`),
								}},
								"os_name": {
									Symbols: []ddprofiledefinition.SymbolConfig{
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco IOS Software`), MatchValue: "IOS"},
										{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr", MatchPatternCompiled: mustCompileRegex(`Cisco IOS XR Software`), MatchValue: "IOSXR"},
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
							{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Cisco IOS XR Software (Cisco ASR9K Series), Version 4.2.3[Default] Copyright (c) 2013 by Cisco Systems, Inc.")},
						},
					}, nil,
				)
			},
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor":  {Value: "Cisco", IsExactMatch: false},
				"version": {Value: "4.2.3[Default]", IsExactMatch: false},
				"model":   {Value: "Cisco ASR9K Series", IsExactMatch: false},
				"os_name": {Value: "IOSXR", IsExactMatch: false},
			},
			expectedError: false,
		},
		"base metadata exact via sysObjectIDs": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Selector: ddprofiledefinition.SelectorSpec{
						{
							SysObjectID: ddprofiledefinition.SelectorIncludeExclude{
								Include: []string{"1.3.6.1.4.1.9.1.669"},
							},
						},
					},
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"vendor": {Value: "Cisco"},
								"type":   {Value: "Firewall"},
								"model":  {Value: "Generic"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				"vendor": {Value: "Cisco", IsExactMatch: true},
				"type":   {Value: "Firewall", IsExactMatch: true},
				"model":  {Value: "Generic", IsExactMatch: true},
			},
			expectedError: false,
		},
		"sysObjectIDs exact + sysobjectid wildcard → base wins overlap": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Selector: ddprofiledefinition.SelectorSpec{
						{
							SysObjectID: ddprofiledefinition.SelectorIncludeExclude{
								Include: []string{"1.3.6.1.4.1.9.1.669"},
							},
						},
					},
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"platform": {Value: "Enterprise-Exact"},
								"series":   {Value: "ASA-Exact"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.*", // wildcard match (not exact)
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"platform": {Value: "Enterprise-Wildcard"},
								"series":   {Value: "ASA-Wildcard"},
								"model":    {Value: "ASA5510-Wildcard"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				// base exact overwrites wildcard for overlapping keys
				"platform": {Value: "Enterprise-Exact", IsExactMatch: true},
				"series":   {Value: "ASA-Exact", IsExactMatch: true},
				// non-overlapping key from wildcard remains
				"model": {Value: "ASA5510-Wildcard", IsExactMatch: false},
			},
			expectedError: false,
		},
		"non-exact + sysobjectid wildcard → wildcard wins": {
			profile: &ddsnmp.Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]ddprofiledefinition.MetadataField{
								"platform": {Value: "Enterprise-Base"},
								"series":   {Value: "ASA-Base"},
							},
						},
					},
					SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
						{
							SysobjectID: "1.3.6.1.4.1.9.1.*", // wildcard
							Metadata: map[string]ddprofiledefinition.MetadataField{
								"platform": {Value: "Enterprise-Wildcard"},
								"series":   {Value: "ASA-Wildcard"},
								"model":    {Value: "ASA5510"},
							},
						},
					},
				},
			},
			setupMock:   func(m *snmpmock.MockHandler) {},
			sysobjectid: "1.3.6.1.4.1.9.1.669",
			expectedResult: map[string]ddsnmp.MetaTag{
				// wildcard (not exact) is applied first and base (non-exact) cannot overwrite
				"platform": {Value: "Enterprise-Wildcard", IsExactMatch: false},
				"series":   {Value: "ASA-Wildcard", IsExactMatch: false},
				"model":    {Value: "ASA5510", IsExactMatch: false},
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
			collector := newDeviceMetadataCollector(mockHandler, missingOIDs, logger.New(), tc.sysobjectid)

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
