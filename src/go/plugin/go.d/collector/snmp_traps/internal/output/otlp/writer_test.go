// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"errors"
	"math"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestParseOTLPEndpoint(t *testing.T) {
	tests := map[string]struct {
		raw      string
		target   string
		insecure bool
		wantErr  bool
	}{
		"default": {
			target:   "127.0.0.1:4317",
			insecure: true,
		},
		"bare host port": {
			raw:      "localhost:4317",
			target:   "localhost:4317",
			insecure: true,
		},
		"plaintext url": {
			raw:      "http://localhost:4317",
			target:   "localhost:4317",
			insecure: true,
		},
		"tls url": {
			raw:      "https://otel.example.test:4317",
			target:   "otel.example.test:4317",
			insecure: false,
		},
		"unsupported scheme": {
			raw:     "grpc://localhost:4317",
			wantErr: true,
		},
		"path rejected": {
			raw:     "http://localhost:4317/v1/logs",
			wantErr: true,
		},
		"trailing slash rejected": {
			raw:     "http://localhost:4317/",
			wantErr: true,
		},
		"missing port": {
			raw:     "localhost",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseOTLPEndpoint(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.target, got.target)
			assert.Equal(t, tc.insecure, got.insecure)
		})
	}
}

func TestOTLPTargetIsLoopback(t *testing.T) {
	tests := map[string]struct {
		target string
		want   bool
	}{
		"ipv4 loopback": {target: "127.0.0.1:4317", want: true},
		"ipv6 loopback": {target: "[::1]:4317", want: true},
		"localhost":     {target: "localhost:4317", want: true},
		"remote ipv4":   {target: "192.0.2.10:4317", want: false},
		"remote dns":    {target: "otel.example.test:4317", want: false},
		"invalid":       {target: "otel.example.test", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, targetIsLoopback(tc.target))
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		want    Policy
		wantErr bool
	}{
		"defaults": {
			cfg: Config{},
			want: Policy{
				target:         "127.0.0.1:4317",
				insecure:       true,
				requestTimeout: defaultOTLPRequestTimeout,
				flushInterval:  defaultOTLPFlushInterval,
				batchSize:      defaultOTLPBatchSize,
				queueCapacity:  defaultOTLPQueueCapacity,
			},
		},
		"custom values": {
			cfg: Config{
				Endpoint:       "localhost:14317",
				RequestTimeout: "2s",
				FlushInterval:  "250ms",
				BatchSize:      16,
				QueueCapacity:  64,
				Headers:        map[string]string{"Authorization": "Bearer test-token"},
			},
			want: Policy{
				target:         "localhost:14317",
				insecure:       true,
				requestTimeout: 2 * time.Second,
				flushInterval:  250 * time.Millisecond,
				batchSize:      16,
				queueCapacity:  64,
			},
		},
		"bad duration": {
			cfg:     Config{RequestTimeout: "-1s"},
			wantErr: true,
		},
		"bad header": {
			cfg:     Config{Headers: map[string]string{"grpc-timeout": "1S"}},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Normalize(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want.target, got.target)
			assert.Equal(t, tc.want.insecure, got.insecure)
			assert.Equal(t, tc.want.requestTimeout, got.requestTimeout)
			assert.Equal(t, tc.want.flushInterval, got.flushInterval)
			assert.Equal(t, tc.want.batchSize, got.batchSize)
			assert.Equal(t, tc.want.queueCapacity, got.queueCapacity)
		})
	}
}

