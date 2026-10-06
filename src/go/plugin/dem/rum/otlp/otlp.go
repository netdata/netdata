// SPDX-License-Identifier: GPL-3.0-or-later

// Package otlp exports RUM observations (document activations, vital updates, JS errors,
// custom events, console logs) and browser spans to independent OTLP/gRPC
// receivers. It talks the wire protocol directly via
// generated go.opentelemetry.io/proto/otlp types — no OTel SDK, so there
// is no global tracer/meter state to fight with a host process.
package otlp

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc/metadata"
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
	// ShutdownFlushTimeout bounds the final flush: the plugin has a 3s
	// SIGTERM grace and a black-holed endpoint must not consume it.
	ShutdownFlushTimeout = time.Second
)

// Counters receives the rum.otlp chart bookkeeping (sent/dropped/errors
// dimensions). *aggregate.Aggregator satisfies this directly.
type Counters interface {
	Add(counter string, n uint64)
}

// Logs owns one site's enabled event-log destination and queue.
type Logs struct {
	*transport
	client collogspb.LogsServiceClient
	now    func() time.Time
	ch     chan queued
}

func NewLogs(
	ctx context.Context,
	cfg config.Destination,
	siteName string,
	counters Counters,
	redactor *redact.Redactor,
) (*Logs, error) {
	t, err := newTransport(ctx, cfg, siteName, counters, redactor)
	if err != nil {
		return nil, err
	}
	return &Logs{
		transport: t,
		client:    collogspb.NewLogsServiceClient(t.conn),
		now:       time.Now,
		ch:        make(chan queued, queueCap),
	}, nil
}

// Run drives the batching/export loop until ctx is done, then flushes
// whatever is queued once more and returns. Callers run it in its own
// goroutine; Close releases the gRPC connection afterwards. The owner joins
// producers before cancellation and supplies one shared, bounded final context.
func (e *Logs) Run(ctx context.Context, final func() context.Context) {
	e.conn.Connect()
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]queued, 0, maxBatch)
	// An unattempted normal batch belongs to the final drain. An attempted
	// export is consumed even on failure, since retrying could duplicate data.
	flush := func(parent context.Context) bool {
		if len(batch) == 0 {
			return true
		}
		if !e.export(parent, batch, exportTimeout) {
			return false
		}
		batch = batch[:0]
		return true
	}
normal:
	for {
		select {
		case <-ctx.Done():
			break normal
		case it, ok := <-e.ch:
			if !ok {
				break normal
			}
			batch = append(batch, it)
			if len(batch) >= maxBatch && !flush(ctx) {
				break normal
			}
		case <-ticker.C:
			if !flush(ctx) {
				break normal
			}
		}
	}
	// Producers have been joined by the owner. Use its shared budget once,
	// including for a batch whose normal export never began.
	parent := final()
	flushFinal := func() {
		if !flush(parent) {
			e.counters.Add(aggregate.CounterOTLPDropped, uint64(len(batch)))
			batch = batch[:0]
		}
	}
	for {
		if len(batch) >= maxBatch {
			flushFinal()
		}
		select {
		case it, ok := <-e.ch:
			if !ok {
				flushFinal()
				return
			}
			batch = append(batch, it)
		default:
			flushFinal()
			return
		}
	}
}

// Ingest consumes an aggregation result, builds records and enqueues them,
// counting queue overflow as dropped. Only enabled exporters are constructed.
func (e *Logs) Ingest(b *beacon.Beacon, result aggregate.Result) {
	if !result.Accepted || b.Site != e.siteName {
		return
	}
	if !result.Investigated {
		return // measured, but its session is not investigated
	}
	if result.Observation == nil {
		return
	}
	recs := e.build(result.Observation, result.PageView)
	for _, q := range recs {
		select {
		case e.ch <- q:
		default:
			e.counters.Add(aggregate.CounterOTLPDropped, 1)
		}
	}
}

// export reports whether a send was attempted, including failed attempts.
func (e *Logs) export(parent context.Context, batch []queued, timeout time.Duration) bool {
	if parent.Err() != nil {
		return false
	}
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
		e.failed("logs", err)
		e.counters.Add(aggregate.CounterOTLPErrors, uint64(len(batch)))
		return true
	}
	rejected := rejectedCount(resp.GetPartialSuccess().GetRejectedLogRecords(), uint64(len(batch)))
	if rejected > 0 {
		e.rejected("logs", rejected)
	}
	e.counters.Add(aggregate.CounterOTLPErrors, rejected)
	e.counters.Add(aggregate.CounterOTLPSent, uint64(len(batch))-rejected)
	return true
}
