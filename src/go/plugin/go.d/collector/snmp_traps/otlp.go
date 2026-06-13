// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	defaultOTLPEndpoint       = "http://127.0.0.1:4317"
	defaultOTLPRequestTimeout = 5 * time.Second
	defaultOTLPFlushInterval  = 200 * time.Millisecond
	defaultOTLPBatchSize      = 512
	defaultOTLPQueueCapacity  = defaultQueueCapacity
	otlpServiceName           = "netdata-snmptrap"
)

type otlpRuntimeConfig struct {
	target         string
	insecure       bool
	headers        metadata.MD
	requestTimeout time.Duration
	flushInterval  time.Duration
	batchSize      int
	queueCapacity  int
}

type otlpEndpoint struct {
	target   string
	insecure bool
}

type otlpTrapWriter struct {
	client collogpb.LogsServiceClient
	conn   *grpc.ClientConn

	queue   chan *TrapEntry
	flushCh chan chan error
	closeCh chan chan error
	doneCh  chan struct{}

	headers        metadata.MD
	requestTimeout time.Duration
	flushInterval  time.Duration
	batchSize      int
	jobName        string
	metrics        *perJobMetrics
	terminalErrors bool

	mu       sync.Mutex
	closed   bool
	lastErr  error
	lastErrM sync.Mutex
}

func newOTLPTrapWriter(ctx context.Context, jobName string, cfg OTLPConfig, metrics *perJobMetrics) (*otlpTrapWriter, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	runtimeCfg, err := validateOTLPConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newOTLPTrapWriterWithRuntimeConfig(ctx, jobName, runtimeCfg, metrics, true)
}

func newOTLPTrapWriterWithRuntimeConfig(ctx context.Context, jobName string, runtimeCfg otlpRuntimeConfig, metrics *perJobMetrics, terminalErrors bool) (*otlpTrapWriter, error) {
	conn, client, err := newOTLPClient(ctx, runtimeCfg)
	if err != nil {
		return nil, err
	}

	w := &otlpTrapWriter{
		client:         client,
		conn:           conn,
		queue:          make(chan *TrapEntry, runtimeCfg.queueCapacity),
		flushCh:        make(chan chan error),
		closeCh:        make(chan chan error),
		doneCh:         make(chan struct{}),
		headers:        runtimeCfg.headers,
		requestTimeout: runtimeCfg.requestTimeout,
		flushInterval:  runtimeCfg.flushInterval,
		batchSize:      runtimeCfg.batchSize,
		jobName:        jobName,
		metrics:        metrics,
		terminalErrors: terminalErrors,
	}
	go w.worker()
	return w, nil
}

func validateOTLPConfig(cfg OTLPConfig) (otlpRuntimeConfig, error) {
	if !cfg.Enabled {
		return otlpRuntimeConfig{}, nil
	}

	ep, err := parseOTLPEndpoint(cfg.Endpoint)
	if err != nil {
		return otlpRuntimeConfig{}, fmt.Errorf("otlp.endpoint: %w", err)
	}

	requestTimeout, err := parseOTLPDuration(cfg.RequestTimeout, defaultOTLPRequestTimeout, "otlp.request_timeout")
	if err != nil {
		return otlpRuntimeConfig{}, err
	}
	flushInterval, err := parseOTLPDuration(cfg.FlushInterval, defaultOTLPFlushInterval, "otlp.flush_interval")
	if err != nil {
		return otlpRuntimeConfig{}, err
	}

	batchSize := cfg.BatchSize
	if batchSize == 0 {
		batchSize = defaultOTLPBatchSize
	}
	if batchSize < 0 {
		return otlpRuntimeConfig{}, fmt.Errorf("otlp.batch_size must be positive, got %d", cfg.BatchSize)
	}

	queueCapacity := cfg.QueueCapacity
	if queueCapacity == 0 {
		queueCapacity = defaultOTLPQueueCapacity
	}
	if queueCapacity < 0 {
		return otlpRuntimeConfig{}, fmt.Errorf("otlp.queue_capacity must be positive, got %d", cfg.QueueCapacity)
	}

	headers, err := buildOTLPMetadata(cfg.Headers)
	if err != nil {
		return otlpRuntimeConfig{}, err
	}

	return otlpRuntimeConfig{
		target:         ep.target,
		insecure:       ep.insecure,
		headers:        headers,
		requestTimeout: requestTimeout,
		flushInterval:  flushInterval,
		batchSize:      batchSize,
		queueCapacity:  queueCapacity,
	}, nil
}

