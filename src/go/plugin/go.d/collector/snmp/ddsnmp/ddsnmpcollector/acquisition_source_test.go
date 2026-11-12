// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"runtime"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

type sourceTestHandler struct {
	snmputils.Client
	get         func([]string) (*gosnmp.Message, error)
	walk        func(string) ([]gosnmp.VarBind, error)
	gets, walks int
}

func (h *sourceTestHandler) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	h.gets++
	return h.get(oids)
}
func (h *sourceTestHandler) BulkWalkAll(ctx context.Context, oid string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	h.walks++
	values, err := h.walk(oid)
	return values, gosnmp.WalkEnd{}, err
}
func (h *sourceTestHandler) WalkAll(ctx context.Context, oid string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	return h.BulkWalkAll(context.Background(), oid, 0)
}

func TestSourceValuesPreserveDecodedData(t *testing.T) {
	for name, tc := range map[string]struct {
		value gosnmp.Value
		want  ddsnmp.SourceValue
	}{
		"null":             {gosnmp.NullValue(), ddsnmp.SourceValue{Kind: "null"}},
		"no such object":   {gosnmp.NoSuchObjectValue(), ddsnmp.SourceValue{Kind: "null"}},
		"nil bytes":        {gosnmp.OctetStringValue(nil), ddsnmp.SourceValue{Kind: "bytes_nil"}},
		"empty bytes":      {gosnmp.OctetStringValue([]byte{}), ddsnmp.SourceValue{Kind: "bytes", Bytes: []byte{}}},
		"binary":           {gosnmp.OctetStringValue([]byte{0, 255}), ddsnmp.SourceValue{Kind: "bytes", Bytes: []byte{0, 255}}},
		"opaque":           {gosnmp.OpaqueValue([]byte{1}), ddsnmp.SourceValue{Kind: "bytes", Bytes: []byte{1}}},
		"oid":              {gosnmp.ObjectIdentifierValue(".1.3.6"), ddsnmp.SourceValue{Kind: "string", Text: ".1.3.6"}},
		"address":          {gosnmp.IPAddressValue(netip.MustParseAddr("192.0.2.1")), ddsnmp.SourceValue{Kind: "string", Text: "192.0.2.1"}},
		"empty address":    {gosnmp.IPAddressValue(netip.Addr{}), ddsnmp.SourceValue{Kind: "null"}},
		"signed minimum":   {gosnmp.IntegerValue(math.MinInt32), ddsnmp.SourceValue{Kind: "signed", Text: "-2147483648"}},
		"unsigned maximum": {gosnmp.Counter64Value(math.MaxUint64), ddsnmp.SourceValue{Kind: "unsigned", Text: "18446744073709551615"}},
		"time ticks":       {gosnmp.TimeTicksValue(math.MaxUint32), ddsnmp.SourceValue{Kind: "unsigned", Text: "4294967295"}},
		"NaN payload":      {gosnmp.OpaqueDoubleValue(math.Float64frombits(0x7ff8000000000042)), ddsnmp.SourceValue{Kind: "float64", Text: "7ff8000000000042"}},
		"negative zero":    {gosnmp.OpaqueFloatValue(math.Float32frombits(0x80000000)), ddsnmp.SourceValue{Kind: "float32", Text: "80000000"}},
	} {
		t.Run(name, func(t *testing.T) {
			pdu := gosnmp.VarBind{Name: "1.2.3.0", Value: tc.value}
			handler := &sourceTestHandler{get: func([]string) (*gosnmp.Message, error) {
				return &gosnmp.Message{Community: "never retain this", VarBinds: []gosnmp.VarBind{pdu}}, nil
			}}
			source := &SourceRecorder{}
			packet, err := source.Wrap(handler).Get(context.Background(), []string{"1.2.3.0"})
			require.NoError(t, err)
			evidence := source.Finish()
			require.Equal(t, tc.want, evidence[0].PDUs[0].Value)
			if b, _ := packet.VarBinds[0].Value.Bytes(); len(b) > 0 {
				b[0] ^= 255
				require.Equal(t, tc.want, evidence[0].PDUs[0].Value)
			}
			encoded, err := json.Marshal(evidence)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "never retain this")
			var restored []*ddsnmp.SourceOperation
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
			handler.get = func(oids []string) (*gosnmp.Message, error) {
				packet := &gosnmp.Message{VarBinds: []gosnmp.VarBind{{Name: oids[0], Value: gosnmp.OctetStringValue([]byte("first"))}, {Name: oids[0], Value: gosnmp.OctetStringValue([]byte("last"))}, {Name: root + ".99", Value: gosnmp.NoSuchInstanceValue()}}}
				if tc.getFailure && handler.gets == 2 {
					return packet, context.DeadlineExceeded
				}
				return packet, nil
			}
			handler.walk = func(string) ([]gosnmp.VarBind, error) {
				pdus := []gosnmp.VarBind{{Name: root + ".1.7", Value: gosnmp.OctetStringValue([]byte("eth0"))}}
				if tc.walkFailure {
					return pdus, context.DeadlineExceeded
				}
				return pdus, nil
			}
			source := &SourceRecorder{}
			var reports []AcquisitionProfileReport
			collector := New(Config{Version: gosnmp.Version2c, MaxOIDs: 1, SnmpClient: source.Wrap(handler), Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: tc.profile}}, Log: logger.New(), AcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { reports = append(reports, r) })})
			values, err := collector.Collect(context.Background())
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

