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

// Browser spans are batched like log records but by destination:
// "" is otel.endpoint (the logs connection), anything else a site's
// tracing.export_to.
const (
	spanQueueCap = 2000
	maxSpanBatch = 50
)

type spanItem struct {
	site, dest string
	n          uint64
	rs         *tracepb.ResourceSpans
}

type traceClients struct {
	mu      sync.Mutex
	clients map[string]coltracepb.TraceServiceClient
	conns   []*grpc.ClientConn
}

// enqueueSpans builds this beacon's ResourceSpans and queues them.
func (e *Exporter) enqueueSpans(b *beacon.Beacon) {
	rs := e.resourceSpans(b)
	if rs == nil {
		return
	}
	n := uint64(len(b.Spans))
	if e.disabled && b.TraceExportTo == "" {
		e.counters.Add(b.Site, agg.CounterSpansDropped, n)
		return
	}
	select {
	case e.spanCh <- spanItem{
		site: b.Site,
		dest: b.TraceExportTo,
		n:    n,
		rs:   rs,
	}:
	default:
		e.counters.Add(b.Site, agg.CounterSpansDropped, n)
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
	byDest := map[string][]spanItem{}
	for _, it := range batch {
		byDest[it.dest] = append(byDest[it.dest], it)
	}
	for dest, items := range byDest {
		req := &coltracepb.ExportTraceServiceRequest{}
		for _, it := range items {
			req.ResourceSpans = append(req.ResourceSpans, it.rs)
		}
		count := func(counter string) {
			for _, it := range items {
				e.counters.Add(it.site, counter, it.n)
			}
		}
		client := e.traceClient(dest)
		if client == nil {
			count(agg.CounterSpansErrors)
			continue
		}
		ctx, cancel := context.WithTimeout(parent, timeout)
		if dest == "" && e.authMD != nil {
			ctx = metadata.NewOutgoingContext(ctx, e.authMD)
		}
		resp, err := client.Export(ctx, req)
		cancel()
		if err != nil {
			count(agg.CounterSpansErrors)
			continue
		}
		if ps := resp.GetPartialSuccess(); ps != nil && ps.RejectedSpans > 0 {
			exportLog.Warningf(
				"rum/otlp: span export to %q partially rejected: %d spans — %s",
				e.redact(dest),
				ps.RejectedSpans,
				e.redact(ps.ErrorMessage),
			)
			count(agg.CounterSpansErrors)
			continue
		}
		count(agg.CounterSpansSent)
	}
}

// traceClient returns the trace client for dest, dialing a site's
// export_to on first use (plaintext host:port, or TLS for https://).
func (e *Exporter) traceClient(dest string) coltracepb.TraceServiceClient {
	if dest == "" {
		if e.conn == nil {
			return nil
		}
		dest = "\x00local"
	}
	e.traces.mu.Lock()
	defer e.traces.mu.Unlock()
	if c, ok := e.traces.clients[dest]; ok {
		return c
	}
	if e.traces.clients == nil {
		e.traces.clients = map[string]coltracepb.TraceServiceClient{}
	}
	if dest == "\x00local" {
		c := coltracepb.NewTraceServiceClient(e.conn)
		e.traces.clients[dest] = c
		return c
	}
	target, creds := strings.TrimPrefix(dest, "https://"), insecure.NewCredentials()
	if strings.HasPrefix(dest, "https://") {
		creds = credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		exportLog.Errorf("rum/otlp: span destination %s: %v", e.redact(dest), e.redact(err.Error()))
		return nil
	}
	conn.Connect()
	e.traces.conns = append(e.traces.conns, conn)
	c := coltracepb.NewTraceServiceClient(conn)
	e.traces.clients[dest] = c
	return c
}

func (e *Exporter) closeTraceConns() {
	e.traces.mu.Lock()
	defer e.traces.mu.Unlock()
	for _, c := range e.traces.conns {
		_ = c.Close()
	}
	e.traces.conns = nil
}