func parseOTLPDuration(raw string, fallback time.Duration, name string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return d, nil
}

func parseOTLPEndpoint(raw string) (otlpEndpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultOTLPEndpoint
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return otlpEndpoint{}, err
		}
		var insecureTransport bool
		switch u.Scheme {
		case "http":
			insecureTransport = true
		case "https":
			insecureTransport = false
		default:
			return otlpEndpoint{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
		}
		if u.Host == "" {
			return otlpEndpoint{}, errors.New("missing host")
		}
		if u.Path != "" {
			return otlpEndpoint{}, errors.New("path is not supported for OTLP/gRPC endpoints")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return otlpEndpoint{}, errors.New("query and fragment are not supported for OTLP/gRPC endpoints")
		}
		if err := validateOTLPTarget(u.Host); err != nil {
			return otlpEndpoint{}, err
		}
		return otlpEndpoint{target: u.Host, insecure: insecureTransport}, nil
	}

	if strings.ContainsAny(raw, "/?#") {
		return otlpEndpoint{}, errors.New("bare endpoint must be host:port")
	}
	if err := validateOTLPTarget(raw); err != nil {
		return otlpEndpoint{}, err
	}
	return otlpEndpoint{target: raw, insecure: true}, nil
}

func otlpTargetIsLoopback(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func validateOTLPTarget(target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("endpoint must be host:port: %w", err)
	}
	if host == "" || port == "" {
		return errors.New("endpoint must include host and port")
	}
	return nil
}

func buildOTLPMetadata(headers map[string]string) (metadata.MD, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		k := strings.ToLower(strings.TrimSpace(key))
		if err := validateOTLPMetadataKey(k); err != nil {
			return nil, fmt.Errorf("otlp.headers[%q]: %w", key, err)
		}
		normalized[k] = value
	}
	return metadata.New(normalized), nil
}

func validateOTLPMetadataKey(key string) error {
	if key == "" {
		return errors.New("header name is empty")
	}
	if strings.HasPrefix(key, "grpc-") {
		return errors.New("grpc-* metadata headers are reserved")
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid metadata header character %q", r)
	}
	return nil
}

func newOTLPClient(ctx context.Context, cfg otlpRuntimeConfig) (*grpc.ClientConn, collogpb.LogsServiceClient, error) {
	var creds credentials.TransportCredentials
	if cfg.insecure {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}

	conn, err := grpc.NewClient(cfg.target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP gRPC client: %w", err)
	}
	if err := waitForOTLPReady(ctx, conn, cfg.requestTimeout); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	client := collogpb.NewLogsServiceClient(conn)
	if err := otlpExport(ctx, client, cfg.headers, cfg.requestTimeout, &collogpb.ExportLogsServiceRequest{}); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("OTLP preflight export: %w", err)
	}
	return conn, client, nil
}

func waitForOTLPReady(ctx context.Context, conn *grpc.ClientConn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return errors.New("OTLP gRPC connection shut down")
		}
		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("OTLP gRPC connection not ready: %w", ctx.Err())
		}
	}
}

func otlpExport(ctx context.Context, client collogpb.LogsServiceClient, headers metadata.MD, timeout time.Duration, req *collogpb.ExportLogsServiceRequest) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if len(headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, headers.Copy())
	}

	resp, err := client.Export(ctx, req)
	if err != nil {
		return err
	}
	if ps := resp.GetPartialSuccess(); ps != nil {
		if ps.GetRejectedLogRecords() > 0 || ps.GetErrorMessage() != "" {
			return fmt.Errorf("OTLP partial success rejected=%d message=%q", ps.GetRejectedLogRecords(), ps.GetErrorMessage())
		}
	}
	return nil
}

