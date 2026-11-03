// SPDX-License-Identifier: GPL-3.0-or-later

package rum_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type contractLogs struct {
	collogs.UnimplementedLogsServiceServer
	mu     sync.Mutex
	count  int
	reject bool
}

func (s *contractLogs) Export(
	_ context.Context,
	r *collogs.ExportLogsServiceRequest,
) (*collogs.ExportLogsServiceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range r.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			s.count += len(scope.LogRecords)
		}
	}
	if s.reject {
		return nil, status.Error(codes.Unavailable, "synthetic receiver unavailable")
	}
	return &collogs.ExportLogsServiceResponse{}, nil
}

type contractTraces struct {
	coltraces.UnimplementedTraceServiceServer
	mu    sync.Mutex
	count int
}

func (s *contractTraces) Export(
	_ context.Context,
	r *coltraces.ExportTraceServiceRequest,
) (*coltraces.ExportTraceServiceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range r.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			s.count += len(scope.Spans)
		}
	}
	return &coltraces.ExportTraceServiceResponse{}, nil
}
func contractReceiver(t *testing.T, reject bool) (string, *contractLogs, *contractTraces) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	logs, traces := &contractLogs{
		reject: reject,
	}, &contractTraces{}
	srv := grpc.NewServer()
	collogs.RegisterLogsServiceServer(srv, logs)
	coltraces.RegisterTraceServiceServer(srv, traces)
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Stop(); <-done })
	return "http://" + ln.Addr().String(), logs, traces
}
func contractSite(t *testing.T) (*rum.Collector, *rumregistry.Registry, *history.Store) {
	t.Helper()
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	hub, store := rumregistry.New(), history.NewStore(db)
	site := rum.New(rum.Dependencies{
		Registry: hub,
		History:  store,
	})
	site.Name, site.AllowedOrigins = "shop", []string{"https://example.org"}
	return site, hub, store
}
func sendContractBeacon(t *testing.T, hub *rumregistry.Registry, body []byte) {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodPost,
		"http://"+hub.Availability().Listen+"/rum/shop/collect",
		strings.NewReader(string(body)),
	)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://example.org")
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/124.0.0.0 Safari/537.36")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusAccepted, res.StatusCode)
}
func TestOptionalExportsThroughNativeJobs(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		logs, traces, reject bool
	}{
		{name: "core only"}, {name: "logs only", logs: true}, {name: "traces only", traces: true}, {name: "both", logs: true, traces: true}, {name: "log rejection preserves traces and history", logs: true, traces: true, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logEndpoint, logs, _ := contractReceiver(t, tc.reject)
			traceEndpoint, _, traces := contractReceiver(t, false)
			site, hub, store := contractSite(t)
			site.EventLogs.Enabled, site.EventLogs.IncludeConsoleLogs = tc.logs, true
			site.EventLogs.Destination.Endpoint = new(logEndpoint)
			site.Tracing.Enabled, site.Tracing.Destination.Endpoint = tc.traces, new(traceEndpoint)
			recv := receiver.New(receiver.Dependencies{
				Registry: hub,
			})
			recv.Listen = "127.0.0.1:0"
			startJob(t, "receiver", "receiver", recv)
			job, out, stop := startJob(t, "rum", "shop", site)
			tickUntil(t, job, out, "SET 'available' = 1")
			assert.Equal(t, tc.logs, strings.Contains(out.String(), "rum.otlp"), "chart enablement before any traffic")
			assert.Equal(
				t,
				tc.traces,
				strings.Contains(out.String(), "rum.spans"),
				"chart enablement before any traffic",
			)
			raw, err := os.ReadFile("../../rum/faro/testdata/tracing.json")
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(raw, &body))
			meta := body["meta"].(map[string]any)
			meta["session"] = map[string]any{"id": "export-contract"}
			meta["page"] = map[string]any{"id": "checkout-document", "url": "https://example.org/checkout"}
			body["events"] = append(
				body["events"].([]any),
				map[string]any{
					"name":       "document_activated",
					"attributes": map[string]string{"observation_id": "checkout-document", "observation_sequence": "1"},
				},
			)
			body["logs"] = []map[string]any{
				{"message": "payment slow", "level": "warn", "timestamp": time.Now().UTC().Format(time.RFC3339Nano)},
			}
			raw, err = json.Marshal(body)
			require.NoError(t, err)
			sendContractBeacon(t, hub, raw)
			data, _, release, ok := hub.AcquireSite("shop")
			require.True(t, ok)
			a := data.Aggregator
			release()
			stop()
			logs.mu.Lock()
			logCount := logs.count
			logs.mu.Unlock()
			traces.mu.Lock()
			traceCount := traces.count
			traces.mu.Unlock()
			assert.Equal(t, tc.logs, logCount > 0)
			assert.Equal(t, tc.traces, traceCount > 0)
			counters := a.Snapshot().Counters
			assert.Zero(t, counters[aggregate.CounterInvalidMeasurements])
			if !tc.logs {
				assert.Zero(t, counters[aggregate.CounterOTLPSent])
				assert.Zero(t, counters[aggregate.CounterOTLPDropped])
				assert.Zero(t, counters[aggregate.CounterOTLPErrors])
			}
			if !tc.traces {
				assert.Zero(t, counters[aggregate.CounterSpansSent])
				assert.Zero(t, counters[aggregate.CounterSpansDropped])
				assert.Zero(t, counters[aggregate.CounterSpansErrors])
			} else {
				assert.EqualValues(t, 2, counters[aggregate.CounterSpansSent])
			}
			if tc.reject {
				assert.Positive(t, counters[aggregate.CounterOTLPErrors])
				assert.Zero(t, counters[aggregate.CounterOTLPSent])
			}
			rows, err := store.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+10, 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.EqualValues(t, 1, rows[0].Pageviews)
		})
	}
}

