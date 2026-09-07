// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"regexp"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

func TestBGPProcessingEvidenceFollowsCollection(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.30.1"
	for name, tc := range map[string]struct {
		configure func(*ddprofiledefinition.BGPConfig)
		want      ddsnmp.ProcessingEvent
		rows      int
	}{
		"optional descriptor omission": {
			configure: func(c *ddprofiledefinition.BGPConfig) {
				c.Descriptors.Description.Symbol = ddprofiledefinition.SymbolConfig{OID: root + ".4", ExtractValueCompiled: regexp.MustCompile("^never(.)$")}
			},
			want: ddsnmp.ProcessingEvent{RowIndex: "42", Field: "descriptors.description", OID: root + ".4.42", Reason: "extract_mismatch"}, rows: 1,
		},
		"boolean conversion rejects row": {
			configure: func(c *ddprofiledefinition.BGPConfig) {
				c.Admin.Enabled = ddprofiledefinition.BGPValueConfig{Value: "invalid"}
			},
			want: ddsnmp.ProcessingEvent{RowIndex: "42", Field: "admin.enabled", Reason: "conversion"},
		},
		"raw enrichment omission preserves numeric value": {
			configure: func(c *ddprofiledefinition.BGPConfig) {
				c.Connection.EstablishedUptime.Symbol.ExtractValueCompiled = regexp.MustCompile("^never(.)$")
			},
			want: ddsnmp.ProcessingEvent{RowIndex: "42", Field: "connection.established_uptime", OID: root + ".4.42", Reason: "extract_mismatch", Stage: "raw_text"}, rows: 1,
		},
		"device state identifies exact field": {
			configure: func(c *ddprofiledefinition.BGPConfig) {
				c.Device.States.Idle = ddprofiledefinition.BGPValueConfig{Value: "invalid"}
			},
			want: ddsnmp.ProcessingEvent{RowIndex: "42", Field: "device_counts.states.idle", Reason: "conversion"},
		},
		"incomplete identity": {
			configure: func(c *ddprofiledefinition.BGPConfig) { c.Identity.Neighbor = ddprofiledefinition.BGPValueConfig{} },
			want:      ddsnmp.ProcessingEvent{RowIndex: "42", Field: "row", Reason: "incomplete_identity"},
		},
		"no signals": {
			configure: func(c *ddprofiledefinition.BGPConfig) {
				c.State = ddprofiledefinition.BGPStateConfig{}
				c.Connection = ddprofiledefinition.BGPConnectionConfig{}
			},
			want: ddsnmp.ProcessingEvent{RowIndex: "42", Field: "row", Reason: "no_signals"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := tableBGPTestConfig()
			tc.configure(&cfg)
			pdus := []gosnmp.SnmpPDU{createIntegerPDU(root+".2.42", 6), createGauge32PDU(root+".3.42", 65001), createGauge32PDU(root+".4.42", 7200)}
			handler := &sourceTestHandler{walk: func(string) ([]gosnmp.SnmpPDU, error) { return pdus, nil }}
			source := &SourceRecorder{}
			var report AcquisitionProfileReport
			collector := New(Config{SnmpClient: source.Wrap(handler), Log: logger.New(), Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{cfg}}}}, InitialAcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r })})
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].BGPRows, tc.rows)
			require.Len(t, report.Routes, 1)
			require.Contains(t, report.Routes[0].Processing, tc.want)
			require.NoError(t, ddsnmp.ValidateProcessingEvents(report.Routes[0].Processing))
			require.Len(t, source.Finish(), 1)
			require.Equal(t, 1, handler.walks)
		})
	}
}