func TestOTLPTrapEntrySerialization(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:              "local",
		ReportType:           model.ReportTypeTrap,
		ReceivedRealtimeUsec: 123456,
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		TrapName:             "IF-MIB::linkDown",
		Category:             "state_change",
		Severity:             "crit",
		Message:              "Interface down",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		DeviceHostname:       "switch-1",
		DeviceVendor:         "cisco",
		PduType:              model.PduTypeTrap,
		SnmpVersion:          model.SnmpVersionV2c,
		SourceVnodeID:        "vnode-1",
		TopologyInterface:    "Gi1/0/1",
		TopologyNeighbors:    "switch-2",
		Labels:               map[string]string{"site": "lab"},
		Varbinds: []model.VarbindValue{{
			Name:  "ifName",
			OID:   "1.3.6.1.2.1.31.1.1.1.1.7",
			Type:  "OctetString",
			Value: "Gi1/0/1",
		}},
	}

	req, err := buildOTLPExportRequest("local", []*model.TrapEntry{entry})
	require.NoError(t, err)
	require.Len(t, req.ResourceLogs, 1)
	rl := req.ResourceLogs[0]
	resAttrs := otlpAttrMap(rl.Resource.Attributes)
	assert.Equal(t, "netdata-snmptrap", resAttrs["service.name"].GetStringValue())
	assert.Equal(t, "local", resAttrs["service.instance.id"].GetStringValue())

	require.Len(t, rl.ScopeLogs, 1)
	require.Len(t, rl.ScopeLogs[0].LogRecords, 1)
	record := rl.ScopeLogs[0].LogRecords[0]
	assert.Equal(t, uint64(123456000), record.TimeUnixNano)
	assert.Equal(t, logpb.SeverityNumber_SEVERITY_NUMBER_ERROR2, record.SeverityNumber)
	assert.Equal(t, "ERROR2", record.SeverityText)
	assert.Equal(t, "Interface down", record.Body.GetStringValue())
	assert.Equal(t, "snmp.trap.state_change", record.EventName)

	attrs := otlpAttrMap(record.Attributes)
	assert.Equal(t, "192.0.2.10", attrs["network.peer.address"].GetStringValue())
	assert.Equal(t, "192.0.2.10", attrs["snmp.source.ip"].GetStringValue())
	assert.Equal(t, "v2c", attrs["snmp.version"].GetStringValue())
	assert.Equal(t, "1.3.6.1.6.3.1.1.5.3", attrs["snmp.trap.oid"].GetStringValue())
	assert.Equal(t, "IF-MIB::linkDown", attrs["snmp.trap.name"].GetStringValue())
	assert.Equal(t, "state_change", attrs["snmp.trap.category"].GetStringValue())
	assert.Equal(t, "crit", attrs["snmp.trap.severity"].GetStringValue())
	assert.Equal(t, "trap", attrs["snmp.trap.pdu_type"].GetStringValue())
	assert.Equal(t, "trap", attrs["snmp.trap.report_type"].GetStringValue())
	assert.Equal(t, "switch-1", attrs["snmp.device.hostname"].GetStringValue())
	assert.Equal(t, "cisco", attrs["snmp.device.vendor"].GetStringValue())
	assert.Equal(t, "vnode-1", attrs["netdata.nidl.node"].GetStringValue())
	assert.Equal(t, "Gi1/0/1", attrs["netdata.topology.interface"].GetStringValue())
	assert.Equal(t, "switch-2", attrs["netdata.topology.neighbors"].GetStringValue())
	assert.Equal(t, "lab", attrs["trap.site"].GetStringValue())

	varbinds := otlpKVListMap(attrs["snmp.varbinds"])
	ifName := otlpKVListMap(varbinds["ifName"])
	assert.Equal(t, "1.3.6.1.2.1.31.1.1.1.1.7", ifName["oid"].GetStringValue())
	assert.Equal(t, "OctetString", ifName["type"].GetStringValue())
	assert.Equal(t, "Gi1/0/1", ifName["value"].GetStringValue())
}

func TestOTLPAnyValueCanonicalTypes(t *testing.T) {
	tests := map[string]struct {
		value any
		want  *commonpb.AnyValue
	}{
		"nil":      {want: &commonpb.AnyValue{}},
		"string":   {value: "text", want: otlpStringValue("text")},
		"int64":    {value: int64(-42), want: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: -42}}},
		"uint64":   {value: uint64(42), want: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
		"big uint": {value: uint64(math.MaxInt64) + 1, want: otlpStringValue("9223372036854775808")},
		"float64":  {value: 1.25, want: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 1.25}}},
		"bool":     {value: true, want: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
		"bytes":    {value: []byte{0x00, 0x0f, 0xff}, want: otlpStringValue("000fff")},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, otlpAnyValue(output.CanonicalizeValue(tc.value)))
		})
	}
}

