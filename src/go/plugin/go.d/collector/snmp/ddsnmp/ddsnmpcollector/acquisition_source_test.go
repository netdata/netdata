// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

type sourceTestHandler struct {
	gosnmp.Handler
	get         func([]string) (*gosnmp.SnmpPacket, error)
	walk        func(string) ([]gosnmp.SnmpPDU, error)
	gets, walks int
}

func (*sourceTestHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (*sourceTestHandler) MaxOids() int                { return 1 }
func (h *sourceTestHandler) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	h.gets++
	return h.get(oids)
}
func (h *sourceTestHandler) BulkWalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	h.walks++
	return h.walk(oid)
}
func (h *sourceTestHandler) WalkAll(oid string) ([]gosnmp.SnmpPDU, error) { return h.BulkWalkAll(oid) }

func TestSourceValuesPreserveDecodedData(t *testing.T) {
	for name, tc := range map[string]struct {
		value any
		want  ddsnmp.SourceValue
	}{
		"null":              {nil, ddsnmp.SourceValue{Kind: "null"}},
		"nil bytes":         {[]byte(nil), ddsnmp.SourceValue{Kind: "bytes_nil"}},
		"empty bytes":       {[]byte{}, ddsnmp.SourceValue{Kind: "bytes", Bytes: []byte{}}},
		"binary":            {[]byte{0, 255}, ddsnmp.SourceValue{Kind: "bytes", Bytes: []byte{0, 255}}},
		"non UTF8 string":   {"\xff", ddsnmp.SourceValue{Kind: "string_bytes", Bytes: []byte{255}}},
		"text":              {"device", ddsnmp.SourceValue{Kind: "string", Text: "device"}},
		"signed minimum":    {int64(math.MinInt64), ddsnmp.SourceValue{Kind: "signed", Text: "-9223372036854775808"}},
		"unsigned maximum":  {uint64(math.MaxUint64), ddsnmp.SourceValue{Kind: "unsigned", Text: "18446744073709551615"}},
		"NaN payload":       {math.Float64frombits(0x7ff8000000000042), ddsnmp.SourceValue{Kind: "float64", Text: "7ff8000000000042"}},
		"negative zero":     {math.Float32frombits(0x80000000), ddsnmp.SourceValue{Kind: "float32", Text: "80000000"}},
		"unsupported value": {make(chan int), ddsnmp.SourceValue{Kind: "unavailable"}},
	} {
		t.Run(name, func(t *testing.T) {
			pdu := gosnmp.SnmpPDU{Name: "1.2.3.0", Type: gosnmp.OctetString, Value: tc.value}
			handler := &sourceTestHandler{get: func([]string) (*gosnmp.SnmpPacket, error) {
				return &gosnmp.SnmpPacket{Community: "never retain this", Variables: []gosnmp.SnmpPDU{pdu}}, nil
			}}
			source := &SourceRecorder{}
			packet, err := source.Wrap(handler).Get([]string{"1.2.3.0"})
			require.NoError(t, err)
			evidence := source.Finish()
			require.Equal(t, tc.want, evidence[0].PDUs[0].Value)
			if b, ok := packet.Variables[0].Value.([]byte); ok && len(b) > 0 {
				b[0] ^= 255
				require.Equal(t, tc.want, evidence[0].PDUs[0].Value)
			}
			encoded, err := json.Marshal(evidence)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "never retain this")
			var restored []ddsnmp.SourceOperation
			require.NoError(t, json.Unmarshal(encoded, &restored))
			require.NoError(t, ddsnmp.ValidateSourceOperations(restored))
			require.Equal(t, tc.want.Kind, restored[0].PDUs[0].Value.Kind)
			require.Equal(t, tc.want.Text, restored[0].PDUs[0].Value.Text)
			require.True(t, bytes.Equal(tc.want.Bytes, restored[0].PDUs[0].Value.Bytes))
		})
	}
}