func testWalkSources(t *testing.T, collector *Collector, execution *AcquisitionExecutionReport) []*ddsnmp.SourceOperation {
	t.Helper()
	require.NotNil(t, execution)
	var result []*ddsnmp.SourceOperation
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
			handler := &sourceTestHandler{get: func(oids []string) (*gosnmp.Message, error) {
				return &gosnmp.Message{VarBinds: []gosnmp.VarBind{createGauge32PDU(oids[0], 6)}}, nil
			}}
			source := &SourceRecorder{}
			client := source.Wrap(handler)
			collector := New(Config{Version: gosnmp.Version2c, MaxOIDs: 1, SnmpClient: client, Log: logger.New(), Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{cfg}}}}, AcquisitionObserver: AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {})})
			if tc.replace {
				collector.SetSNMPClient(client)
			}
			_, err := collector.Collect(context.Background())
			require.NoError(t, err)
			evidence := source.Finish()
			require.NotEmpty(t, evidence)
			require.Nil(t, source.operations, "ownership transfers to the completed context")
			require.Nil(t, source.Finish(), "transfer occurs once")
			before, err := json.Marshal(evidence)
			require.NoError(t, err)
			_, err = collector.Collect(context.Background())
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
		pdus := make([]gosnmp.VarBind, rows)
		for i := range pdus {
			pdus[i] = createGauge32PDU(fmt.Sprintf("%s.1.%d", root, i), uint(i))
		}
		profile := &ddsnmp.Profile{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{Kind: ddsnmp.KindIfName, MetricsConfig: ddprofiledefinition.MetricsConfig{Table: ddprofiledefinition.SymbolConfig{OID: root, Name: "table"}, Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}}}}}}}
		for name, tc := range map[string]struct{ capture bool }{"disabled": {}, "enabled": {capture: true}} {
			b.Run(fmt.Sprintf("rows=%d/%s", rows, name), func(b *testing.B) {
				log := logger.New()
				b.ReportAllocs()
				for b.Loop() {
					var client snmputils.Client = &sourceTestHandler{walk: func(string) ([]gosnmp.VarBind, error) { return pdus, nil }}
					source := &SourceRecorder{}
					var observer AcquisitionObserver
					if tc.capture {
						client = source.Wrap(client)
						observer = AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {})
					}
					collector := New(Config{Version: gosnmp.Version2c, MaxOIDs: 1, SnmpClient: client, Log: log, Profiles: []*ddsnmp.Profile{profile}, AcquisitionObserver: observer})
					results, err := collector.Collect(context.Background())
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