func TestOTLPTrapEntrySerializationOmitsCommunityVarbind(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:              "local",
		ReportType:           model.ReportTypeTrap,
		ReceivedRealtimeUsec: 123456,
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "Interface down",
		SourceIP:             "192.0.2.10",
		PduType:              model.PduTypeTrap,
		SnmpVersion:          model.SnmpVersionV1,
		Varbinds: []model.VarbindValue{
			{OID: model.SNMPTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
			{Name: "ifName", OID: "1.3.6.1.2.1.31.1.1.1.1.7", Type: "OctetString", Value: "Gi1/0/1"},
		},
	}

	req, err := buildOTLPExportRequest("local", []*model.TrapEntry{entry})
	require.NoError(t, err)
	require.Len(t, req.ResourceLogs, 1)
	require.Len(t, req.ResourceLogs[0].ScopeLogs, 1)
	require.Len(t, req.ResourceLogs[0].ScopeLogs[0].LogRecords, 1)

	attrs := otlpAttrMap(req.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Attributes)
	varbinds := otlpKVListMap(attrs["snmp.varbinds"])
	assert.NotContains(t, varbinds, "snmpTrapCommunity.0")
	assert.Contains(t, varbinds, "ifName")
}

func TestOTLPDedupSummarySerialization(t *testing.T) {
	entry := &model.TrapEntry{
		JobName:              "local",
		ReportType:           model.ReportTypeDedupSummary,
		ReceivedRealtimeUsec: 1000,
		Severity:             "info",
		Message:              "DEDUPLICATED TRAPS",
		SummaryCounts: &model.DedupSummary{
			TotalSuppressed: 12,
			Fingerprints:    2,
			PeriodSec:       5,
			ByTrap:          map[string]int64{"1.3.6.1.6.3.1.1.5.3": 12},
		},
	}

	record, err := trapEntryToOTLPLogRecord(entry)
	require.NoError(t, err)
	assert.Equal(t, logpb.SeverityNumber_SEVERITY_NUMBER_INFO, record.SeverityNumber)
	assert.Equal(t, "snmp.trap.deduplication_summary", record.EventName)

	attrs := otlpAttrMap(record.Attributes)
	assert.Equal(t, int64(12), attrs["snmp.trap.suppressed_count"].GetIntValue())
	assert.Equal(t, int64(2), attrs["snmp.trap.suppressed_fingerprints"].GetIntValue())
	assert.Equal(t, int64(5), attrs["snmp.trap.report_period_sec"].GetIntValue())
	assert.Equal(t, "deduplication_summary", attrs["snmp.trap.report_type"].GetStringValue())
	summary := otlpKVListMap(attrs["snmp.varbinds"])
	assert.Equal(t, int64(12), summary["total_suppressed"].GetIntValue())
	assert.Equal(t, int64(5), summary["period_sec"].GetIntValue())
	assert.Equal(t, int64(2), summary["fingerprints"].GetIntValue())
}

func TestOTLPDecodeErrorSerialization(t *testing.T) {
	const packetHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	entry := &model.TrapEntry{
		JobName:              "local",
		ReportType:           model.ReportTypeDecodeError,
		ReceivedRealtimeUsec: 2000,
		Category:             "diagnostic",
		Severity:             "warning",
		Message:              "SNMP trap decode failed from 192.0.2.10: malformed_pdu: BER: trailing data",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          model.SnmpVersionV2c,
		DecodeError: &model.DecodeErrorInfo{
			Kind:          "malformed_pdu",
			Error:         "BER: trailing data",
			PacketSize:    42,
			PacketSHA256:  packetHash,
			SourceUDPPort: 9162,
			Listener:      "0.0.0.0:162",
			EngineID:      "8000000001020304",
		},
	}

	record, err := trapEntryToOTLPLogRecord(entry)
	require.NoError(t, err)
	assert.Equal(t, logpb.SeverityNumber_SEVERITY_NUMBER_WARN, record.SeverityNumber)
	assert.Equal(t, "snmp.trap.decode_error", record.EventName)
	assert.Equal(t, "decode_error", otlpAttrMap(record.Attributes)["snmp.trap.report_type"].GetStringValue())

	attrs := otlpAttrMap(record.Attributes)
	assert.Equal(t, "malformed_pdu", attrs["snmp.trap.decode_error.kind"].GetStringValue())
	assert.Equal(t, "BER: trailing data", attrs["snmp.trap.decode_error.message"].GetStringValue())
	assert.Equal(t, int64(42), attrs["snmp.trap.packet_size"].GetIntValue())
	assert.Equal(t, packetHash, attrs["snmp.trap.packet_sha256"].GetStringValue())
	assert.Equal(t, int64(9162), attrs["network.peer.port"].GetIntValue())
	assert.Equal(t, "0.0.0.0:162", attrs["netdata.trap.listener"].GetStringValue())
	assert.Equal(t, "8000000001020304", attrs["snmp.engine_id"].GetStringValue())

	details := otlpKVListMap(attrs["snmp.varbinds"])
	assert.Equal(t, "malformed_pdu", details["kind"].GetStringValue())
	assert.Equal(t, "BER: trailing data", details["error"].GetStringValue())
	assert.Equal(t, int64(42), details["packet_size"].GetIntValue())
	assert.Equal(t, packetHash, details["packet_sha256"].GetStringValue())
}