// Opt-in smoke sends through the native collector to an actual Netdata receiver.
// Query the receiver's stored netdata-rum stream for session dem-otlp-storage-smoke
// after this test; unit fakes cannot establish storage/query preservation.
func TestOTLPReceiverStorageSmoke(t *testing.T) {
	endpoint := os.Getenv("NETDATA_DEM_OTLP_SMOKE_ENDPOINT")
	if endpoint == "" {
		t.Skip("set NETDATA_DEM_OTLP_SMOKE_ENDPOINT to an isolated Netdata OTLP receiver")
	}
	site, hub, store := contractSite(t)
	site.EventLogs.Enabled, site.EventLogs.IncludeConsoleLogs = true, true
	site.EventLogs.Destination.Endpoint = new(endpoint)
	recv := receiver.New(receiver.Dependencies{
		Registry: hub,
	})
	recv.Listen = "127.0.0.1:0"
	startJob(t, "receiver", "receiver", recv)
	_, _, stop := startJob(t, "rum", "shop", site)
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	raw := fmt.Sprintf(
		`{"meta":{"page":{"id":"smoke-document","url":"https://example.org/checkout"},"session":{"id":"dem-otlp-storage-smoke"},"browser":{"name":"Chrome"}},"events":[{"name":"document_activated","attributes":{"observation_id":"smoke-document","observation_sequence":"1"}},{"name":"checkout_attempt","timestamp":%q,"attributes":{"cart_total":"42"}}],"logs":[{"message":"payment slow","level":"warn","timestamp":%q},{"message":"payment failed","level":"error","timestamp":%q,"context":{"type":"TypeError","stackFrames":"{\"filename\":\"https://example.org/app.js\",\"function\":\"checkout\",\"lineno\":12,\"colno\":3}"}}]}`,
		timestamp,
		timestamp,
		timestamp,
	)
	sendContractBeacon(t, hub, []byte(raw))
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	a := data.Aggregator
	release()
	stop()
	counters := a.Snapshot().Counters
	assert.Zero(t, counters[aggregate.CounterInvalidMeasurements])
	require.EqualValues(t, 4, counters[aggregate.CounterOTLPSent])
	require.Zero(t, counters[aggregate.CounterOTLPErrors])
	require.Zero(t, counters[aggregate.CounterJSErrors])
	rows, err := store.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+10, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestEventExportRecoveryKeepsNativeHistory(t *testing.T) {
	endpoint, logs, _ := contractReceiver(t, true)
	site, hub, store := contractSite(t)
	site.EventLogs.Enabled = true
	site.EventLogs.Destination.Endpoint = new(endpoint)
	recv := receiver.New(receiver.Dependencies{
		Registry: hub,
	})
	recv.Listen = "127.0.0.1:0"
	startJob(t, "receiver", "receiver", recv)
	_, _, stop := startJob(t, "rum", "shop", site)
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	a := data.Aggregator
	release()
	send := func(page string) {
		sendContractBeacon(
			t,
			hub,
			[]byte(
				fmt.Sprintf(
					`{"meta":{"page":{"id":"%[1]s","url":"https://example.org/%[1]s"},"session":{"id":"recovering-session"}},"events":[{"name":"document_activated","attributes":{"observation_id":"%[1]s","observation_sequence":"1"}}]}`,
					page,
				),
			),
		)
	}
	send("first")
	require.Eventually(
		t,
		func() bool { return a.Snapshot().Counters[aggregate.CounterOTLPErrors] > 0 },
		4*time.Second,
		10*time.Millisecond,
	)
	logs.mu.Lock()
	logs.reject = false
	logs.mu.Unlock()
	send("second")
	require.Eventually(
		t,
		func() bool { return a.Snapshot().Counters[aggregate.CounterOTLPSent] > 0 },
		4*time.Second,
		10*time.Millisecond,
	)
	stop()
	counters := a.Snapshot().Counters
	assert.Zero(t, counters[aggregate.CounterInvalidMeasurements])
	assert.EqualValues(t, 1, counters[aggregate.CounterOTLPErrors])
	assert.EqualValues(t, 1, counters[aggregate.CounterOTLPSent])
	assert.Zero(t, counters[aggregate.CounterOTLPDropped])
	rows, err := store.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+10, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0].Pageviews)
}
