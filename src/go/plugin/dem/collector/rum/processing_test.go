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
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
)

type processingLogs struct {
	collogspb.UnimplementedLogsServiceServer
	mu    sync.Mutex
	kinds []string
	guids []string
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
					if attr.Key == "dem.vnode_guid" {
						s.guids = append(s.guids, attr.Value.GetStringValue())
					}
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
) (*processor, *processingHistory, func() []string, *processingLogs, *transportCounters) {
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
	transport := newTransportCounters()
	exporter, err := otlp.NewLogs(
		context.Background(),
		config.Destination{
			Endpoint: new("http://" + listener.Addr().String()),
		},
		"site",
		transport,
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, exporter.Close()) })
	// Exercise the real export queue and its retirement drain without waiting for
	// a batch timer. All producer calls finish before worker cancellation.
	drain := func() []string {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		finalCtx, finish := context.WithTimeout(context.Background(), time.Second)
		defer finish()
		exporter.Run(ctx, func() context.Context { return finalCtx })
		return logs.snapshot()
	}
	return &processor{
		aggregator: a,
		logs:       exporter,
	}, h, drain, logs, transport
}

func TestProcessingDeduplicatesWithoutMutatingInput(t *testing.T) {
	p, history, drain, _, _ := newProcessingTest(t, aggregate.InvestigateCfg{
		Rate: 1,
	})
	event := &beacon.Beacon{
		Site:         "site",
		Received:     time.Now(),
		SessionID:    "session",
		ExperienceID: "page",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "page", Revision: 1}},
		Path:         "/checkout",
		PageGroup:    "/checkout",
	}
	before := *event
	p.Ingest(event)
	p.Ingest(event)
	assert.Equal(t, before, *event)
	snapshot := p.aggregator.Snapshot()
	assert.EqualValues(t, 2, snapshot.Counters[aggregate.CounterAccepted])
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterPageviews])
	require.Len(t, history.events, 1, "a replay cannot create another retained activity record")
	assert.Equal(t, "pageview", history.events[0].Type)
	assert.Equal(t, []string{"pageview"}, drain())
}

func TestProcessingSamplingPromotionKeepsHistoryAndExportsCurrentObservation(t *testing.T) {
	const session = "late-error"
	const rate = 0.000001
	require.False(t, aggregate.SessionSampled(session, rate))
	p, history, drain, _, _ := newProcessingTest(t, aggregate.InvestigateCfg{
		Rate:       rate,
		KeepErrors: true,
	})
	now := time.Now()
	browserTime := now.Add(-5 * time.Minute)
	first := &beacon.Beacon{
		Site:         "site",
		Received:     now,
		SessionID:    session,
		ExperienceID: "document",
		Path:         "/checkout",
		PageGroup:    "/checkout",
		Events: []beacon.Event{
			{Kind: beacon.EventDocument, ID: "document", Revision: 1, Time: browserTime},
			{Name: "earlier-action", Time: browserTime},
		},
	}
	p.Ingest(first)
	assert.Empty(t, history.events)
	rows, _ := p.aggregator.Live(0, 10, "")
	require.Len(t, rows, 1) // Live measurements do not follow investigation sampling.
	assert.True(t, rows[0].PageView)
	assert.EqualValues(t, 1, p.aggregator.Snapshot().Counters[aggregate.CounterPageviews])
	promoted := &beacon.Beacon{
		Site:         "site",
		Received:     now.Add(time.Second),
		SessionID:    session,
		ExperienceID: "document",
		Path:         "/checkout",
		PageGroup:    "/checkout",
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
	assert.Equal(t, now.UnixMicro(), history.events[0].ObservedUS)
	assert.Equal(t, now.UnixMicro(), history.events[1].ObservedUS)
	assert.Equal(t, "failure", history.events[2].Fingerprint)
	// Prior observations replay into history only; export sees neither the old
	// pageview nor custom event. It does not invent a session start.
	assert.Equal(t, []string{"error"}, drain())
	assert.EqualValues(t, 1, p.aggregator.Snapshot().Counters[aggregate.CounterJSErrors])
}

func TestProcessingUninvestigatedAndRejectedObservationsDoNotExport(t *testing.T) {
	const rate = 0.000001
	require.False(t, aggregate.SessionSampled("quiet", rate))
	p, history, drain, _, transport := newProcessingTest(t, aggregate.InvestigateCfg{
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
	p.Reject(beacon.RejectOrigin)
	assert.Empty(t, drain())
	assert.Empty(t, history.events)
	snapshot := p.aggregator.Snapshot()
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterAccepted])
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterJSErrors])
	assert.EqualValues(t, 1, snapshot.Counters[beacon.RejectOrigin])
	assert.Zero(t, transport.get(otlp.CounterLogsDropped))
	assert.Zero(t, transport.get(otlp.CounterLogsSent))
}

func TestProcessingQueuedExportsKeepGUIDAcrossReassignment(t *testing.T) {
	const guidA = "11111111-1111-4111-8111-111111111111"
	const guidB = "22222222-2222-4222-8222-222222222222"
	p, history, drain, exported, transport := newProcessingTest(t, aggregate.InvestigateCfg{
		Rate: 1,
	})
	admit := func(guid, path string) {
		p.aggregator.SetVnodeGUID(guid)
		p.Ingest(
			&beacon.Beacon{
				Site:         "site",
				SessionID:    "session",
				ExperienceID: path,
				PageGroup:    path,
				Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: path, Revision: 1}},
			},
		)
	}
	admit(guidA, "/old")
	admit(guidB, "/new")
	require.Len(t, history.events, 2)
	assert.Equal(t, guidA, history.events[0].VnodeGUID)
	assert.Equal(t, guidB, history.events[1].VnodeGUID)
	assert.Equal(t, []string{"pageview", "pageview"}, drain())
	exported.mu.Lock()
	guids := append([]string(nil), exported.guids...)
	exported.mu.Unlock()
	assert.Equal(t, []string{guidA, guidB}, guids, "flush reads captured records rather than current configuration")
	assert.EqualValues(t, 2, transport.get(otlp.CounterLogsSent), "export diagnostics remain job-wide")
	assert.EqualValues(
		t,
		1,
		p.aggregator.Snapshot().Counters[aggregate.CounterPageviews],
		"traffic belongs to the new measurement window",
	)
}