func TestOTLPTrapWriterPreflightHeadersAndFlush(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	policy, err := Normalize(Config{
		Endpoint:       srv.endpoint,
		Headers:        map[string]string{"authorization": "Bearer test-token"},
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	recorder := &outcomeRecorder{}
	writer, err := Prepare(t.Context(), "local", policy, Options{Authoritative: true, Report: recorder.report})
	require.NoError(t, err)
	require.NoError(t, writer.Start())
	defer writer.Close()

	require.Len(t, srv.requests(), 1, "preflight export")
	assert.Equal(t, []string{"Bearer test-token"}, srv.metadata()[0].Get("authorization"))

	require.NoError(t, writer.Write(&model.TrapEntry{
		JobName:              "local",
		ReportType:           model.ReportTypeTrap,
		ReceivedRealtimeUsec: time.Now().UnixMicro(),
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "Interface down",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          model.SnmpVersionV2c,
		PduType:              model.PduTypeTrap,
	}))
	require.NoError(t, writer.Flush())

	reqs := srv.requests()
	require.Len(t, reqs, 2)
	require.Len(t, reqs[1].ResourceLogs[0].ScopeLogs[0].LogRecords, 1)
	assert.Empty(t, recorder.snapshot())
}

func TestOTLPTrapWriterReportsNonAuthoritativeExportFailure(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	recorder := &outcomeRecorder{}
	writer := newTestOTLPTrapWriter(t, srv, false, recorder.report)
	entry := testTrapEntry()

	srv.setExportErr(errors.New("export failed"))
	require.NoError(t, writer.Write(entry))
	require.Error(t, writer.Flush())

	outcomes := recorder.snapshot()
	require.Len(t, outcomes, 1)
	assert.Equal(t, output.BackendOTLP, outcomes[0].Backend)
	assert.Equal(t, output.StageExport, outcomes[0].Stage)
	assert.Equal(t, uint64(1), outcomes[0].FailedEntries)
	assert.False(t, outcomes[0].Authoritative)
	assert.ErrorContains(t, outcomes[0].Err, "export failed")

	srv.setExportErr(nil)
	require.NoError(t, writer.Close())
}

func TestOTLPTrapWriterAuthoritativeAsyncExportFailureRecordsTerminalWriteFailure(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	recorder := &outcomeRecorder{}
	writer := newTestOTLPTrapWriter(t, srv, true, recorder.report)
	entry := testTrapEntry()

	srv.setExportErr(errors.New("export failed"))
	require.NoError(t, writer.Write(entry))
	require.Error(t, writer.Flush())

	outcomes := recorder.snapshot()
	require.Len(t, outcomes, 1)
	assert.Equal(t, output.BackendOTLP, outcomes[0].Backend)
	assert.Equal(t, output.StageExport, outcomes[0].Stage)
	assert.Equal(t, uint64(1), outcomes[0].FailedEntries)
	assert.True(t, outcomes[0].Authoritative)
	assert.ErrorContains(t, outcomes[0].Err, "export failed")

	srv.setExportErr(nil)
	require.NoError(t, writer.Close())
}

func TestOTLPTrapWriterDrainQueueCountsFailuresAfterFirstFailedBatch(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	policy, err := Normalize(Config{
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      2,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	conn, client, err := newOTLPClient(t.Context(), policy)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	srv.setExportErr(errors.New("export failed"))

	recorder := &outcomeRecorder{}
	writer := &Writer{
		client:         client,
		conn:           conn,
		queue:          make(chan *model.TrapEntry, policy.queueCapacity),
		headers:        policy.headers,
		requestTimeout: policy.requestTimeout,
		batchSize:      policy.batchSize,
		jobName:        "local",
		report:         recorder.report,
		authoritative:  true,
	}
	entry := testTrapEntry()
	for range 5 {
		writer.queue <- entry
	}

	batch := make([]*model.TrapEntry, 0, writer.batchSize)
	reportedFailed := 0
	err = writer.drainQueue(&batch, &reportedFailed)
	require.Error(t, err)
	_ = writer.exportPending(&batch, &reportedFailed)

	assert.Equal(t, uint64(5), recorder.failedEntries())

	reqs := srv.requests()
	totalRecords := 0
	for i, req := range reqs {
		if len(req.ResourceLogs) == 0 {
			continue
		}
		require.Len(t, req.ResourceLogs, 1)
		require.Len(t, req.ResourceLogs[0].ScopeLogs, 1)
		records := req.ResourceLogs[0].ScopeLogs[0].LogRecords
		assert.LessOrEqualf(t, len(records), policy.batchSize, "request %d exceeded batch size", i)
		totalRecords += len(records)
	}
	assert.Equal(t, 5, totalRecords)

}

func TestOTLPTrapWriterPreflightFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	policy, err := Normalize(Config{
		Endpoint:       endpoint,
		RequestTimeout: "50ms",
	})
	require.NoError(t, err)
	_, err = Prepare(t.Context(), "local", policy, Options{})
	require.Error(t, err)
}

func TestOTLPTrapWriterFlushAfterCloseReturns(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	writer := newTestOTLPTrapWriter(t, srv, true, nil)
	require.NoError(t, writer.Close())

	done := make(chan error, 1)
	go func() {
		done <- writer.Flush()
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, output.ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("Flush blocked after Close")
	}
}

func TestOTLPTrapWriterPreparedLifecycle(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	policy, err := Normalize(Config{
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	writer, err := Prepare(t.Context(), "local", policy, Options{})
	require.NoError(t, err)

	require.ErrorIs(t, writer.Write(testTrapEntry()), output.ErrNotStarted)
	require.ErrorIs(t, writer.Flush(), output.ErrNotStarted)
	require.NoError(t, writer.Close())
	require.ErrorIs(t, writer.Start(), output.ErrClosed)
}

func TestOTLPTrapWriterWriteQueueFull(t *testing.T) {
	writer := &Writer{
		queue:   make(chan *model.TrapEntry, 1),
		started: true,
	}

	require.NoError(t, writer.Write(&model.TrapEntry{JobName: "local", Message: "first"}))
	require.ErrorIs(t, writer.Write(&model.TrapEntry{JobName: "local", Message: "second"}), output.ErrQueueFull)
}

func TestOTLPTrapWriterWorkerPanicFailsClosed(t *testing.T) {
	recorder := &outcomeRecorder{}
	writer := &Writer{
		queue:          make(chan *model.TrapEntry, 1),
		flushCh:        make(chan chan error),
		closeCh:        make(chan chan error),
		doneCh:         make(chan struct{}),
		flushInterval:  time.Hour,
		batchSize:      1,
		requestTimeout: time.Millisecond,
		report:         recorder.report,
		authoritative:  true,
	}
	require.NoError(t, writer.Start())

	entry := testTrapEntry()
	require.NoError(t, writer.Write(entry))
	select {
	case <-writer.doneCh:
	case <-time.After(time.Second):
		t.Fatal("OTLP worker did not exit after panic")
	}

	assert.Equal(t, uint64(1), recorder.failedEntries())

	require.ErrorIs(t, writer.Write(&model.TrapEntry{JobName: "local", Message: "second"}), output.ErrClosed)
	err := writer.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SNMP trap OTLP writer panic")
}

func TestOTLPTrapWriterWorkerPanicUnblocksFlushAndAccountsQueuedEntries(t *testing.T) {
	recorder := &outcomeRecorder{}
	writer := &Writer{
		queue:          make(chan *model.TrapEntry, 10),
		flushCh:        make(chan chan error),
		closeCh:        make(chan chan error),
		doneCh:         make(chan struct{}),
		flushInterval:  time.Hour,
		batchSize:      10,
		requestTimeout: time.Millisecond,
		report:         recorder.report,
		authoritative:  true,
	}
	require.NoError(t, writer.Start())

	entry := testTrapEntry()
	require.NoError(t, writer.Write(entry))
	require.NoError(t, writer.Write(entry))

	done := make(chan error, 1)
	go func() {
		done <- writer.Flush()
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SNMP trap OTLP writer panic")
	case <-time.After(time.Second):
		t.Fatal("Flush blocked after OTLP worker panic")
	}
	select {
	case <-writer.doneCh:
	case <-time.After(time.Second):
		t.Fatal("OTLP worker did not exit after panic")
	}

	assert.Equal(t, uint64(2), recorder.failedEntries())

}

func TestOTLPTrapWriterExternalReceiver(t *testing.T) {
	endpoint := os.Getenv("NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT")
	if endpoint == "" {
		t.Skip("set NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT to run against a real OTLP/gRPC logs receiver")
	}

	policy, err := Normalize(Config{
		Endpoint:       endpoint,
		RequestTimeout: "5s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	writer, err := Prepare(t.Context(), "external", policy, Options{Authoritative: true})
	require.NoError(t, err)
	require.NoError(t, writer.Start())
	defer writer.Close()

	require.NoError(t, writer.Write(&model.TrapEntry{
		JobName:              "external",
		ReportType:           model.ReportTypeTrap,
		ReceivedRealtimeUsec: time.Now().UnixMicro(),
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		TrapName:             "SNMPv2-MIB::coldStart",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "External receiver interop test",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          model.SnmpVersionV2c,
		PduType:              model.PduTypeTrap,
	}))
	require.NoError(t, writer.Flush())
}

type otlpFixture struct {
	collogpb.UnimplementedLogsServiceServer

	t        *testing.T
	endpoint string
	server   *grpc.Server
	err      error

	mu       sync.Mutex
	reqs     []*collogpb.ExportLogsServiceRequest
	incoming []metadata.MD
}

func startOTLPFixture(t *testing.T, exportErr error) *otlpFixture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	f := &otlpFixture{
		t:        t,
		endpoint: "http://" + ln.Addr().String(),
		server:   grpc.NewServer(),
		err:      exportErr,
	}
	collogpb.RegisterLogsServiceServer(f.server, f)
	go func() {
		_ = f.server.Serve(ln)
	}()
	t.Cleanup(func() {
		f.server.Stop()
		_ = ln.Close()
	})
	return f
}

func newTestOTLPTrapWriter(t *testing.T, srv *otlpFixture, authoritative bool, report output.OutcomeReporter) *Writer {
	t.Helper()
	policy, err := Normalize(Config{
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	writer, err := Prepare(t.Context(), "local", policy, Options{Authoritative: authoritative, Report: report})
	require.NoError(t, err)
	require.NoError(t, writer.Start())
	return writer
}

type outcomeRecorder struct {
	mu       sync.Mutex
	outcomes []output.Outcome
}

func (r *outcomeRecorder) report(outcome output.Outcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes = append(r.outcomes, outcome)
}

func (r *outcomeRecorder) snapshot() []output.Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]output.Outcome(nil), r.outcomes...)
}

func (r *outcomeRecorder) failedEntries() uint64 {
	var total uint64
	for _, outcome := range r.snapshot() {
		total += outcome.FailedEntries
	}
	return total
}

func testTrapEntry() *model.TrapEntry {
	return &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  time.Now().UnixMicro(),
		ReceivedMonotonicUsec: 1,
		TrapOID:               "1.3.6.1.6.3.1.1.5.3",
		Category:              "state_change",
		Severity:              "warning",
		Message:               "Interface down",
		SourceIP:              "192.0.2.10",
		SourceUDPPeer:         "192.0.2.10:162",
		SnmpVersion:           model.SnmpVersionV2c,
		PduType:               model.PduTypeTrap,
	}
}

func (f *otlpFixture) setExportErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *otlpFixture) Export(ctx context.Context, req *collogpb.ExportLogsServiceRequest) (*collogpb.ExportLogsServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		f.incoming = append(f.incoming, md.Copy())
	} else {
		f.incoming = append(f.incoming, metadata.MD{})
	}
	if f.err != nil {
		return nil, f.err
	}
	return &collogpb.ExportLogsServiceResponse{}, nil
}

func (f *otlpFixture) requests() []*collogpb.ExportLogsServiceRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*collogpb.ExportLogsServiceRequest, len(f.reqs))
	copy(out, f.reqs)
	return out
}

func (f *otlpFixture) metadata() []metadata.MD {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]metadata.MD, len(f.incoming))
	copy(out, f.incoming)
	return out
}

func otlpAttrMap(attrs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	m := make(map[string]*commonpb.AnyValue, len(attrs))
	for _, attr := range attrs {
		m[attr.Key] = attr.Value
	}
	return m
}

func otlpKVListMap(v *commonpb.AnyValue) map[string]*commonpb.AnyValue {
	m := make(map[string]*commonpb.AnyValue)
	for _, kv := range v.GetKvlistValue().GetValues() {
		m[kv.Key] = kv.Value
	}
	return m
}
