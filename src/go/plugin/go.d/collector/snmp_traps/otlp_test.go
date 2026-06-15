// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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
			assert.Equal(t, tc.want, otlpTargetIsLoopback(tc.target))
		})
	}
}

func collectSourceMetricsForEntry(t *testing.T, metrics *perJobMetrics, entry *TrapEntry) (metrix.CollectorStore, metrix.Labels) {
	t.Helper()
	sourceID, sourceKind := metrics.fallbackSourceIdentityForTest(entry)
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	collectSourceMetrics(store, "local", metrics)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
	return store, metrix.Labels{"job_name": "local", "source_id": sourceID, "source_kind": sourceKind}
}

func assertSourceMetricValue(t *testing.T, store metrix.CollectorStore, metric string, labels metrix.Labels, want float64) {
	t.Helper()
	if v, ok := store.Read().Value(metric, labels); !ok || v != want {
		t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, want)
	}
}

func TestValidateOTLPConfig(t *testing.T) {
	tests := map[string]struct {
		cfg     OTLPConfig
		want    otlpRuntimeConfig
		wantErr bool
	}{
		"disabled ignores invalid endpoint": {
			cfg: OTLPConfig{Endpoint: "not a valid endpoint"},
		},
		"enabled defaults": {
			cfg: OTLPConfig{Enabled: true},
			want: otlpRuntimeConfig{
				target:         "127.0.0.1:4317",
				insecure:       true,
				requestTimeout: defaultOTLPRequestTimeout,
				flushInterval:  defaultOTLPFlushInterval,
				batchSize:      defaultOTLPBatchSize,
				queueCapacity:  defaultOTLPQueueCapacity,
			},
		},
		"custom values": {
			cfg: OTLPConfig{
				Enabled:        true,
				Endpoint:       "localhost:14317",
				RequestTimeout: "2s",
				FlushInterval:  "250ms",
				BatchSize:      16,
				QueueCapacity:  64,
				Headers:        map[string]string{"Authorization": "Bearer test-token"},
			},
			want: otlpRuntimeConfig{
				target:         "localhost:14317",
				insecure:       true,
				requestTimeout: 2 * time.Second,
				flushInterval:  250 * time.Millisecond,
				batchSize:      16,
				queueCapacity:  64,
			},
		},
		"bad duration": {
			cfg:     OTLPConfig{Enabled: true, RequestTimeout: "-1s"},
			wantErr: true,
		},
		"bad header": {
			cfg:     OTLPConfig{Enabled: true, Headers: map[string]string{"grpc-timeout": "1S"}},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := validateOTLPConfig(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if !tc.cfg.Enabled {
				assert.Empty(t, got.target)
				return
			}
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
	entry := &TrapEntry{
		JobName:              "local",
		ReportType:           ReportTypeTrap,
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
		PduType:              PduTypeTrap,
		SnmpVersion:          SnmpVersionV2c,
		SourceVnodeID:        "vnode-1",
		TopologyInterface:    "Gi1/0/1",
		TopologyNeighbors:    "switch-2",
		Labels:               map[string]string{"site": "lab"},
		Varbinds: []VarbindValue{{
			Name:  "ifName",
			OID:   "1.3.6.1.2.1.31.1.1.1.1.7",
			Type:  "OctetString",
			Value: "Gi1/0/1",
		}},
	}

	req, err := buildOTLPExportRequest("local", []*TrapEntry{entry})
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

func TestOTLPTrapEntrySerializationOmitsCommunityVarbind(t *testing.T) {
	entry := &TrapEntry{
		JobName:              "local",
		ReportType:           ReportTypeTrap,
		ReceivedRealtimeUsec: 123456,
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "Interface down",
		SourceIP:             "192.0.2.10",
		PduType:              PduTypeTrap,
		SnmpVersion:          SnmpVersionV1,
		Varbinds: []VarbindValue{
			{OID: snmpTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
			{Name: "ifName", OID: "1.3.6.1.2.1.31.1.1.1.1.7", Type: "OctetString", Value: "Gi1/0/1"},
		},
	}

	req, err := buildOTLPExportRequest("local", []*TrapEntry{entry})
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
	entry := &TrapEntry{
		JobName:              "local",
		ReportType:           ReportTypeDedupSummary,
		ReceivedRealtimeUsec: 1000,
		Severity:             "info",
		Message:              "DEDUPLICATED TRAPS",
		SummaryCounts: &DedupSummary{
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

	entry := &TrapEntry{
		JobName:              "local",
		ReportType:           ReportTypeDecodeError,
		ReceivedRealtimeUsec: 2000,
		Category:             "diagnostic",
		Severity:             "warning",
		Message:              "SNMP trap decode failed from 192.0.2.10: malformed_pdu: BER: trailing data",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          SnmpVersionV2c,
		DecodeError: &DecodeErrorInfo{
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
	metrics := &perJobMetrics{}

	writer, err := newOTLPTrapWriter(t.Context(), "local", OTLPConfig{
		Enabled:        true,
		Endpoint:       srv.endpoint,
		Headers:        map[string]string{"authorization": "Bearer test-token"},
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	}, metrics)
	require.NoError(t, err)
	defer writer.Close()

	require.Len(t, srv.requests(), 1, "preflight export")
	assert.Equal(t, []string{"Bearer test-token"}, srv.metadata()[0].Get("authorization"))

	require.NoError(t, writer.Write(&TrapEntry{
		JobName:              "local",
		ReportType:           ReportTypeTrap,
		ReceivedRealtimeUsec: time.Now().UnixMicro(),
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "Interface down",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          SnmpVersionV2c,
		PduType:              PduTypeTrap,
	}))
	require.NoError(t, writer.Flush())

	reqs := srv.requests()
	require.Len(t, reqs, 2)
	require.Len(t, reqs[1].ResourceLogs[0].ScopeLogs[0].LogRecords, 1)
	assert.Equal(t, uint64(0), metrics.errors.otlpExportFailed.Load())
}

func TestOTLPTrapWriterSecondaryAsyncExportFailureDoesNotRecordTerminalWriteFailure(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	metrics := &perJobMetrics{}
	writer := newTestOTLPTrapWriter(t, srv, metrics, false)

	primary := &mockTrapWriter{}
	fanout := newFanoutTrapWriter(primary, writer, metrics)
	entry := testFanoutTrapEntry()

	srv.setExportErr(errors.New("export failed"))
	require.NoError(t, fanout.Write(entry))
	require.Error(t, fanout.Flush())

	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(0), metrics.pipeline.writeFailed.Load())

	store, labels := collectSourceMetricsForEntry(t, metrics, entry)
	assertSourceMetricValue(t, store, "snmp_trap_source_pipeline_write_failed", labels, 0)
	assertSourceMetricValue(t, store, "snmp_trap_source_errors_otlp_export_failed", labels, 1)

	srv.setExportErr(nil)
	require.NoError(t, writer.Close())
}

func TestOTLPTrapWriterAuthoritativeAsyncExportFailureRecordsTerminalWriteFailure(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	metrics := &perJobMetrics{}
	writer := newTestOTLPTrapWriter(t, srv, metrics, true)
	entry := testFanoutTrapEntry()

	srv.setExportErr(errors.New("export failed"))
	require.NoError(t, writer.Write(entry))
	require.Error(t, writer.Flush())

	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(1), metrics.pipeline.writeFailed.Load())

	store, labels := collectSourceMetricsForEntry(t, metrics, entry)
	for metric, expected := range map[string]float64{
		"snmp_trap_source_pipeline_write_failed":     1,
		"snmp_trap_source_errors_otlp_export_failed": 1,
	} {
		assertSourceMetricValue(t, store, metric, labels, expected)
	}

	srv.setExportErr(nil)
	require.NoError(t, writer.Close())
}

func TestOTLPTrapWriterDrainQueueCountsFailuresAfterFirstFailedBatch(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	runtimeCfg, err := validateOTLPConfig(OTLPConfig{
		Enabled:        true,
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      2,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	conn, client, err := newOTLPClient(t.Context(), runtimeCfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	srv.setExportErr(errors.New("export failed"))

	metrics := &perJobMetrics{}
	writer := &otlpTrapWriter{
		client:         client,
		conn:           conn,
		queue:          make(chan *TrapEntry, runtimeCfg.queueCapacity),
		headers:        runtimeCfg.headers,
		requestTimeout: runtimeCfg.requestTimeout,
		batchSize:      runtimeCfg.batchSize,
		jobName:        "local",
		metrics:        metrics,
		terminalErrors: true,
	}
	entry := testFanoutTrapEntry()
	for range 5 {
		writer.queue <- entry
	}

	batch := make([]*TrapEntry, 0, writer.batchSize)
	reportedFailed := 0
	err = writer.drainQueue(&batch, &reportedFailed)
	require.Error(t, err)
	_ = writer.exportPending(&batch, &reportedFailed)

	assert.Equal(t, uint64(5), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(5), metrics.pipeline.writeFailed.Load())

	reqs := srv.requests()
	totalRecords := 0
	for i, req := range reqs {
		if len(req.ResourceLogs) == 0 {
			continue
		}
		require.Len(t, req.ResourceLogs, 1)
		require.Len(t, req.ResourceLogs[0].ScopeLogs, 1)
		records := req.ResourceLogs[0].ScopeLogs[0].LogRecords
		assert.LessOrEqualf(t, len(records), runtimeCfg.batchSize, "request %d exceeded batch size", i)
		totalRecords += len(records)
	}
	assert.Equal(t, 5, totalRecords)

	sourceID, sourceKind := metrics.fallbackSourceIdentityForTest(entry)
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	collectSourceMetrics(store, "local", metrics)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	labels := metrix.Labels{"job_name": "local", "source_id": sourceID, "source_kind": sourceKind}
	for metric, expected := range map[string]float64{
		"snmp_trap_source_pipeline_write_failed":     5,
		"snmp_trap_source_errors_otlp_export_failed": 5,
	} {
		if v, ok := store.Read().Value(metric, labels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, expected)
		}
	}
}

func TestOTLPTrapWriterPreflightFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	_, err = newOTLPTrapWriter(t.Context(), "local", OTLPConfig{
		Enabled:        true,
		Endpoint:       endpoint,
		RequestTimeout: "50ms",
	}, nil)
	require.Error(t, err)
}

func TestOTLPTrapWriterFlushAfterCloseReturns(t *testing.T) {
	srv := startOTLPFixture(t, nil)
	writer, err := newOTLPTrapWriter(t.Context(), "local", OTLPConfig{
		Enabled:        true,
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	done := make(chan error, 1)
	go func() {
		done <- writer.Flush()
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errWriterClosed)
	case <-time.After(time.Second):
		t.Fatal("Flush blocked after Close")
	}
}

func TestOTLPTrapWriterWriteQueueFull(t *testing.T) {
	writer := &otlpTrapWriter{
		queue: make(chan *TrapEntry, 1),
	}

	require.NoError(t, writer.Write(&TrapEntry{JobName: "local", Message: "first"}))
	require.ErrorIs(t, writer.Write(&TrapEntry{JobName: "local", Message: "second"}), errQueueFull)
}

func TestOTLPTrapWriterWorkerPanicFailsClosed(t *testing.T) {
	metrics := &perJobMetrics{}
	writer := &otlpTrapWriter{
		queue:          make(chan *TrapEntry, 1),
		flushCh:        make(chan chan error),
		closeCh:        make(chan chan error),
		doneCh:         make(chan struct{}),
		flushInterval:  time.Hour,
		batchSize:      1,
		requestTimeout: time.Millisecond,
		metrics:        metrics,
		terminalErrors: true,
	}
	go writer.worker()

	entry := testFanoutTrapEntry()
	require.NoError(t, writer.Write(entry))
	select {
	case <-writer.doneCh:
	case <-time.After(time.Second):
		t.Fatal("OTLP worker did not exit after panic")
	}

	assert.Equal(t, uint64(1), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(1), metrics.pipeline.writeFailed.Load())

	store, labels := collectSourceMetricsForEntry(t, metrics, entry)
	for metric, expected := range map[string]float64{
		"snmp_trap_source_pipeline_write_failed":     1,
		"snmp_trap_source_errors_otlp_export_failed": 1,
	} {
		assertSourceMetricValue(t, store, metric, labels, expected)
	}

	require.ErrorIs(t, writer.Write(&TrapEntry{JobName: "local", Message: "second"}), errWriterClosed)
	err := writer.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SNMP trap OTLP writer panic")
}

func TestOTLPTrapWriterWorkerPanicUnblocksFlushAndAccountsQueuedEntries(t *testing.T) {
	metrics := &perJobMetrics{}
	writer := &otlpTrapWriter{
		queue:          make(chan *TrapEntry, 10),
		flushCh:        make(chan chan error),
		closeCh:        make(chan chan error),
		doneCh:         make(chan struct{}),
		flushInterval:  time.Hour,
		batchSize:      10,
		requestTimeout: time.Millisecond,
		metrics:        metrics,
		terminalErrors: true,
	}
	go writer.worker()

	entry := testFanoutTrapEntry()
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

	assert.Equal(t, uint64(2), metrics.errors.otlpExportFailed.Load())
	assert.Equal(t, uint64(2), metrics.pipeline.writeFailed.Load())

	sourceID, sourceKind := metrics.fallbackSourceIdentityForTest(entry)
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	collectSourceMetrics(store, "local", metrics)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	labels := metrix.Labels{"job_name": "local", "source_id": sourceID, "source_kind": sourceKind}
	for metric, expected := range map[string]float64{
		"snmp_trap_source_pipeline_write_failed":     2,
		"snmp_trap_source_errors_otlp_export_failed": 2,
	} {
		if v, ok := store.Read().Value(metric, labels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, expected)
		}
	}
}

func TestOTLPTrapWriterExternalReceiver(t *testing.T) {
	endpoint := os.Getenv("NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT")
	if endpoint == "" {
		t.Skip("set NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT to run against a real OTLP/gRPC logs receiver")
	}

	writer, err := newOTLPTrapWriter(t.Context(), "external", OTLPConfig{
		Enabled:        true,
		Endpoint:       endpoint,
		RequestTimeout: "5s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	}, &perJobMetrics{})
	require.NoError(t, err)
	defer writer.Close()

	require.NoError(t, writer.Write(&TrapEntry{
		JobName:              "external",
		ReportType:           ReportTypeTrap,
		ReceivedRealtimeUsec: time.Now().UnixMicro(),
		TrapOID:              "1.3.6.1.6.3.1.1.5.3",
		TrapName:             "SNMPv2-MIB::coldStart",
		Category:             "state_change",
		Severity:             "warning",
		Message:              "External receiver interop test",
		SourceIP:             "192.0.2.10",
		SourceUDPPeer:        "192.0.2.10",
		SnmpVersion:          SnmpVersionV2c,
		PduType:              PduTypeTrap,
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

func newTestOTLPTrapWriter(t *testing.T, srv *otlpFixture, metrics *perJobMetrics, terminalErrors bool) *otlpTrapWriter {
	t.Helper()
	runtimeCfg, err := validateOTLPConfig(OTLPConfig{
		Enabled:        true,
		Endpoint:       srv.endpoint,
		RequestTimeout: "2s",
		FlushInterval:  "1h",
		BatchSize:      10,
		QueueCapacity:  10,
	})
	require.NoError(t, err)
	writer, err := newOTLPTrapWriterWithRuntimeConfig(t.Context(), "local", runtimeCfg, metrics, terminalErrors)
	require.NoError(t, err)
	return writer
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
