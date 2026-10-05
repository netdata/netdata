// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
)

type processingLogs struct {
	collogspb.UnimplementedLogsServiceServer
	mu    sync.Mutex
	kinds []string
}

func (s *processingLogs) Export(
	_ context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range req.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			for _, record := range scope.LogRecords {
				for _, attr := range record.Attributes {
					if attr.Key == "rum.type" {
						s.kinds = append(s.kinds, attr.Value.GetStringValue())
					}
				}
			}
		}
	}
	return &collogspb.ExportLogsServiceResponse{}, nil
}

func (s *processingLogs) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.kinds...)
}

type processingHistory struct{ events []aggregate.HistoryEvent }

func (h *processingHistory) Event(event aggregate.HistoryEvent) { h.events = append(h.events, event) }

func newProcessingTest(
	t *testing.T,
	investigate aggregate.InvestigateCfg,
) (*processor, *processingHistory, func() []string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	logs := &processingLogs{}
	collogspb.RegisterLogsServiceServer(server, logs)
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); <-done })
	a := aggregate.New(
		5*time.Minute,
		aggregate.SiteCfg{
			Name:        "site",
			PageGroups:  20,
			Countries:   20,
			Investigate: investigate,
		},
	)
	h := &processingHistory{}
	a.SetHistorySink(h)
	exporter, err := otlp.New(
		context.Background(),
		otlp.Config{
			Enabled:  "yes",
			Endpoint: listener.Addr().String(),
		},
		"site",
		"",
		a,
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, exporter.Close()) })
	// Exercise the real export queue and its retirement drain without waiting for
	// a batch timer. All producer calls finish before worker cancellation.
	drain := func() []string {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		exporter.Run(ctx)
		return logs.snapshot()
	}
	return &processor{
		aggregator: a,
		exporter:   exporter,
	}, h, drain
}

func TestProcessingDeduplicatesWithoutMutatingInput(t *testing.T) {
	p, history, drain := newProcessingTest(t, aggregate.InvestigateCfg{})
	event := &beacon.Beacon{
		Site:      "site",
		Received:  time.Now(),
		SessionID: "session",
		PageID:    "page",
		Path:      "/checkout",
		PageGroup: "/checkout",
	}
	before := *event
	p.Ingest(event)
	p.Ingest(event)
	assert.Equal(t, before, *event)
	snapshot := p.aggregator.Snapshot()
	assert.EqualValues(t, 2, snapshot.Counters[aggregate.CounterAccepted])
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterPageviews])
	require.Len(t, history.events, 2)
	assert.Equal(t, "pageview", history.events[0].Type)
	assert.Equal(t, "activity", history.events[1].Type)
	assert.Equal(t, []string{"session_start", "pageview"}, drain())
}

func TestProcessingSamplingPromotionKeepsHistoryAndExportsCurrentObservation(t *testing.T) {
	const session = "late-error"
	const rate = 0.000001
	require.False(t, beacon.SessionSampled(session, rate))
	p, history, drain := newProcessingTest(t, aggregate.InvestigateCfg{
		Rate:       rate,
		KeepErrors: true,
	})
	now := time.Now()
	first := &beacon.Beacon{
		Site:      "site",
		Received:  now,
		SessionID: session,
		Path:      "/checkout",
		PageGroup: "/checkout",
		Events:    []beacon.Event{{Name: "earlier-action", Time: now}},
	}
	p.Ingest(first)
	assert.Empty(t, history.events)
	rows, _ := p.aggregator.Live(0, 10)
	require.Len(t, rows, 1) // Live measurements do not follow investigation sampling.
	assert.True(t, rows[0].PageView)
	assert.EqualValues(t, 1, p.aggregator.Snapshot().Counters[aggregate.CounterPageviews])
	promoted := &beacon.Beacon{
		Site:      "site",
		Received:  now.Add(time.Second),
		SessionID: session,
		Path:      "/checkout",
		PageGroup: "/checkout",
		Errors: []beacon.Error{
			{Fingerprint: "failure", Type: "Error", Message: "failed", Time: now.Add(time.Second)},
		},
	}
	p.Ingest(promoted)
	require.Len(t, history.events, 3)
	assert.Equal(
		t,
		[]string{"pageview", "event", "error"},
		[]string{history.events[0].Type, history.events[1].Type, history.events[2].Type},
	)
	assert.Equal(t, now.UnixMicro(), history.events[0].TSUnixUS)
	assert.Equal(t, now.UnixMicro(), history.events[1].TSUnixUS)
	assert.Equal(t, "failure", history.events[2].Fingerprint)
	// Prior observations replay into history only; export sees neither the old
	// pageview nor custom event. Its first investigated observation starts a session.
	assert.Equal(t, []string{"session_start", "error"}, drain())
	assert.EqualValues(t, 1, p.aggregator.Snapshot().Counters[aggregate.CounterJSErrors])
}

func TestProcessingUninvestigatedAndRejectedObservationsDoNotExport(t *testing.T) {
	const rate = 0.000001
	require.False(t, beacon.SessionSampled("quiet", rate))
	p, history, drain := newProcessingTest(t, aggregate.InvestigateCfg{
		Rate: rate,
	})
	p.Ingest(
		&beacon.Beacon{
			Site:      "site",
			Received:  time.Now(),
			SessionID: "quiet",
			Path:      "/checkout",
			Errors:    []beacon.Error{{Fingerprint: "failure", Message: "failed"}},
		},
	)
	p.Ingest(&beacon.Beacon{
		Site:      "other",
		SessionID: "wrong",
		Errors:    []beacon.Error{{Message: "wrong site"}},
	})
	p.Reject("site", beacon.RejectOrigin)
	p.Reject("other", beacon.RejectRate)
	assert.Empty(t, drain())
	assert.Empty(t, history.events)
	snapshot := p.aggregator.Snapshot()
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterAccepted])
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterJSErrors])
	assert.EqualValues(t, 1, snapshot.Counters[beacon.RejectOrigin])
	assert.Zero(t, snapshot.Counters[beacon.RejectRate])
	assert.Zero(t, snapshot.Counters[aggregate.CounterOTLPDropped])
	assert.Zero(t, snapshot.Counters[aggregate.CounterOTLPSent])
}