func (w *otlpTrapWriter) worker() {
	batch := make([]*TrapEntry, 0, w.batchSize)
	reportedFailed := 0
	var activeReplyCh chan error

	defer func() {
		if v := recover(); v != nil {
			err := fmt.Errorf("SNMP trap OTLP writer panic: %v", v)
			w.setClosedWithError(err)
			w.accountWorkerPanicFailures(batch, reportedFailed)
			if w.conn != nil {
				_ = w.conn.Close()
			}
			if activeReplyCh != nil {
				activeReplyCh <- err
			}
		}
		close(w.doneCh)
	}()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		queueCh := w.queue
		if len(batch) >= w.batchSize {
			queueCh = nil
		}

		select {
		case entry := <-queueCh:
			batch = append(batch, entry)
			if len(batch) >= w.batchSize {
				_ = w.exportPending(&batch, &reportedFailed)
			}

		case <-ticker.C:
			_ = w.exportPending(&batch, &reportedFailed)

		case replyCh := <-w.flushCh:
			activeReplyCh = replyCh
			err := w.drainQueue(&batch, &reportedFailed)
			if exportErr := w.exportPending(&batch, &reportedFailed); err == nil {
				err = exportErr
			}
			replyCh <- err
			activeReplyCh = nil

		case replyCh := <-w.closeCh:
			activeReplyCh = replyCh
			err := w.drainQueue(&batch, &reportedFailed)
			if exportErr := w.exportPending(&batch, &reportedFailed); err == nil {
				err = exportErr
			}
			replyCh <- err
			activeReplyCh = nil
			return
		}
	}
}

func (w *otlpTrapWriter) Write(entry *TrapEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errWriterClosed
	}
	select {
	case w.queue <- entry:
		return nil
	default:
		return errQueueFull
	}
}

func (w *otlpTrapWriter) Flush() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errWriterClosed
	}

	replyCh := make(chan error, 1)
	select {
	case w.flushCh <- replyCh:
		w.mu.Unlock()
	case <-w.doneCh:
		w.mu.Unlock()
		return errWriterClosed
	}
	return <-replyCh
}

func (w *otlpTrapWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		w.lastErrM.Lock()
		defer w.lastErrM.Unlock()
		return w.lastErr
	}
	w.closed = true

	replyCh := make(chan error, 1)
	select {
	case w.closeCh <- replyCh:
		w.mu.Unlock()
	case <-w.doneCh:
		w.mu.Unlock()
		w.lastErrM.Lock()
		defer w.lastErrM.Unlock()
		return w.lastErr
	}
	err := <-replyCh
	<-w.doneCh
	if closeErr := w.conn.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		w.setLastErr(err)
	}
	return err
}

func (w *otlpTrapWriter) drainQueue(batch *[]*TrapEntry, reportedFailed *int) error {
	var firstErr error
	for {
		select {
		case entry := <-w.queue:
			*batch = append(*batch, entry)
			if len(*batch) >= w.batchSize {
				if err := w.exportPending(batch, reportedFailed); err != nil {
					if firstErr == nil {
						firstErr = err
					}
					// During explicit flush/close drains, every queued entry must
					// be accounted for. Keep draining in bounded chunks after a
					// failed export instead of carrying an oversized failed batch.
					*batch = (*batch)[:0]
					*reportedFailed = 0
				}
			}
		default:
			return firstErr
		}
	}
}

func (w *otlpTrapWriter) exportPending(batch *[]*TrapEntry, reportedFailed *int) error {
	if len(*batch) == 0 {
		return nil
	}
	req, err := buildOTLPExportRequest(w.jobName, *batch)
	if err != nil {
		w.incNewOTLPExportFailures(*batch, reportedFailed)
		w.setLastErr(err)
		return err
	}
	if err := otlpExport(context.Background(), w.client, w.headers, w.requestTimeout, req); err != nil {
		w.incNewOTLPExportFailures(*batch, reportedFailed)
		w.setLastErr(err)
		return err
	}
	*batch = (*batch)[:0]
	*reportedFailed = 0
	return nil
}

