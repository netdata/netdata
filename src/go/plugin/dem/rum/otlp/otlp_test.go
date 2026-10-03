// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"bytes"
	"context"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

// ---- test doubles ----

// recCounters records Add() calls (the rum.otlp chart dims).
type recCounters struct {
	mu sync.Mutex
	n  map[string]uint64 // site+"/"+counter -> n
}

func newRecCounters() *recCounters {
	return &recCounters{
		n: map[string]uint64{},
	}
}

func (r *recCounters) Add(site, counter string, n uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n[site+"/"+counter] += n
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

func startServer(t *testing.T, svc *fakeLogsService) string {
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

func newRedactor(t *testing.T, secretValue string) *secrets.Redactor {
	t.Helper()
	return secrets.NewRedactor(secretValue)
}

func newExporter(t *testing.T, endpoint string, counters Counters, redact *secrets.Redactor) *Exporter {
	t.Helper()
	e, err := New(context.Background(), config.OTelCfg{
		Enabled:  "yes",
		Endpoint: endpoint,
	}, counters, redact)
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
	site     string
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
		site:     q.site,
		body:     q.rec.GetBody().GetStringValue(),
		severity: q.rec.SeverityText,
		attrs:    attrs,
	}
}

// sessionStart is the record every case using mkBeacon()'s default
// SessionID produces first, since each subtest starts a fresh tracker.
var sessionStart = simpleRec{
	site:     "s1",
	body:     "session start",
	severity: "INFO",
	attrs: map[string]string{
		"rum.type": "session_start", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
		"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
	},
}

func TestBuild(t *testing.T) {
	tests := map[string]struct {
		beacon func() *beacon.Beacon
		secret string // known secret value the redactor must strip
		want   []simpleRec
	}{
		"pageview": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.PageView = true
				return b
			},
			want: []simpleRec{sessionStart, {
				site: "s1", body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"session start fires once per TTL": {
			beacon: func() *beacon.Beacon { return mkBeacon() },
			want:   []simpleRec{sessionStart},
		},
		"no session id never starts a session": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.SessionID = ""
				b.PageView = true
				return b
			},
			want: []simpleRec{{
				site: "s1", body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"error is redacted and severity ERROR": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Errors = []beacon.Error{
					{
						Type:    "TypeError",
						Message: "token=SEKRET1234 undefined",
						Stack:   "at f (x.js:1:1) SEKRET1234",
						Time:    t0,
					},
				}
				return b
			},
			secret: "SEKRET1234",
			want: []simpleRec{sessionStart, {
				site: "s1", body: "TypeError: token=[REDACTED] undefined @ /pricing", severity: "ERROR",
				attrs: map[string]string{
					"rum.type": "error", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
					"error.type": "TypeError", "error.message": "token=[REDACTED] undefined", "error.stack": "at f (x.js:1:1) [REDACTED]",
				},
			}},
		},
		"event carries name and redacted attrs": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Events = []beacon.Event{
					{Name: "checkout", Attrs: map[string]string{"coupon": "SEKRET1234"}, Time: t0},
				}
				return b
			},
			secret: "SEKRET1234",
			want: []simpleRec{sessionStart, {
				site: "s1", body: "event checkout @ /pricing", severity: "INFO",
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
					site: "s1", body: "session start", severity: "INFO",
					attrs: map[string]string{
						"rum.type": "session_start", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing", "page.view": "checkout-view",
						"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
						"app.version": "1.2.3", "app.environment": "production",
					},
				},
				{
					site: "s1", body: "TypeError: boom @ /pricing", severity: "ERROR",
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
			want: []simpleRec{sessionStart, {
				site: "s1", body: "console warn: slow request", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "console", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := &Exporter{
				sessions: newSessionTracker(),
				now:      func() time.Time { return t0 },
				redactor: newRedactor(t, tc.secret),
			}
			var got []simpleRec
			for _, q := range e.build(tc.beacon()) {
				got = append(got, simplify(q))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s:\n got  %#v\n want %#v", name, got, tc.want)
			}
		})
	}
}

func TestBuildSessionStartOnlyOnce(t *testing.T) {
	e := &Exporter{
		sessions: newSessionTracker(),
		now:      func() time.Time { return t0 },
	}
	first := e.build(mkBeacon())
	if len(first) != 1 || first[0].rec.Attributes[0].GetValue().GetStringValue() != "session_start" {
		t.Fatalf("first beacon of a session must emit session_start: %#v", first)
	}
	second := e.build(mkBeacon())
	if len(second) != 0 {
		t.Fatalf("second beacon within TTL must not re-emit session_start: %#v", second)
	}
}

// ---- export over real gRPC ----

func TestExport(t *testing.T) {
	tests := map[string]struct {
		resp    *collogspb.ExportLogsServiceResponse
		failRPC bool
		want    map[string]uint64
	}{
		"success counts sent": {
			want: map[string]uint64{"s1/otlp_sent": 1},
		},
		"partial success counts errors not sent": {
			resp: &collogspb.ExportLogsServiceResponse{
				PartialSuccess: &collogspb.ExportLogsPartialSuccess{
					RejectedLogRecords: 1,
					ErrorMessage:       "boom",
				},
			},
			want: map[string]uint64{"s1/otlp_errors": 1},
		},
		"rpc failure counts errors": {
			failRPC: true,
			want:    map[string]uint64{"s1/otlp_errors": 1},
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

			// a fresh session on a fresh tracker yields exactly one
			// session_start record — enough to exercise one export call.
			recs := e.build(mkBeacon())
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

	b1 := mkBeacon() // produces session_start + (no pageview) = 1 record, fills the queue
	e.Ingest(b1)
	b2 := mkBeacon()
	b2.SessionID = "sess2" // a different session so it still produces a record to drop
	e.Ingest(b2)

	want := map[string]uint64{"s1/otlp_dropped": 1}
	if got := counters.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("counters = %#v, want %#v", got, want)
	}
}

func TestDisabledCountsDrops(t *testing.T) {
	counters := newRecCounters()
	e, err := New(context.Background(), config.OTelCfg{
		Enabled: "no",
	}, counters, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.conn != nil {
		t.Fatal("disabled exporter must never dial")
	}
	b := mkBeacon()
	b.PageView = true
	e.Ingest(b)
	got := counters.snapshot()
	if got["s1/otlp_dropped"] != 2 { // session_start + pageview
		t.Fatalf("counters = %#v, want 2 dropped", got)
	}
}

func TestDialCredsBothOrNeither(t *testing.T) {
	if _, err := dialCreds(context.Background(), config.OTelCfg{
		TLSCert: "/x.pem",
	}); err == nil {
		t.Fatal("tls_cert without tls_key must error")
	}
	if _, err := dialCreds(context.Background(), config.OTelCfg{}); err != nil {
		t.Fatalf("plaintext default must not error: %v", err)
	}
}

// TestShutdownFlushIsBounded: with a black-holed endpoint and a queued
// record, cancelling Run must return within the shutdown flush budget, not
// the regular export timeout (the agent's SIGTERM grace is 3s).
func TestShutdownFlushIsBounded(t *testing.T) {
	e := newExporter(t, "192.0.2.1:4317", newRecCounters(), nil)
	e.Ingest(mkBeacon())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
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
		t.Fatalf("shutdown flush took %v, want about %v", d, shutdownFlushTimeout)
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
			if got := truncate(tc.in, tc.n); got != tc.want {
				t.Fatalf("truncate(%q,%d)=%q want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

// TestInFlightPeriodicExportAbortsOnCancel: a ticker-driven export already
// blocked on a black-holed endpoint must abort when Run's ctx is cancelled,
// not run out its 5s export timeout (the agent's SIGTERM grace is 3s).
func TestInFlightPeriodicExportAbortsOnCancel(t *testing.T) {
	e := newExporter(t, "192.0.2.1:4317", newRecCounters(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()
	e.Ingest(mkBeacon())
	time.Sleep(batchPeriod + 300*time.Millisecond) // ticker fired: export now in flight
	cancel()
	start := time.Now()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancelling an in-flight export")
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("in-flight export held shutdown for %v", d)
	}
	_ = e.Close()
}

// Investigate sampling: a beacon whose session is sampled out is
// still measured, but produces no events.
func TestIngestSkipsSampledOutBeacons(t *testing.T) {
	counters := newRecCounters()
	e := newExporter(t, "127.0.0.1:1", counters, nil)
	b := mkBeacon()
	b.SampledOut = true
	e.Ingest(b)
	if n := len(e.ch); n != 0 {
		t.Fatalf("queued %d records for a sampled-out beacon", n)
	}
	if got := counters.snapshot(); len(got) != 0 {
		t.Fatalf("sampling out is not a drop: counters = %#v", got)
	}
}

func TestShutdownDrainsAcceptedQueues(t *testing.T) {
	logs := &fakeLogsService{}
	traces := &fakeTraceService{}
	counters := newRecCounters()
	e := newExporter(t, startServer(t, logs), counters, nil)
	traceDest := startTraceServer(t, traces)
	var logRecords int
	for range 300 {
		b := tracedBeacon()
		b.PageView = true
		b.TraceExportTo = traceDest
		e.Ingest(b)
	}
	logRecords = len(e.ch)
	require.Greater(t, logRecords, maxBatch)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.Run(ctx)
	got := counters.snapshot()
	require.EqualValues(t, logRecords, got["s1/"+agg.CounterOTLPSent])
	require.EqualValues(t, 300, got["s1/"+agg.CounterSpansSent])
	require.Empty(t, e.ch)
	require.Empty(t, e.spanCh)
}

func TestPartialRejectionDiagnosticsRedactSecrets(t *testing.T) {
	const secret = "endpoint-secret-value"
	var output bytes.Buffer
	previous := exportLog
	exportLog = logger.NewWithWriter(&output)
	t.Cleanup(func() { exportLog = previous })

	logs := &fakeLogsService{
		resp: &collogspb.ExportLogsServiceResponse{
			PartialSuccess: &collogspb.ExportLogsPartialSuccess{
				RejectedLogRecords: 1,
				ErrorMessage:       "rejected " + secret,
			},
		},
	}
	counters := newRecCounters()
	e := newExporter(t, startServer(t, logs), counters, newRedactor(t, secret))
	e.export(context.Background(), e.build(mkBeacon()), exportTimeout)
	require.Equal(t, uint64(1), counters.snapshot()["s1/otlp_errors"])
	require.Contains(t, output.String(), "partial rejection")
	require.NotContains(t, output.String(), secret)
	require.Contains(t, output.String(), "[REDACTED]")
	output.Reset()

	traces := &fakeTraceService{
		resp: &coltracepb.ExportTraceServiceResponse{
			PartialSuccess: &coltracepb.ExportTracePartialSuccess{
				RejectedSpans: 1,
				ErrorMessage:  "rejected " + secret,
			},
		},
	}
	b := tracedBeacon()
	b.TraceExportTo = startTraceServer(t, traces)
	e.exportSpans(context.Background(), []spanItem{{
		site: b.Site, dest: b.TraceExportTo, rs: e.resourceSpans(b), n: 1,
	}}, exportTimeout)
	require.Equal(t, uint64(1), counters.snapshot()["s1/spans_errors"])
	require.Contains(t, output.String(), "partially rejected")
	require.NotContains(t, output.String(), secret)
	require.Contains(t, output.String(), "[REDACTED]")
}
