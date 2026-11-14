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

func (e *Traces) Ingest(result aggregate.Result) {
	if !result.Investigated || len(result.Observation.Spans) == 0 {
		return
	}
	rs := e.resourceSpans(result.Observation)
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
		e.counters.Add(CounterSpansDropped, n)
	}
}

func (e *Traces) resourceSpans(b *beacon.Beacon) *tracepb.ResourceSpans {
	service := b.ServiceName
	if service == "" {
		service = "rum:" + b.Site
	}
	res := []*commonpb.KeyValue{strAttr("service.name", service), strAttr("rum.site", b.Site)}
	for _, kv := range [][2]string{{"dem.vnode_guid", b.VnodeGUID}, {"rum.browser", b.Browser}, {"rum.browser_version", b.BrowserVersion}, {"rum.device", b.Device}, {"rum.country", b.Country}, {"rum.session_id", b.SessionID}, {"rum.experience_id", b.ExperienceID}, {"rum.view_id", b.ViewID}, {"rum.view", b.View}, {"rum.document_entry", b.PageGroup}} {
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

// Run exports queued spans until ctx is done, then drains them once with the
// owner's shared, bounded final context (see runQueue).
func (e *Traces) Run(ctx context.Context, final func() context.Context) {
	e.conn.Connect()
	runQueue(
		ctx,
		final,
		e.spanCh,
		maxSpanBatch,
		func(ctx context.Context, batch []spanItem) bool { return e.exportSpans(ctx, batch, exportTimeout) },
		func(batch []spanItem) { e.counters.Add(CounterSpansDropped, spanCount(batch)) },
	)
}

func spanCount(batch []spanItem) uint64 {
	var n uint64
	for _, it := range batch {
		n += it.n
	}
	return n
}

// exportSpans reports whether a send was attempted, including failed attempts.
func (e *Traces) exportSpans(parent context.Context, batch []spanItem, timeout time.Duration) bool {
	if parent.Err() != nil {
		return false
	}
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: make([]*tracepb.ResourceSpans, 0, len(batch)),
	}
	for _, it := range batch {
		req.ResourceSpans = append(req.ResourceSpans, it.rs)
	}
	n := spanCount(batch)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if e.authMD != nil {
		ctx = metadata.NewOutgoingContext(ctx, e.authMD)
	}
	resp, err := e.client.Export(ctx, req)
	if err != nil {
		e.failed("traces", err)
		e.counters.Add(CounterSpansErrors, n)
		return true
	}
	rejected := rejectedCount(resp.GetPartialSuccess().GetRejectedSpans(), n)
	if rejected > 0 {
		e.rejected("traces", rejected)
	}
	e.counters.Add(CounterSpansErrors, rejected)
	e.counters.Add(CounterSpansSent, n-rejected)
	return true
}