func (w *otlpTrapWriter) incNewOTLPExportFailures(batch []*TrapEntry, reportedFailed *int) {
	if len(batch) <= *reportedFailed {
		return
	}
	w.incOTLPExportFailed(uint64(len(batch) - *reportedFailed))
	if w.metrics != nil {
		for _, entry := range batch[*reportedFailed:] {
			if w.terminalErrors {
				w.metrics.recordWriteFailure(entry, "otlp_export_failed")
			} else {
				w.metrics.recordSourceError(entry, "otlp_export_failed")
			}
		}
	}
	*reportedFailed = len(batch)
}

func (w *otlpTrapWriter) accountWorkerPanicFailures(batch []*TrapEntry, reportedFailed int) {
	if reportedFailed < 0 {
		reportedFailed = 0
	}
	pending := make([]*TrapEntry, 0, len(batch)-min(reportedFailed, len(batch))+len(w.queue))
	if reportedFailed < len(batch) {
		pending = append(pending, batch[reportedFailed:]...)
	}
	for {
		select {
		case entry := <-w.queue:
			pending = append(pending, entry)
		default:
			reported := 0
			w.incNewOTLPExportFailures(pending, &reported)
			return
		}
	}
}

func (w *otlpTrapWriter) incOTLPExportFailed(n uint64) {
	if w.metrics != nil {
		w.metrics.addError("otlp_export_failed", n)
	}
}

func (w *otlpTrapWriter) setLastErr(err error) {
	w.lastErrM.Lock()
	if w.lastErr == nil {
		w.lastErr = err
	}
	w.lastErrM.Unlock()
}

func (w *otlpTrapWriter) setClosedWithError(err error) {
	w.setLastErr(err)
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

func buildOTLPExportRequest(jobName string, entries []*TrapEntry) (*collogpb.ExportLogsServiceRequest, error) {
	records := make([]*logpb.LogRecord, 0, len(entries))
	for _, entry := range entries {
		record, err := trapEntryToOTLPLogRecord(entry)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return &collogpb.ExportLogsServiceRequest{
		ResourceLogs: []*logpb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					otlpKVString("service.name", otlpServiceName),
					otlpKVString("service.instance.id", jobName),
				},
			},
			ScopeLogs: []*logpb.ScopeLogs{{
				LogRecords: records,
			}},
		}},
	}, nil
}

func trapEntryToOTLPLogRecord(entry *TrapEntry) (*logpb.LogRecord, error) {
	if entry == nil {
		return nil, errNilEntry
	}

	ts := uint64(0)
	if entry.ReceivedRealtimeUsec > 0 {
		ts = uint64(entry.ReceivedRealtimeUsec) * 1000
	}
	sevNum, sevText, severitySlug := otlpSeverity(entry.Severity)

	return &logpb.LogRecord{
		TimeUnixNano:         ts,
		ObservedTimeUnixNano: ts,
		SeverityNumber:       sevNum,
		SeverityText:         sevText,
		Body:                 otlpStringValue(entry.Message),
		EventName:            otlpEventName(entry),
		Attributes:           otlpTrapAttributes(entry, severitySlug),
	}, nil
}

func otlpSeverity(severity Severity) (logpb.SeverityNumber, string, string) {
	switch severity {
	case "emerg":
		return logpb.SeverityNumber_SEVERITY_NUMBER_FATAL, "FATAL", "emerg"
	case "alert":
		return logpb.SeverityNumber_SEVERITY_NUMBER_ERROR3, "ERROR3", "alert"
	case "crit":
		return logpb.SeverityNumber_SEVERITY_NUMBER_ERROR2, "ERROR2", "crit"
	case "err":
		return logpb.SeverityNumber_SEVERITY_NUMBER_ERROR, "ERROR", "err"
	case "warning":
		return logpb.SeverityNumber_SEVERITY_NUMBER_WARN, "WARN", "warning"
	case "info":
		return logpb.SeverityNumber_SEVERITY_NUMBER_INFO, "INFO", "info"
	case "debug":
		return logpb.SeverityNumber_SEVERITY_NUMBER_DEBUG, "DEBUG", "debug"
	default:
		return logpb.SeverityNumber_SEVERITY_NUMBER_INFO2, "INFO2", "notice"
	}
}

