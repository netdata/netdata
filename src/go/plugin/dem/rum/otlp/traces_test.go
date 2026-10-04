// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"encoding/hex"
	"net"
	"sync"
	"testing"

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

// Independently configured exporters keep destinations and session starts isolated.
func TestSpansReachTheirDestination(t *testing.T) {
	local, remote := &fakeTraceService{}, &fakeTraceService{}
	logs := &fakeLogsService{}
	localCounters, remoteCounters := newRecCounters(), newRecCounters()
	localExporter := configuredExporter(
		t,
		Config{
			Enabled:  "yes",
			Endpoint: startTraceServer(t, local),
		},
		"s1",
		"",
		localCounters,
		nil,
	)
	remoteExporter := configuredExporter(
		t,
		Config{
			Enabled:  "yes",
			Endpoint: startServer(t, logs),
		},
		"s2",
		startTraceServer(t, remote),
		remoteCounters,
		nil,
	)
	first, second := tracedBeacon(), tracedBeacon()
	second.Site = "s2"
	localExporter.Ingest(first, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	remoteExporter.Ingest(second, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	sampled := tracedBeacon()
	localExporter.Ingest(sampled, aggregate.Result{
		Accepted: true,
	})
	// Each site sees the same session identifier for the first time independently.
	require.Len(t, localExporter.ch, 1)
	require.Len(t, remoteExporter.ch, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	localExporter.Run(ctx)
	remoteExporter.Run(ctx)
	require.Equal(t, 1, local.count())
	require.Equal(t, 1, remote.count())
	require.EqualValues(t, 1, localCounters.snapshot()["spans_sent"])
	require.EqualValues(t, 1, remoteCounters.snapshot()["spans_sent"])
	require.EqualValues(t, 1, remoteCounters.snapshot()["otlp_sent"])
	require.Equal(t, "s1", resourceSite(local.reqs[0]))
	require.Equal(t, "s2", resourceSite(remote.reqs[0]))
	require.Equal(t, "s2", logs.requests()[0].ResourceLogs[0].Resource.Attributes[1].Value.GetStringValue())
}

func resourceSite(req *coltracepb.ExportTraceServiceRequest) string {
	for _, attr := range req.ResourceSpans[0].Resource.Attributes {
		if attr.Key == "rum.site" {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func TestExternalTracingWithLogsDisabled(t *testing.T) {
	remote := &fakeTraceService{}
	counters := newRecCounters()
	e := configuredExporter(t, Config{
		Enabled: "no",
	}, "s1", startTraceServer(t, remote), counters, nil)
	e.Ingest(tracedBeacon(), aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.Run(ctx)
	require.Equal(t, 1, remote.count())
	require.EqualValues(t, 1, counters.snapshot()["spans_sent"])
	require.EqualValues(t, 1, counters.snapshot()["otlp_dropped"])
}

func TestTraceBatchPreservesPerBeaconResourcesAndScopes(t *testing.T) {
	remote := &fakeTraceService{}
	e := configuredExporter(t, Config{
		Enabled: "no",
	}, "s1", startTraceServer(t, remote), newRecCounters(), nil)
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
	e.Run(ctx)
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

func TestTraceAuthenticationStaysWithLocalEndpoint(t *testing.T) {
	local, remote := &fakeTraceService{}, &fakeTraceService{}
	cfg := Config{
		Enabled:   "yes",
		Endpoint:  startTraceServer(t, local),
		AuthToken: "local-token",
	}
	localExporter := configuredExporter(t, cfg, "s1", "", newRecCounters(), nil)
	remoteExporter := configuredExporter(t, cfg, "s2", startTraceServer(t, remote), newRecCounters(), nil)
	first, second := tracedBeacon(), tracedBeacon()
	first.SessionID = ""
	second.SessionID = ""
	second.Site = "s2"
	localExporter.Ingest(first, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	remoteExporter.Ingest(second, aggregate.Result{
		Accepted:     true,
		Investigated: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	localExporter.Run(ctx)
	remoteExporter.Run(ctx)
	require.Equal(t, 1, local.count())
	require.Equal(t, 1, remote.count())
	require.Equal(t, []string{"Bearer local-token"}, local.authorization)
	require.Empty(t, remote.authorization)
}
