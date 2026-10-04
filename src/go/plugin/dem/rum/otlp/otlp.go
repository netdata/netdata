// SPDX-License-Identifier: GPL-3.0-or-later

// Package otlp exports RUM events (pageviews, JS errors, session starts,
// custom events, console logs) as OTLP logs over gRPC to the local
// otel.plugin. It talks the wire protocol directly via
// generated go.opentelemetry.io/proto/otlp types — no OTel SDK, so there
// is no global tracer/meter state to fight with a host process.
package otlp

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
)

var exportLog = logger.New()

// service.name resource attribute used by RUM exports.
const serviceName = "netdata-rum"

// Batching bounds: flush at 100 records or 1s, whichever
// comes first; a bounded queue drops on overflow rather than blocking
// the collector.
const (
	maxBatch      = 100
	batchPeriod   = time.Second
	queueCap      = 10000
	exportTimeout = 5 * time.Second
	// shutdownFlushTimeout bounds the final flush: the plugin has a 3s
	// SIGTERM grace and a black-holed endpoint must not consume it.
	shutdownFlushTimeout = time.Second
)

// Counters receives the rum.otlp chart bookkeeping (sent/dropped/errors
// dimensions). *agg.Aggregator satisfies this directly.
type Counters interface {
	Add(counter string, n uint64)
}

// Exporter batches beacon-derived records and exports them as OTLP logs.
// It implements beacon.Sink so it can sit in the collector's fan-out
// alongside the in-memory aggregator.
type Exporter struct {
	siteName         string
	traceDestination string
	conn             *grpc.ClientConn
	client           collogspb.LogsServiceClient
	counters         Counters
	redactor         *secrets.Redactor
	authMD           metadata.MD

	disabled bool // otel.enabled == "no": events have nowhere to go
	sessions *sessionTracker
	now      func() time.Time

	ch     chan queued
	spanCh chan spanItem
	traces traceConnection
}

// New builds an exporter for the site OTLP configuration. grpc.NewClient
// is nonblocking; Connect starts the first attempt in the background,
// with reconnect backoff from 1s to 60s. Configuration errors, such as bad
// TLS material, prevent the site runtime from starting.
func New(
	ctx context.Context,
	cfg Config,
	siteName string,
	traceDestination string,
	counters Counters,
	redactor *secrets.Redactor,
) (*Exporter, error) {
	e := &Exporter{
		siteName:         siteName,
		traceDestination: traceDestination,
		counters:         counters,
		redactor:         redactor,
		sessions:         newSessionTracker(),
		now:              time.Now,
		ch:               make(chan queued, queueCap),
		spanCh:           make(chan spanItem, spanQueueCap),
	}
	if cfg.Enabled == "no" {
		e.disabled = true
		return e, nil
	}
	creds, err := dialCreds(ctx, cfg)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(cfg.Endpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   60 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp: dial %s: %w", cfg.Endpoint, err)
	}
	conn.Connect() // kick off the first attempt now; never blocks
	e.conn = conn
	e.client = collogspb.NewLogsServiceClient(conn)
	if cfg.AuthToken != "" {
		e.authMD = metadata.Pairs("authorization", "Bearer "+cfg.AuthToken)
	}
	return e, nil
}