func otlpEventName(entry *TrapEntry) string {
	if entry.ReportType == ReportTypeDedupSummary {
		return "snmp.trap.deduplication_summary"
	}
	if entry.ReportType == ReportTypeDecodeError {
		return "snmp.trap.decode_error"
	}
	cat := string(entry.Category)
	if cat == "" {
		cat = "unknown"
	}
	return "snmp.trap." + cat
}

func otlpTrapAttributes(entry *TrapEntry, severitySlug string) []*commonpb.KeyValue {
	reportType := string(entry.ReportType)
	if reportType == "" {
		reportType = string(ReportTypeTrap)
	}

	attrs := []*commonpb.KeyValue{
		otlpKVString("snmp.trap.report_type", reportType),
	}

	if entry.ReportType != ReportTypeDedupSummary {
		sourceIP := entry.SourceIP
		if sourceIP == "" {
			sourceIP = entry.SourceUDPPeer
		}
		udpPeer := entry.SourceUDPPeer
		if udpPeer == "" {
			udpPeer = entry.SourceIP
		}
		attrs = append(attrs, otlpKVString("network.peer.address", udpPeer))
		attrs = append(attrs, otlpKVString("snmp.source.ip", sourceIP))
		attrs = appendStringAttr(attrs, "snmp.version", string(entry.SnmpVersion))
		attrs = appendStringAttr(attrs, "snmp.trap.oid", entry.TrapOID)
		attrs = appendStringAttr(attrs, "snmp.trap.name", entry.TrapName)
		category := string(entry.Category)
		if category == "" {
			category = "unknown"
		}
		attrs = append(attrs, otlpKVString("snmp.trap.category", category))
		attrs = append(attrs, otlpKVString("snmp.trap.severity", severitySlug))
		attrs = appendStringAttr(attrs, "snmp.trap.pdu_type", string(entry.PduType))
		attrs = appendStringAttr(attrs, "snmp.source.reverse_dns", entry.ReverseDNS)
		attrs = appendStringAttr(attrs, "snmp.device.hostname", entry.DeviceHostname)
		attrs = appendStringAttr(attrs, "snmp.device.vendor", entry.DeviceVendor)
		attrs = appendStringAttr(attrs, "netdata.nidl.node", entry.SourceVnodeID)
		attrs = appendStringAttr(attrs, "netdata.topology.interface", entry.TopologyInterface)
		attrs = appendStringAttr(attrs, "netdata.topology.neighbors", entry.TopologyNeighbors)
	}
	if entry.DecodeError != nil {
		appendDecodeErrorOTLPAttributes(&attrs, entry.DecodeError)
	}

	if entry.SummaryCounts != nil {
		attrs = append(attrs,
			otlpKVInt("snmp.trap.suppressed_count", entry.SummaryCounts.TotalSuppressed),
			otlpKVInt("snmp.trap.suppressed_fingerprints", entry.SummaryCounts.Fingerprints),
			otlpKVInt("snmp.trap.report_period_sec", entry.SummaryCounts.PeriodSec),
		)
	}

	if v := otlpVarbindsValue(entry); v != nil {
		attrs = append(attrs, &commonpb.KeyValue{Key: "snmp.varbinds", Value: v})
	}

	for _, key := range sortedMapKeys(entry.Labels) {
		val := entry.Labels[key]
		attrs = append(attrs, otlpKVString("trap."+strings.ToLower(key), val))
	}

	return attrs
}

func appendDecodeErrorOTLPAttributes(attrs *[]*commonpb.KeyValue, info *DecodeErrorInfo) {
	*attrs = appendStringAttr(*attrs, "snmp.trap.decode_error.kind", info.Kind)
	*attrs = appendStringAttr(*attrs, "snmp.trap.decode_error.message", info.Error)
	*attrs = append(*attrs, otlpKVInt("snmp.trap.packet_size", int64(info.PacketSize)))
	*attrs = appendStringAttr(*attrs, "snmp.trap.packet_sha256", info.PacketSHA256)
	if info.SourceUDPPort > 0 {
		*attrs = append(*attrs, otlpKVInt("network.peer.port", int64(info.SourceUDPPort)))
	}
	*attrs = appendStringAttr(*attrs, "netdata.trap.listener", info.Listener)
	*attrs = appendStringAttr(*attrs, "snmp.engine_id", info.EngineID)
}