func TestCrossTableProcessingEvidenceFollowsCollection(t *testing.T) {
	for name, tc := range map[string]struct {
		missing bool
		reason  string
	}{
		"pattern omission":             {reason: "pattern_mismatch"},
		"missing transformed instance": {missing: true, reason: "missing_dependency"},
	} {
		t.Run(name, func(t *testing.T) {
			const base = "1.3.6.1.4.1.99999.57"
			dependency, primary := crossTableDependencyTestConfigs(base)
			primary.MetricTags[0].Symbol.MatchPatternCompiled = regexp.MustCompile("^never$")
			profile := &ddsnmp.Profile{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{Kind: ddsnmp.KindArpEntry, MetricsConfig: primary}}}}
			ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
			handler := &sourceTestHandler{walk: func(oid string) ([]gosnmp.SnmpPDU, error) {
				if oid == primary.Table.OID {
					return []gosnmp.SnmpPDU{createGauge32PDU(primary.Symbols[0].OID+".7", 10)}, nil
				}
				index := ".7"
				if tc.missing {
					index = ".8"
				}
				return []gosnmp.SnmpPDU{createStringPDU(primary.MetricTags[0].Symbol.OID+index, "unmatched")}, nil
			}}
			source := &SourceRecorder{}
			var report AcquisitionProfileReport
			collector := New(Config{SnmpClient: source.Wrap(handler), Log: logger.New(), Profiles: []*ddsnmp.Profile{profile}, InitialAcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r })})
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].TopologyMetrics, 1)
			require.Empty(t, results[0].TopologyMetrics[0].Tags)
			var route *AcquisitionRouteReport
			for i := range report.Routes {
				if report.Routes[i].RootOID == primary.Table.OID {
					route = &report.Routes[i]
				}
			}
			require.NotNil(t, route)
			require.Contains(t, route.Processing, ddsnmp.ProcessingEvent{RowIndex: "7", Field: metricTagDisplayName(primary.MetricTags[0]), OID: primary.MetricTags[0].Symbol.OID + ".7", Reason: tc.reason})
			require.Len(t, route.Sources, 2)
			require.Equal(t, dependency.Table.Name, primary.MetricTags[0].Table)
			require.Len(t, source.Finish(), 2)
		})
	}
}

func TestMetadataProcessingEvidenceUsesLogicalField(t *testing.T) {
	const (
		oid         = "1.3.6.1.4.1.99999.60.1.0"
		fallbackOID = "1.3.6.1.4.1.99999.60.2.0"
	)
	for name, tc := range map[string]struct {
		symbolName string
		format     string
		missing    bool
		pattern    bool
		fallback   bool
		value      string
		wantReason string
		wantErr    bool
	}{
		"missing named symbol": {
			symbolName: "vendorSerialNumber", missing: true, wantReason: "missing_input",
		},
		"missing unnamed symbol": {
			missing: true, wantReason: "missing_input",
		},
		"invalid date conversion": {
			symbolName: "vendorDate", format: "text_date", value: "not a date", wantReason: "conversion", wantErr: true,
		},
		"empty date": {
			format: "snmp_dateandtime", wantReason: "empty_date",
		},
		"pattern omission": {
			symbolName: "vendorSerialNumber", pattern: true, value: "unmatched", wantReason: "pattern_mismatch",
		},
		"fallback symbol omission": {
			fallback: true, value: "unmatched", wantReason: "extract_mismatch",
		},
	} {
		t.Run(name, func(t *testing.T) {
			symbol := ddprofiledefinition.SymbolConfig{OID: oid, Name: tc.symbolName, Format: tc.format}
			if tc.pattern {
				symbol.MatchPatternCompiled = regexp.MustCompile("^never$")
			}
			field := ddprofiledefinition.MetadataField{Symbol: symbol}
			if tc.fallback {
				symbol.ExtractValueCompiled = regexp.MustCompile("^never(.)$")
				field = ddprofiledefinition.MetadataField{
					Symbols: []ddprofiledefinition.SymbolConfig{symbol, {OID: fallbackOID, Name: "fallbackSerialNumber"}},
				}
			}
			profile := &ddsnmp.Profile{
				SourceFile: "synthetic.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metadata: ddprofiledefinition.MetadataConfig{
						ddprofiledefinition.MetadataDeviceResource: {
							Fields: map[string]ddprofiledefinition.MetadataField{"serial_number": field},
						},
					},
				},
			}
			handler := &sourceTestHandler{get: func(oids []string) (*gosnmp.SnmpPacket, error) {
				pdu := createStringPDU(oids[0], tc.value)
				if oids[0] == fallbackOID {
					pdu = createStringPDU(fallbackOID, "fallback value")
				} else if tc.missing {
					pdu = createNoSuchObjectPDU(oid)
				}
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}}, nil
			}}
			var report AcquisitionProfileReport
			collector := New(Config{
				SnmpClient: new(SourceRecorder).Wrap(handler),
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
				InitialAcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
					report = r
				}),
			})
			results, err := collector.Collect()
			require.Equal(t, tc.wantErr, err != nil)
			require.Len(t, report.Routes, 1)
			require.Equal(t, []ddsnmp.ProcessingEvent{{Field: "serial_number", OID: oid, Reason: tc.wantReason}}, report.Routes[0].Processing)
			if tc.fallback {
				require.Len(t, results, 1)
				require.Equal(t, "fallback value", results[0].DeviceMetadata["serial_number"].Value)
			}
		})
	}
}