// dialCreds builds transport credentials: plaintext unless TLS options
// are set. A client certificate and key must be supplied together.
func dialCreds(ctx context.Context, cfg Config) (credentials.TransportCredentials, error) {
	if cfg.TLSCert == "" && cfg.TLSKey == "" && cfg.TLSCA == "" {
		return insecure.NewCredentials(), nil
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return nil, fmt.Errorf("otlp: tls_cert and tls_key must be set together")
	}
	tlsCfg, err := tlscfg.NewTLSConfig(
		ctx,
		tlscfg.TLSConfig{
			TLSCA:   cfg.TLSCA,
			TLSCert: cfg.TLSCert,
			TLSKey:  cfg.TLSKey,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("otlp: TLS configuration: %w", err)
	}
	tlsCfg.MinVersion = tls.VersionTLS12
	return credentials.NewTLS(tlsCfg), nil
}

// ValidateConfig checks transport material without starting a gRPC client.
func ValidateConfig(ctx context.Context, cfg Config) error {
	if cfg.Enabled == "no" {
		return nil
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("otlp endpoint is required")
	}
	_, err := dialCreds(ctx, cfg)
	return err
}

// Run drives the batching/export loop until ctx is done, then flushes
// whatever is queued once more and returns. Callers run it in its own
// goroutine; Close releases the gRPC connection afterwards.
func (e *Exporter) Run(ctx context.Context) {
	var wg sync.WaitGroup
	var once sync.Once
	var finalCtx context.Context
	var finalCancel context.CancelFunc
	// Logs and spans share one finalization budget.
	final := func() context.Context {
		once.Do(func() { finalCtx, finalCancel = context.WithTimeout(context.Background(), shutdownFlushTimeout) })
		return finalCtx
	}
	wg.Go(func() { e.runSpans(ctx, final) })
	defer func() {
		wg.Wait()
		if finalCancel != nil {
			finalCancel()
		}
	}()
	if e.disabled {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]queued, 0, maxBatch)
	// Periodic exports run under ctx so cancellation aborts one already in
	// flight; the final flush after cancellation gets its own short budget.
	flush := func(parent context.Context, timeout time.Duration) {
		if len(batch) == 0 {
			return
		}
		e.export(parent, batch, timeout)
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			parent := final()
			// Producers have been joined by the owner before cancellation.
			for {
				if len(batch) >= maxBatch {
					flush(parent, exportTimeout)
				}
				select {
				case q := <-e.ch:
					batch = append(batch, q)
				default:
					flush(parent, exportTimeout)
					return
				}
			}
		case q, ok := <-e.ch:
			if !ok {
				flush(final(), exportTimeout)
				return
			}
			batch = append(batch, q)
			if len(batch) >= maxBatch {
				flush(ctx, exportTimeout)
			}
		case <-ticker.C:
			flush(ctx, exportTimeout)
		}
	}
}

// Close releases the gRPC connection. Safe to call once Run has
// returned (or concurrently — the conn is only touched here).
func (e *Exporter) Close() error {
	e.closeTraceConn()
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}

// Ingest implements beacon.Sink: it builds records and enqueues them,
// counting queue overflow as dropped. When OTLP is disabled, records
// are counted as dropped because there is no fallback export path.
func (e *Exporter) Ingest(b *beacon.Beacon) {
	if b.Site != e.siteName {
		return
	}
	if b.SampledOut {
		return // measured, but its session is not investigated
	}
	if len(b.Spans) > 0 {
		e.enqueueSpans(b)
	}
	recs := e.build(b)
	if len(recs) == 0 {
		return
	}
	if e.disabled {
		e.counters.Add(agg.CounterOTLPDropped, uint64(len(recs)))
		return
	}
	for _, q := range recs {
		select {
		case e.ch <- q:
		default:
			e.counters.Add(agg.CounterOTLPDropped, 1)
		}
	}
}

// Reject is a no-op: rejected beacons never produced a valid record.
func (e *Exporter) Reject(string, string) {}

func (e *Exporter) redact(s string) string {
	if e.redactor == nil || s == "" {
		return s
	}
	return e.redactor.Apply(s)
}

// export sends one batch with the owning site's resource attributes.
func (e *Exporter) export(parent context.Context, batch []queued, timeout time.Duration) {
	recs := make([]*logspb.LogRecord, 0, len(batch))
	for _, q := range batch {
		recs = append(recs, q.rec)
	}
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					strAttr("service.name", serviceName), strAttr("rum.site", e.siteName),
				},
			},
			ScopeLogs: []*logspb.ScopeLogs{
				{Scope: &commonpb.InstrumentationScope{
					Name: "digital-experience-rum",
				}, LogRecords: recs},
			},
		}},
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if e.authMD != nil {
		ctx = metadata.NewOutgoingContext(ctx, e.authMD)
	}
	resp, err := e.client.Export(ctx, req)
	if err != nil {
		e.counters.Add(agg.CounterOTLPErrors, uint64(len(batch)))
		return
	}
	if ps := resp.GetPartialSuccess(); ps != nil && ps.RejectedLogRecords > 0 {
		// OTLP cannot identify rejected records; count the whole batch as errors.
		exportLog.Warningf(
			"rum/otlp: export partial rejection: %d records — %s",
			ps.RejectedLogRecords,
			e.redact(ps.ErrorMessage),
		)
		e.counters.Add(agg.CounterOTLPErrors, uint64(len(batch)))
		return
	}
	e.counters.Add(agg.CounterOTLPSent, uint64(len(batch)))
}
