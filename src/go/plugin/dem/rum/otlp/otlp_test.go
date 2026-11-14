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
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
)

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

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

func simplify(rec *logspb.LogRecord) simpleRec {
	attrs := map[string]string{}
	for _, kv := range rec.Attributes {
		attrs[kv.Key] = kv.GetValue().GetStringValue()
	}
	return simpleRec{
		body:     rec.GetBody().GetStringValue(),
		severity: rec.SeverityText,
		attrs:    attrs,
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

func accepted(b *beacon.Beacon) aggregate.Result {
	return aggregate.Result{
		Observation:  b,
		Investigated: true,
		PageView:     true,
	}
}

func ptr(s string) *string { return &s }

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

// Investigate sampling: a beacon whose session is sampled out is
// still measured, but produces no events.
func TestIngestSkipsSampledOutBeacons(t *testing.T) {
	counters := newRecCounters()
	e := newExporter(t, "127.0.0.1:1", counters, nil)
	b := mkBeacon()
	e.Ingest(aggregate.Result{
		Observation: b,
	})
	if n := len(e.ch); n != 0 {
		t.Fatalf("queued %d records for a sampled-out beacon", n)
	}
	if got := counters.snapshot(); len(got) != 0 {
		t.Fatalf("sampling out is not a drop: counters = %#v", got)
	}
}

func TestExportUsesAcceptedMeasurementUpdatesAndIdentity(t *testing.T) {
	counters := newRecCounters()
	exporter := newExporter(t, "127.0.0.1:1", counters, nil)
	owner := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Name:       "s1",
		PageGroups: 20,
		Countries:  20,
		Investigate: aggregate.InvestigateCfg{
			Rate: 1,
		},
	})
	event := &beacon.Beacon{
		Browser:      "Chrome",
		AppVersion:   "first-release",
		Site:         "s1",
		Received:     time.Now(),
		SessionID:    "session",
		ExperienceID: "document",
		PageGroup:    "/entry",
		Path:         "/route",
		View:         "route",
		ViewID:       "view",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "metric", Revision: 1, Value: 0.1}},
	}
	exporter.Ingest(owner.Ingest(event))
	require.Len(t, exporter.ch, 2)
	page, vital := simplify(<-exporter.ch), simplify(<-exporter.ch)
	require.Equal(t, "pageview", page.attrs["rum.type"])
	require.Equal(t, "vital", vital.attrs["rum.type"])
	require.Equal(t, "document", vital.attrs["document.experience.id"])
	require.Equal(t, "view", vital.attrs["application.view.id"])
	require.Equal(t, "/entry", vital.attrs["page.group"])
	require.Equal(t, "metric", vital.attrs["metric.id"])
	require.Equal(t, "1", vital.attrs["metric.revision"])
	exporter.Ingest(owner.Ingest(event))
	require.Empty(t, exporter.ch, "filtered empty result must never fall back to original beacon")
	event.Vitals[0].Revision, event.Vitals[0].Value = 2, 0.4
	event.View, event.ViewID = "later-route", "later-view"
	event.Browser, event.AppVersion = "Firefox", "later-release"
	exporter.Ingest(owner.Ingest(event))
	require.Len(t, exporter.ch, 1, "a vital update is not a document activation")
	update := simplify(<-exporter.ch)
	require.Equal(t, "vital", update.attrs["rum.type"])
	require.Equal(t, "metric", update.attrs["metric.id"])
	require.Equal(t, "2", update.attrs["metric.revision"])
	require.Equal(t, "0.4", update.attrs["metric.value"])
	require.Equal(t, "later-route", update.attrs["page.view"], "each revision retains its application view at report time")
	require.Equal(t, "later-view", update.attrs["application.view.id"])
	require.Equal(t, "Chrome", update.attrs["browser.name"])
	require.Equal(t, "first-release", update.attrs["app.version"])
}

// Exporters build records only from the admitted observation, never from the
// browser beacon the aggregator received.
func TestExportDoesNotResurrectFilteredObservation(t *testing.T) {
	counters := newRecCounters()
	logs := newExporter(t, "127.0.0.1:1", counters, nil)
	traces := newTraceExporter(t, "127.0.0.1:1", "s1", counters, nil)
	result := aggregate.Result{
		Investigated: true,
		Observation: &beacon.Beacon{
			Site: "s1",
		},
	}
	logs.Ingest(result)
	traces.Ingest(result)
	require.Empty(t, logs.ch)
	require.Empty(t, traces.spanCh)
	require.Empty(t, counters.snapshot())
}
