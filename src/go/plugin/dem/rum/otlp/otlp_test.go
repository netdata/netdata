// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/require"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

// ---- test doubles ----

// recCounters records Add() calls (the rum.otlp chart dims).
type recCounters struct {
	mu sync.Mutex
	n  map[string]uint64 // counter -> n
}

func newRecCounters() *recCounters {
	return &recCounters{
		n: map[string]uint64{},
	}
}

func (r *recCounters) Add(counter string, n uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n[counter] += n
}

func (r *recCounters) snapshot() map[string]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]uint64, len(r.n))
	for k, v := range r.n {
		out[k] = v
	}
	return out
}

// fakeLogsService is an in-process OTLP LogsService the exporter dials
// over real gRPC (loopback TCP), so tests exercise the actual wire path.
type fakeLogsService struct {
	collogspb.UnimplementedLogsServiceServer
	mu   sync.Mutex
	reqs []*collogspb.ExportLogsServiceRequest
	resp *collogspb.ExportLogsServiceResponse
	err  error
}

func (f *fakeLogsService) Export(
	_ context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	resp, err := f.resp, f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if resp == nil {
		resp = &collogspb.ExportLogsServiceResponse{}
	}
	return resp, nil
}

func (f *fakeLogsService) requests() []*collogspb.ExportLogsServiceRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*collogspb.ExportLogsServiceRequest, len(f.reqs))
	copy(out, f.reqs)
	return out
}

func startServer(t *testing.T, svc collogspb.LogsServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	collogspb.RegisterLogsServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })
	return lis.Addr().String()
}

func newRedactor(t *testing.T, secretValue string) *redact.Redactor {
	t.Helper()
	return redact.NewRedactor(secretValue)
}

func newExporter(t *testing.T, endpoint string, counters Counters, redact *redact.Redactor) *Logs {
	t.Helper()
	e, err := NewLogs(context.Background(), destination(endpoint), "s1", counters, redact)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.now = func() time.Time { return t0 }
	t.Cleanup(func() { _ = e.Close() })
	return e
}

// ---- record building ----

func mkBeacon() *beacon.Beacon {
	return &beacon.Beacon{
		Site:           "s1",
		Received:       t0,
		SessionID:      "sess1",
		Path:           "/pricing",
		PageGroup:      "/pricing",
		Browser:        "Chrome",
		BrowserVersion: "120",
		OS:             "Linux",
		Device:         "desktop",
		Country:        "GR",
	}
}

type simpleRec struct {
	body     string
	severity string
	attrs    map[string]string
}

func simplify(q queued) simpleRec {
	attrs := map[string]string{}
	for _, kv := range q.rec.Attributes {
		attrs[kv.Key] = kv.GetValue().GetStringValue()
	}
	return simpleRec{
		body:     q.rec.GetBody().GetStringValue(),
		severity: q.rec.SeverityText,
		attrs:    attrs,
	}
}

func TestBuild(t *testing.T) {
	tests := map[string]struct {
		pageView bool
		beacon   func() *beacon.Beacon
		want     []simpleRec
	}{
		"pageview": {
			pageView: true,
			beacon: func() *beacon.Beacon {
				b := mkBeacon()

				return b
			},
			want: []simpleRec{{
				body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"no session id never starts a session": {
			pageView: true,
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.SessionID = ""

				return b
			},
			want: []simpleRec{{
				body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"normalized error preserves severity ERROR": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Errors = []beacon.Error{
					{
						Type:    "TypeError",
						Message: "token=[REDACTED] undefined",
						Stack:   "at f (x.js:1:1) [REDACTED]",
						Time:    t0,
					},
				}
				return b
			},
			want: []simpleRec{{
				body: "TypeError: token=[REDACTED] undefined @ /pricing", severity: "ERROR",
				attrs: map[string]string{
					"rum.type": "error", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
					"error.type": "TypeError", "error.message": "token=[REDACTED] undefined", "error.stack": "at f (x.js:1:1) [REDACTED]",
				},
			}},
		},
		"event carries normalized name and attrs": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Events = []beacon.Event{
					{Name: "checkout", Attrs: map[string]string{"coupon": "[REDACTED]"}, Time: t0},
				}
				return b
			},
			want: []simpleRec{{
				body: "event checkout @ /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "event", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
					"event.name": "checkout", "event.attr.coupon": "[REDACTED]",
				},
			}},
		},
		"app version/environment/view and error fingerprint": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.AppVersion, b.Environment, b.View = "1.2.3", "production", "checkout-view"
				b.Errors = []beacon.Error{{Type: "TypeError", Message: "boom", Fingerprint: "abcdef012345", Time: t0}}
				return b
			},
			want: []simpleRec{
				{
					body: "TypeError: boom @ /pricing", severity: "ERROR",
					attrs: map[string]string{
						"rum.type": "error", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing", "page.view": "checkout-view",
						"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
						"app.version": "1.2.3", "app.environment": "production",
						"error.type": "TypeError", "error.message": "boom", "error.stack": "", "error.fingerprint": "abcdef012345",
					},
				},
			},
		},
		"console log": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Logs = []beacon.Log{{Level: "warn", Message: "slow request", Time: t0}}
				return b
			},
			want: []simpleRec{{
				body: "console warn: slow request", severity: "WARN",
				attrs: map[string]string{
					"rum.type": "console", "console.level": "warn", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := &Logs{
				now: func() time.Time { return t0 },
			}
			var got []simpleRec
			for _, q := range e.build(tc.beacon(), tc.pageView) {
				got = append(got, simplify(q))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s:\n got  %#v\n want %#v", name, got, tc.want)
			}
		})
	}
}

