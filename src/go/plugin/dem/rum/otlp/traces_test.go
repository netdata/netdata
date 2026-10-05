// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"encoding/hex"
	"net"
	"sync"
	"testing"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeTraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	mu            sync.Mutex
	reqs          []*coltracepb.ExportTraceServiceRequest
	authorization []string
	resp          *coltracepb.ExportTraceServiceResponse
}

func (f *fakeTraceService) Export(
	ctx context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	md, _ := metadata.FromIncomingContext(ctx)
	f.authorization = append(f.authorization, md.Get("authorization")...)
	if f.resp != nil {
		return f.resp, nil
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (f *fakeTraceService) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func startTraceServer(t *testing.T, svc coltracepb.TraceServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })
	return lis.Addr().String()
}

func tracedBeacon() *beacon.Beacon {
	b := mkBeacon()
	b.ServiceName = "shop.example.com"
	b.Spans = []beacon.Span{{
		TraceID: "360b7dd27710ac98272283e6f1f29102", SpanID: "17c20d0ec32b813e",
		Name: "GET", Kind: 3, StartNS: 1, EndNS: 2, Scope: "@opentelemetry/instrumentation-fetch",
		Attrs: []beacon.SpanAttr{
			{Key: "url.full", Str: "https://api.example.com/orders/[REDACTED]"},
			{Key: "http.response.status_code", Int: 200, IsInt: true},
		},
	}}
	return b
}

// Browser spans preserve normalized values and IDs and carry the RUM resource.
func TestResourceSpans(t *testing.T) {
	e := newTraceExporter(t, "127.0.0.1:1", "s1", newRecCounters(), newRedactor(t, "hunter2"))
	rs := e.resourceSpans(tracedBeacon())
	res := map[string]string{}
	for _, kv := range rs.Resource.Attributes {
		res[kv.Key] = kv.GetValue().GetStringValue()
	}
	if res["service.name"] != "shop.example.com" || res["rum.site"] != "s1" || res["rum.session_id"] != "sess1" {
		t.Fatalf("resource = %v", res)
	}
	sp := rs.ScopeSpans[0].Spans[0]
	if hex.EncodeToString(sp.TraceId) != "360b7dd27710ac98272283e6f1f29102" ||
		hex.EncodeToString(sp.SpanId) != "17c20d0ec32b813e" {
		t.Fatalf("ids = %x %x", sp.TraceId, sp.SpanId)
	}
	for _, kv := range sp.Attributes {
		if kv.Key == "url.full" && kv.GetValue().GetStringValue() != "https://api.example.com/orders/[REDACTED]" {
			t.Fatal("normalized span attribute changed")
		}
		if kv.Key == "http.response.status_code" && kv.GetValue().GetIntValue() != 200 {
			t.Fatalf("int attr = %v", kv)
		}
	}
}

func TestTraceBatchPreservesPerBeaconResourcesAndScopes(t *testing.T) {
	remote := &fakeTraceService{}
	e := newTraceExporter(t, startTraceServer(t, remote), "s1", newRecCounters(), nil)
	first, second := tracedBeacon(), tracedBeacon()
	second.ServiceName = "other-service"
	second.SessionID = "another-session"
	second.Browser = "Firefox"
	second.Device = "mobile"
	second.Country = "US"
	extra := first.Spans[0]
	extra.Scope = "other-scope"
	extra.ScopeVersion = "2"
	first.Spans = append(first.Spans, extra, first.Spans[0])
	e.Ingest(first, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	e.Ingest(second, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.Run(ctx, finalContext(t))
	require.Equal(t, 1, remote.count())
	resources := remote.reqs[0].ResourceSpans
	require.Len(t, resources, 2)
	require.Len(t, resources[0].ScopeSpans, 2)
	require.Len(t, resources[0].ScopeSpans[0].Spans, 2)
	require.Equal(t, "other-scope", resources[0].ScopeSpans[1].Scope.Name)
	require.Equal(t, "2", resources[0].ScopeSpans[1].Scope.Version)
	attrs := map[string]string{}
	for _, attr := range resources[1].Resource.Attributes {
		attrs[attr.Key] = attr.Value.GetStringValue()
	}
	require.Equal(
		t,
		map[string]string{
			"service.name":        "other-service",
			"rum.site":            "s1",
			"rum.session_id":      "another-session",
			"rum.browser":         "Firefox",
			"rum.browser_version": "120",
			"rum.device":          "mobile",
			"rum.country":         "US",
		},
		attrs,
	)
}

func newTraceExporter(t *testing.T, endpoint, site string, c Counters, r *redact.Redactor) *Traces {
	t.Helper()
	e, err := NewTraces(context.Background(), destination(endpoint), site, c, r)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Close()) })
	return e
}
