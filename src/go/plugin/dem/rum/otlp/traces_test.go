// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"encoding/hex"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type fakeTraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	mu   sync.Mutex
	reqs []*coltracepb.ExportTraceServiceRequest
	resp *coltracepb.ExportTraceServiceResponse
}

func (f *fakeTraceService) Export(
	_ context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
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

func startTraceServer(t *testing.T, svc *fakeTraceService) string {
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
			{Key: "url.full", Str: "https://api.example.com/orders/hunter2"},
			{Key: "http.response.status_code", Int: 200, IsInt: true},
		},
	}}
	return b
}

// Browser spans keep their ids, lose secrets, and carry the RUM resource.
func TestResourceSpans(t *testing.T) {
	e := newExporter(t, "127.0.0.1:1", newRecCounters(), newRedactor(t, "hunter2"))
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
		if kv.Key == "url.full" && kv.GetValue().GetStringValue() == "https://api.example.com/orders/hunter2" {
			t.Fatal("secret not redacted from span attribute")
		}
		if kv.Key == "http.response.status_code" && kv.GetValue().GetIntValue() != 200 {
			t.Fatalf("int attr = %v", kv)
		}
	}
}

// Spans go to otel.endpoint by default and to export_to when a site sets it.
func TestSpansReachTheirDestination(t *testing.T) {
	local, remote := &fakeTraceService{}, &fakeTraceService{}
	localAddr, remoteAddr := startTraceServer(t, local), startTraceServer(t, remote)
	counters := newRecCounters()
	e := newExporter(t, localAddr, counters, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()

	e.Ingest(tracedBeacon())
	b := tracedBeacon()
	b.TraceExportTo = remoteAddr
	e.Ingest(b)
	sampled := tracedBeacon()
	sampled.SampledOut = true
	e.Ingest(sampled)

	deadline := time.Now().Add(5 * time.Second)
	for (local.count() == 0 || remote.count() == 0) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if local.count() != 1 || remote.count() != 1 {
		t.Fatalf("exports: local %d, remote %d", local.count(), remote.count())
	}
	if got := counters.snapshot()["s1/spans_sent"]; got != 2 {
		t.Fatalf("spans_sent = %d, want 2 (sampled-out session skipped)", got)
	}
}