func TestNoSyntheticSessionStart(t *testing.T) {
	e := &Logs{
		transport: &transport{},
		now:       func() time.Time { return t0 },
	}
	require.Empty(t, e.build(mkBeacon(), false))
}

// ---- export over real gRPC ----

func TestExport(t *testing.T) {
	tests := map[string]struct {
		resp    *collogspb.ExportLogsServiceResponse
		failRPC bool
		want    map[string]uint64
	}{
		"success counts sent": {
			want: map[string]uint64{"otlp_sent": 1, "otlp_errors": 0},
		},
		"partial success counts errors not sent": {
			resp: &collogspb.ExportLogsServiceResponse{
				PartialSuccess: &collogspb.ExportLogsPartialSuccess{
					RejectedLogRecords: 1,
					ErrorMessage:       "boom",
				},
			},
			want: map[string]uint64{"otlp_errors": 1, "otlp_sent": 0},
		},
		"rpc failure counts errors": {
			failRPC: true,
			want:    map[string]uint64{"otlp_errors": 1},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := &fakeLogsService{
				resp: tc.resp,
			}
			if tc.failRPC {
				svc.err = context.DeadlineExceeded
			}
			addr := startServer(t, svc)
			counters := newRecCounters()
			e := newExporter(t, addr, counters, nil)

			// One admitted pageview produces one record.
			recs := e.build(mkBeacon(), true)
			e.export(context.Background(), recs, exportTimeout)

			if got := counters.snapshot(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s: counters = %#v, want %#v", name, got, tc.want)
			}
			if !tc.failRPC {
				reqs := svc.requests()
				if len(reqs) != 1 {
					t.Fatalf("expected 1 export call, got %d", len(reqs))
				}
				if len(reqs[0].ResourceLogs) != 1 || reqs[0].ResourceLogs[0].Resource == nil {
					t.Fatalf("expected one resource with attrs: %#v", reqs[0])
				}
				attrs := map[string]string{}
				for _, kv := range reqs[0].ResourceLogs[0].Resource.Attributes {
					attrs[kv.Key] = kv.GetValue().GetStringValue()
				}
				want := map[string]string{"service.name": "netdata-rum", "rum.site": "s1"}
				if !reflect.DeepEqual(attrs, want) {
					t.Fatalf("resource attrs = %#v, want %#v", attrs, want)
				}
			}
		})
	}
}

func TestIngestDropsOnFullQueue(t *testing.T) {
	counters := newRecCounters()
	e := newExporter(t, "127.0.0.1:1", counters, nil) // unreachable; nothing drains e.ch
	e.ch = make(chan queued, 1)

	b1 := mkBeacon() // One pageview fills the queue.
	e.Ingest(b1, aggregate.Result{
		Accepted:     true,
		Investigated: true,
		PageView:     true,
	})
	b2 := mkBeacon()
	b2.SessionID = "sess2"
	e.Ingest(b2, aggregate.Result{
		Accepted:     true,
		Investigated: true,
		PageView:     true,
	})

	want := map[string]uint64{"otlp_dropped": 1}
	if got := counters.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("counters = %#v, want %#v", got, want)
	}
}

// TestShutdownFlushIsBounded: with a black-holed endpoint and a queued
// record, cancelling Run must return within the shutdown flush budget, not
// the regular export timeout (the agent's SIGTERM grace is 3s).
func TestShutdownFlushIsBounded(t *testing.T) {
	e := newExporter(t, "192.0.2.1:4317", newRecCounters(), nil)
	e.Ingest(mkBeacon(), aggregate.Result{
		Accepted:     true,
		Investigated: true,
		PageView:     true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx, finalContext(t))
		close(done)
	}()
	time.Sleep(100 * time.Millisecond) // let Run take the record into its batch
	cancel()
	start := time.Now()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancellation")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("shutdown flush took %v, want about %v", d, ShutdownFlushTimeout)
	}
	_ = e.Close()
}

func TestTruncateKeepsRuneBoundaries(t *testing.T) {
	tests := map[string]struct {
		in   string
		n    int
		want string
	}{
		"short string untouched":     {in: "héllo", n: 10, want: "héllo"},
		"cut inside a rune backs up": {in: "héllo", n: 2, want: "h"},
		"cut on a boundary keeps it": {in: "héllo", n: 3, want: "hé"},
		"ascii exact":                {in: "abc", n: 3, want: "abc"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := beacon.Truncate(tc.in, tc.n); got != tc.want {
				t.Fatalf("truncate(%q,%d)=%q want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

// Investigate sampling: a beacon whose session is sampled out is
// still measured, but produces no events.
func TestIngestSkipsSampledOutBeacons(t *testing.T) {
	counters := newRecCounters()
	e := newExporter(t, "127.0.0.1:1", counters, nil)
	b := mkBeacon()
	e.Ingest(b, aggregate.Result{
		Accepted: true,
	})
	if n := len(e.ch); n != 0 {
		t.Fatalf("queued %d records for a sampled-out beacon", n)
	}
	if got := counters.snapshot(); len(got) != 0 {
		t.Fatalf("sampling out is not a drop: counters = %#v", got)
	}
}

func destination(endpoint string) config.Destination {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	return config.Destination{
		Endpoint: &endpoint,
	}
}
func finalContext(t *testing.T) func() context.Context {
	t.Helper()
	var once sync.Once
	var ctx context.Context
	return func() context.Context {
		once.Do(func() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), ShutdownFlushTimeout)
			t.Cleanup(cancel)
		})
		return ctx
	}
}
