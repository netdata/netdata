// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"

	"encoding/hex"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"

	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"google.golang.org/grpc/metadata"
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

// Traces owns one site's enabled browser-span destination and queue.
type Traces struct {
	*transport
	client coltracepb.TraceServiceClient
	spanCh chan spanItem
}

func NewTraces(
	ctx context.Context,
	cfg config.Destination,
	siteName string,
	counters Counters,
	redactor *redact.Redactor,
) (*Traces, error) {
	t, err := newTransport(ctx, cfg, siteName, counters, redactor)
	if err != nil {
		return nil, err
	}
	return &Traces{
		transport: t,
		client:    coltracepb.NewTraceServiceClient(t.conn),
		spanCh:    make(chan spanItem, spanQueueCap),
	}, nil
}

func (e *Traces) Ingest(b *beacon.Beacon, result aggregate.Result) {
	if !result.Accepted || !result.Investigated || b.Site != e.siteName || len(b.Spans) == 0 {
		return
	}
	rs := e.resourceSpans(b)
	if rs == nil {
		return
	}
	var n uint64
	for _, ss := range rs.ScopeSpans {
		n += uint64(len(ss.Spans))
	}
	select {
	case e.spanCh <- spanItem{
		n:  n,
		rs: rs,
	}:
	default:
		e.counters.Add(aggregate.CounterSpansDropped, n)
	}
}

func (e *Traces) resourceSpans(b *beacon.Beacon) *tracepb.ResourceSpans {
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
				attrs = append(attrs, strAttr(a.Key, a.Str))
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
				Message: sp.StatusMessage,
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

// Run batches spans until cancellation, then drains using the owner's shared
// final context. The owner joins producers before cancellation.
func (e *Traces) Run(ctx context.Context, final func() context.Context) {
	e.conn.Connect()
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]spanItem, 0, maxSpanBatch)
	// An unattempted normal batch belongs to the final drain. An attempted
	// export is consumed even on failure, since retrying could duplicate data.
	flush := func(parent context.Context) bool {
		if len(batch) == 0 {
			return true
		}
		if !e.exportSpans(parent, batch, exportTimeout) {
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
		case it, ok := <-e.spanCh:
			if !ok {
				break normal
			}
			batch = append(batch, it)
			if len(batch) >= maxSpanBatch && !flush(ctx) {
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
			var n uint64
			for _, it := range batch {
				n += it.n
			}
			e.counters.Add(aggregate.CounterSpansDropped, n)
			batch = batch[:0]
		}
	}
	for {
		if len(batch) >= maxSpanBatch {
			flushFinal()
		}
		select {
		case it, ok := <-e.spanCh:
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

func (e *Traces) exportSpans(parent context.Context, batch []spanItem, timeout time.Duration) bool {
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: make([]*tracepb.ResourceSpans, 0, len(batch)),
	}
	var n uint64
	for _, it := range batch {
		req.ResourceSpans = append(req.ResourceSpans, it.rs)
		n += it.n
	}
	if parent.Err() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if e.authMD != nil {
		ctx = metadata.NewOutgoingContext(ctx, e.authMD)
	}
	resp, err := e.client.Export(ctx, req)
	if err != nil {
		e.failed("traces", err)
		e.counters.Add(aggregate.CounterSpansErrors, n)
		return true
	}
	rejected := rejectedCount(resp.GetPartialSuccess().GetRejectedSpans(), n)
	if rejected > 0 {
		e.rejected("traces", rejected)
	}
	e.counters.Add(aggregate.CounterSpansErrors, rejected)
	e.counters.Add(aggregate.CounterSpansSent, n-rejected)
	return true
}