func appendStringAttr(attrs []*commonpb.KeyValue, key, val string) []*commonpb.KeyValue {
	if val == "" {
		return attrs
	}
	return append(attrs, otlpKVString(key, val))
}

func otlpVarbindsValue(entry *TrapEntry) *commonpb.AnyValue {
	if entry.DecodeError != nil {
		info := entry.DecodeError
		values := []*commonpb.KeyValue{
			otlpKVString("kind", info.Kind),
			otlpKVString("error", info.Error),
			otlpKVInt("packet_size", int64(info.PacketSize)),
			otlpKVString("packet_sha256", info.PacketSHA256),
		}
		if info.SourceUDPPort > 0 {
			values = append(values, otlpKVInt("source_udp_port", int64(info.SourceUDPPort)))
		}
		if info.Listener != "" {
			values = append(values, otlpKVString("listener", info.Listener))
		}
		if info.SnmpVersion != "" {
			values = append(values, otlpKVString("snmp_version", info.SnmpVersion))
		}
		if info.EngineID != "" {
			values = append(values, otlpKVString("engine_id", info.EngineID))
		}
		return otlpKVListValue(values)
	}

	if entry.SummaryCounts != nil {
		sc := entry.SummaryCounts
		values := []*commonpb.KeyValue{
			otlpKVInt("total_suppressed", sc.TotalSuppressed),
			otlpKVInt("period_sec", sc.PeriodSec),
			otlpKVInt("fingerprints", sc.Fingerprints),
		}
		if len(sc.ByTrap) > 0 {
			keys := make([]string, 0, len(sc.ByTrap))
			for key := range sc.ByTrap {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			byTrap := make([]*commonpb.KeyValue, 0, len(keys))
			for _, key := range keys {
				byTrap = append(byTrap, otlpKVInt(key, sc.ByTrap[key]))
			}
			values = append(values, otlpKV("by_trap", otlpKVListValue(byTrap)))
		}
		return otlpKVListValue(values)
	}

	if len(entry.Varbinds) == 0 {
		return nil
	}
	seenKeys := make(map[string]int)
	values := make([]*commonpb.KeyValue, 0, len(entry.Varbinds))
	for _, vb := range entry.Varbinds {
		if isSensitiveTrapVarbind(vb) {
			continue
		}
		key := vb.Name
		if key == "" {
			key = vb.OID
		}
		if key == "" {
			continue
		}
		seenKeys[key]++
		if seenKeys[key] > 1 {
			key = fmt.Sprintf("%s#%d", key, seenKeys[key])
		}
		fields := []*commonpb.KeyValue{
			otlpKVString("oid", vb.OID),
			otlpKVString("type", string(vb.Type)),
			otlpKV("value", otlpAnyValue(canonicalVarbindValue(vb.Value))),
		}
		if vb.Enum != "" {
			fields = append(fields, otlpKVString("enum", vb.Enum))
		}
		values = append(values, otlpKV(key, otlpKVListValue(fields)))
	}
	if len(values) == 0 {
		return nil
	}
	return otlpKVListValue(values)
}

func otlpKVString(key, val string) *commonpb.KeyValue {
	return otlpKV(key, otlpStringValue(val))
}

func otlpKVInt(key string, val int64) *commonpb.KeyValue {
	return otlpKV(key, &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: val}})
}

func otlpKV(key string, val *commonpb.AnyValue) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: val}
}

func otlpStringValue(val string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: val}}
}

func otlpKVListValue(values []*commonpb.KeyValue) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: values}}}
}

func otlpAnyValue(val any) *commonpb.AnyValue {
	switch v := val.(type) {
	case nil:
		return &commonpb.AnyValue{}
	case string:
		return otlpStringValue(v)
	case int:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(v)}}
	case int64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}
	case uint64:
		if v <= math.MaxInt64 {
			return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(v)}}
		}
		return otlpStringValue(fmt.Sprintf("%d", v))
	case float64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}
	case bool:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}
	default:
		return otlpStringValue(fmt.Sprintf("%v", v))
	}
}