func TestSourceEvidenceFollowsActualCollection(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.1"
	table := func(kind ddsnmp.TopologyKind) ddprofiledefinition.TopologyConfig {
		return ddprofiledefinition.TopologyConfig{Kind: kind, MetricsConfig: ddprofiledefinition.MetricsConfig{
			Table: ddprofiledefinition.SymbolConfig{OID: root, Name: "interfaces"}, Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "interface"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{Tag: "if_name", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".1"}}},
		}}
	}
	for name, tc := range map[string]struct {
		profile                            *ddprofiledefinition.ProfileDefinition
		getFailure, walkFailure, malformed bool
		calls, bindings                    int
		wantErr                            bool
	}{
		"earlier GET batch and exceptional PDUs survive failure": {profile: &ddprofiledefinition.ProfileDefinition{MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
			{MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "first", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".0"}}},
			{MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "second", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".2"}}},
		}}, getFailure: true, calls: 2, bindings: 2, wantErr: true},
		"partial WALK survives failure":                  {profile: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{table(ddsnmp.KindIfName)}}, walkFailure: true, calls: 1, bindings: 1, wantErr: true},
		"shared WALK has one response and two consumers": {profile: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{table(ddsnmp.KindIfName), table(ddsnmp.KindBridgePortIfIndex)}}, calls: 1, bindings: 2},
		"pattern omission leaves successful row":         {profile: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{table(ddsnmp.KindIfName)}}, malformed: true, calls: 1, bindings: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.malformed {
				tc.profile.Topology[0].MetricTags[0].Symbol.MatchPatternCompiled = regexp.MustCompile("^does-not-match$")
			}
			handler := &sourceTestHandler{}
			handler.get = func(oids []string) (*gosnmp.SnmpPacket, error) {
				packet := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.OctetString, Value: []byte("first")}, {Name: oids[0], Type: gosnmp.OctetString, Value: []byte("last")}, {Name: root + ".99", Type: gosnmp.NoSuchInstance}}}
				if tc.getFailure && handler.gets == 2 {
					return packet, context.DeadlineExceeded
				}
				return packet, nil
			}
			handler.walk = func(string) ([]gosnmp.SnmpPDU, error) {
				pdus := []gosnmp.SnmpPDU{{Name: root + ".1.7", Type: gosnmp.OctetString, Value: []byte("eth0")}}
				if tc.walkFailure {
					return pdus, context.DeadlineExceeded
				}
				return pdus, nil
			}
			source := &SourceRecorder{}
			var reports []AcquisitionProfileReport
			collector := New(Config{SnmpClient: source.Wrap(handler), Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: tc.profile}}, Log: logger.New(), InitialAcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { reports = append(reports, r) })})
			values, err := collector.Collect()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, values)
			}
			operations := source.Finish()
			require.Len(t, operations, tc.calls)
			require.Equal(t, tc.calls, handler.gets+handler.walks)
			require.NoError(t, ddsnmp.ValidateSourceOperations(operations))
			bindings := 0
			var events []ddsnmp.ProcessingEvent
			requests := ddsnmp.SourceRequestIndex(operations)
			for _, report := range reports {
				for _, route := range report.Routes {
					bindings += len(route.Sources)
					events = append(events, route.Processing...)
					require.NoError(t, ddsnmp.ValidateSourceBindings(route.Sources, requests))
				}
			}
			require.Equal(t, tc.bindings, bindings)
			if tc.getFailure {
				require.Len(t, operations[0].PDUs, 3)
				require.Len(t, operations[1].PDUs, 3)
				require.Equal(t, "deadline", operations[1].Failure.Reason)
			}
			if tc.walkFailure {
				require.Len(t, operations[0].PDUs, 1)
				require.Equal(t, "deadline", operations[0].Failure.Reason)
			}
			if tc.malformed {
				require.Contains(t, events, ddsnmp.ProcessingEvent{RowIndex: "7", Field: "if_name", OID: root + ".1.7", Reason: "pattern_mismatch"})
			}
		})
	}
}

func testWalkSources(t *testing.T, collector *Collector, execution *AcquisitionExecutionReport) []ddsnmp.SourceOperation {
	t.Helper()
	require.NotNil(t, execution)
	var result []ddsnmp.SourceOperation
	for _, id := range execution.WalkOperations {
		r := sourceRecorder(collector.tableCollector.snmpClient)
		require.NotNil(t, r)
		require.Greater(t, id, uint64(0))
		require.LessOrEqual(t, id, uint64(len(r.operations)))
		result = append(result, r.operations[id-1])
	}
	return result
}

func TestSourceRecorderLifecycleAndClientReplacement(t *testing.T) {
	for name, tc := range map[string]struct{ replace bool }{
		"initial client":     {},
		"replacement client": {replace: true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := scalarBGPTestConfig()
			handler := &sourceTestHandler{get: func(oids []string) (*gosnmp.SnmpPacket, error) {
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createGauge32PDU(oids[0], 6)}}, nil
			}}
			source := &SourceRecorder{}
			client := source.Wrap(handler)
			collector := New(Config{SnmpClient: client, Log: logger.New(), Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{cfg}}}}, InitialAcquisitionObserver: AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {})})
			if tc.replace {
				collector.SetSNMPClient(client)
			}
			_, err := collector.Collect()
			require.NoError(t, err)
			evidence := source.Finish()
			require.NotEmpty(t, evidence)
			require.Nil(t, source.operations, "ownership transfers to the completed context")
			require.Nil(t, source.Finish(), "transfer occurs once")
			before, err := json.Marshal(evidence)
			require.NoError(t, err)
			_, err = collector.Collect()
			require.NoError(t, err)
			require.Nil(t, source.operations, "a completed recorder must not accumulate polling history")
			after, err := json.Marshal(evidence)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func BenchmarkCollectorSourceEvidence(b *testing.B) {
	const root = "1.3.6.1.4.1.99999.1"
	for _, rows := range []int{4096, 100000} {
		pdus := make([]gosnmp.SnmpPDU, rows)
		for i := range pdus {
			pdus[i] = createGauge32PDU(fmt.Sprintf("%s.1.%d", root, i), uint(i))
		}
		profile := &ddsnmp.Profile{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{Kind: ddsnmp.KindIfName, MetricsConfig: ddprofiledefinition.MetricsConfig{Table: ddprofiledefinition.SymbolConfig{OID: root, Name: "table"}, Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}}}}}}}
		for name, tc := range map[string]struct{ capture bool }{"disabled": {}, "enabled": {capture: true}} {
			b.Run(fmt.Sprintf("rows=%d/%s", rows, name), func(b *testing.B) {
				log := logger.New()
				b.ReportAllocs()
				for b.Loop() {
					var client gosnmp.Handler = &sourceTestHandler{walk: func(string) ([]gosnmp.SnmpPDU, error) { return pdus, nil }}
					source := &SourceRecorder{}
					var observer AcquisitionObserver
					if tc.capture {
						client = source.Wrap(client)
						observer = AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {})
					}
					collector := New(Config{SnmpClient: client, Log: log, Profiles: []*ddsnmp.Profile{profile}, InitialAcquisitionObserver: observer})
					results, err := collector.Collect()
					if err != nil {
						b.Fatal(err)
					}
					evidence := source.Finish()
					runtime.KeepAlive(results)
					runtime.KeepAlive(evidence)
				}
			})
		}
	}
}
