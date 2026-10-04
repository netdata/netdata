// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Browser spans share the owning site's immutable trace destination.
const (
	spanQueueCap = 2000
	maxSpanBatch = 50
)

type spanItem struct {
	n  uint64
	rs *tracepb.ResourceSpans
}

type traceConnection struct {
	mu     sync.Mutex
	client coltracepb.TraceServiceClient
	conn   *grpc.ClientConn
}

// enqueueSpans builds this beacon's ResourceSpans and queues them.
func (e *Exporter) enqueueSpans(b *beacon.Beacon) {
	rs := e.resourceSpans(b)
	if rs == nil {
		return
	}
	n := uint64(len(b.Spans))
	if e.disabled && e.traceDestination == "" {
		e.counters.Add(agg.CounterSpansDropped, n)
		return
	}
	select {
	case e.spanCh <- spanItem{
		n:  n,
		rs: rs,
	}:
	default:
		e.counters.Add(agg.CounterSpansDropped, n)
	}
}

func (e *Exporter) resourceSpans(b *beacon.Beacon) *tracepb.ResourceSpans {
	service := b.ServiceName
	if service == "" {
		service = "rum:" + b.Site
	}
	res := []*commonpb.KeyValue{strAttr("service.name", service), strAttr("rum.site", b.Site)}
	for _, kv := range [][2]string{{"rum.browser", b.Browser}, {"rum.browser_version", b.BrowserVersion}, {"rum.device", b.Device}, {"rum.country", b.Country}, {"rum.session_id", b.SessionID}} {
		if kv[1] != "" {
			res = append(res, strAttr(kv[0], kv[1]))
		}
	}
	scopes := map[string]*tracepb.ScopeSpans{}
	var order []string
	for _, sp := range b.Spans {
		traceID, err1 := hex.DecodeString(sp.TraceID)
		spanID, err2 := hex.DecodeString(sp.SpanID)
		if err1 != nil || err2 != nil {
			continue
		}
		var parent []byte
		if sp.ParentSpanID != "" {
			parent, _ = hex.DecodeString(sp.ParentSpanID)
		}
		key := sp.Scope + "@" + sp.ScopeVersion
		ss, ok := scopes[key]
		if !ok {
			ss = &tracepb.ScopeSpans{
				Scope: &commonpb.InstrumentationScope{
					Name:    sp.Scope,
					Version: sp.ScopeVersion,
				},
			}
			scopes[key] = ss
			order = append(order, key)
		}
		attrs := make([]*commonpb.KeyValue, 0, len(sp.Attrs))
		for _, a := range sp.Attrs {
			if a.IsInt {
				attrs = append(
					attrs,
					&commonpb.KeyValue{
						Key: a.Key,
						Value: &commonpb.AnyValue{
							Value: &commonpb.AnyValue_IntValue{
								IntValue: a.Int,
							},
						},
					},
				)
			} else {
				attrs = append(attrs, strAttr(a.Key, e.redact(a.Str)))
			}
		}
		ss.Spans = append(ss.Spans, &tracepb.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			ParentSpanId:      parent,
			Name:              sp.Name,
			Kind:              tracepb.Span_SpanKind(sp.Kind),
			StartTimeUnixNano: sp.StartNS,
			EndTimeUnixNano:   sp.EndNS,
			Attributes:        attrs,
			Status: &tracepb.Status{
				Code:    tracepb.Status_StatusCode(sp.StatusCode),
				Message: e.redact(sp.StatusMessage),
			},
		})
	}
	if len(order) == 0 {
		return nil
	}
	rs := &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: res,
		},
	}
	for _, k := range order {
		rs.ScopeSpans = append(rs.ScopeSpans, scopes[k])
	}
	return rs
}

// runSpans batches queued spans until ctx is done, then flushes once more.
func (e *Exporter) runSpans(ctx context.Context, final func() context.Context) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]spanItem, 0, maxSpanBatch)
	flush := func(parent context.Context, timeout time.Duration) {
		if len(batch) > 0 {
			e.exportSpans(parent, batch, timeout)
			batch = batch[:0]
		}
	}
	for {
		select {
		case <-ctx.Done():
			parent := final()
			for {
				if len(batch) >= maxSpanBatch {
					flush(parent, exportTimeout)
				}
				select {
				case it := <-e.spanCh:
					batch = append(batch, it)
				default:
					flush(parent, exportTimeout)
					return
				}
			}
		case it := <-e.spanCh:
			batch = append(batch, it)
			if len(batch) >= maxSpanBatch {
				flush(ctx, exportTimeout)
			}
		case <-ticker.C:
			flush(ctx, exportTimeout)
		}
	}
}

func (e *Exporter) exportSpans(parent context.Context, batch []spanItem, timeout time.Duration) {
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: make([]*tracepb.ResourceSpans, 0, len(batch)),
	}
	var n uint64
	for _, it := range batch {
		req.ResourceSpans = append(req.ResourceSpans, it.rs)
		n += it.n
	}
	client := e.traceClient()
	if client == nil {
		e.counters.Add(agg.CounterSpansErrors, n)
		return
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if e.traceDestination == "" && e.authMD != nil {
		ctx = metadata.NewOutgoingContext(ctx, e.authMD)
	}
	resp, err := client.Export(ctx, req)
	if err != nil {
		e.counters.Add(agg.CounterSpansErrors, n)
		return
	}
	if ps := resp.GetPartialSuccess(); ps != nil && ps.RejectedSpans > 0 {
		exportLog.Warningf(
			"rum/otlp: span export to %q partially rejected: %d spans — %s",
			e.redact(e.traceDestination),
			ps.RejectedSpans,
			e.redact(ps.ErrorMessage),
		)
		e.counters.Add(agg.CounterSpansErrors, n)
		return
	}
	e.counters.Add(agg.CounterSpansSent, n)
}

// traceClient lazily opens the site's fixed destination. Empty uses the logs
// connection; an external endpoint is plaintext unless prefixed with https://.
func (e *Exporter) traceClient() coltracepb.TraceServiceClient {
	e.traces.mu.Lock()
	defer e.traces.mu.Unlock()
	if e.traces.client != nil {
		return e.traces.client
	}
	if e.traceDestination == "" {
		if e.conn == nil {
			return nil
		}
		e.traces.client = coltracepb.NewTraceServiceClient(e.conn)
		return e.traces.client
	}
	target, creds := strings.TrimPrefix(e.traceDestination, "https://"), insecure.NewCredentials()
	if strings.HasPrefix(e.traceDestination, "https://") {
		creds = credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		exportLog.Errorf("rum/otlp: span destination %s: %v", e.redact(e.traceDestination), e.redact(err.Error()))
		return nil
	}
	conn.Connect()
	e.traces.conn = conn
	e.traces.client = coltracepb.NewTraceServiceClient(conn)
	return e.traces.client
}

func (e *Exporter) closeTraceConn() {
	e.traces.mu.Lock()
	defer e.traces.mu.Unlock()
	if e.traces.conn != nil {
		_ = e.traces.conn.Close()
		e.traces.conn = nil
	}
}
