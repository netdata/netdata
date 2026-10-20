// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/stretchr/testify/require"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Hold the first export preflight while the actual run context is cancelled.
type preflightContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (c *preflightContext) Err() error {
	c.once.Do(func() { close(c.entered); <-c.release })
	return c.Context.Err()
}

func waitShutdownSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for exporter handshake")
	}
}

func TestCancellationBeforeExportPreservesFinalBatch(t *testing.T) {
	for _, signal := range []string{"logs", "traces"} {
		for _, expired := range []bool{false, true} {
			name := signal + "/live final context"
			if expired {
				name = signal + "/expired final context"
			}
			t.Run(name, func(t *testing.T) {
				counters := newRecCounters()
				var run func(context.Context, func() context.Context)
				var received func() int
				var want uint64
				var sent, dropped, errors string
				result := aggregate.Result{
					Observation:  tracedBeacon(),
					Accepted:     true,
					Investigated: true,
					PageView:     true,
				}
				if signal == "logs" {
					remote := &fakeLogsService{}
					e := newExporter(t, startServer(t, remote), counters, nil)
					for i := 0; i < maxBatch+3; i++ {
						e.Ingest(mkBeacon(), result)
					}
					run = e.Run
					want = maxBatch + 3
					received = func() int {
						n := 0
						for _, req := range remote.requests() {
							for _, rs := range req.ResourceLogs {
								for _, ss := range rs.ScopeLogs {
									n += len(ss.LogRecords)
								}
							}
						}
						return n
					}
					sent, dropped, errors = aggregate.CounterOTLPSent, aggregate.CounterOTLPDropped, aggregate.CounterOTLPErrors
				} else {
					remote := &fakeTraceService{}
					e := newTraceExporter(t, startTraceServer(t, remote), "s1", counters, nil)
					b := tracedBeacon()
					b.Spans = append(b.Spans, b.Spans[0])
					result.Observation = b
					for i := 0; i < maxSpanBatch+3; i++ {
						e.Ingest(b, result)
					}
					run = e.Run
					want = 2 * (maxSpanBatch + 3)
					received = func() int {
						remote.mu.Lock()
						defer remote.mu.Unlock()
						n := 0
						for _, req := range remote.reqs {
							for _, rs := range req.ResourceSpans {
								for _, ss := range rs.ScopeSpans {
									n += len(ss.Spans)
								}
							}
						}
						return n
					}
					sent, dropped, errors = aggregate.CounterSpansSent, aggregate.CounterSpansDropped, aggregate.CounterSpansErrors
				}
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx := &preflightContext{
					Context: base,
					entered: make(chan struct{}),
					release: make(chan struct{}),
				}
				var release sync.Once
				defer release.Do(func() { close(ctx.release) })
				finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer finalCancel()
				if expired {
					finalCancel()
				}
				calls := 0
				done := make(chan struct{})
				go func() {
					run(ctx, func() context.Context { calls++; return finalCtx })
					close(done)
				}()
				waitShutdownSignal(t, ctx.entered)
				cancel()
				release.Do(func() { close(ctx.release) })
				waitShutdownSignal(t, done)
				require.Equal(t, 1, calls)
				got := counters.snapshot()
				require.Zero(t, got[errors])
				if expired {
					require.Zero(t, received())
					require.Zero(t, got[sent])
					require.Equal(t, want, got[dropped])
				} else {
					require.Equal(t, int(want), received())
					require.Equal(t, want, got[sent])
					require.Zero(t, got[dropped])
				}
			})
		}
	}
}

// These handlers acknowledge the actual RPC before waiting for cancellation.
type blockedLogsService struct {
	fakeLogsService
	started   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (s *blockedLogsService) Export(
	ctx context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	s.fakeLogsService.Export(ctx, req)
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	select {
	case s.cancelled <- struct{}{}:
	default:
	}
	return nil, ctx.Err()
}

type blockedTraceService struct {
	fakeTraceService
	started   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (s *blockedTraceService) Export(
	ctx context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	s.fakeTraceService.Export(ctx, req)
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	select {
	case s.cancelled <- struct{}{}:
	default:
	}
	return nil, ctx.Err()
}

func TestInFlightPeriodicExportAbortsOnCancel(t *testing.T) {
	for _, signal := range []string{"logs", "traces"} {
		t.Run(signal, func(t *testing.T) {
			counters := newRecCounters()
			started, cancelled := make(chan struct{}), make(chan struct{}, 1)
			var run func(context.Context, func() context.Context)
			var requests func() int
			var sent, dropped, errors string
			result := aggregate.Result{
				Observation:  tracedBeacon(),
				Accepted:     true,
				Investigated: true,
				PageView:     true,
			}
			if signal == "logs" {
				remote := &blockedLogsService{
					started:   started,
					cancelled: cancelled,
				}
				e := newExporter(t, startServer(t, remote), counters, nil)
				e.Ingest(mkBeacon(), result)
				run = e.Run
				requests = func() int { return len(remote.requests()) }
				sent, dropped, errors = aggregate.CounterOTLPSent, aggregate.CounterOTLPDropped, aggregate.CounterOTLPErrors
			} else {
				remote := &blockedTraceService{
					started:   started,
					cancelled: cancelled,
				}
				e := newTraceExporter(t, startTraceServer(t, remote), "s1", counters, nil)
				e.Ingest(tracedBeacon(), result)
				run = e.Run
				requests = remote.count
				sent, dropped, errors = aggregate.CounterSpansSent, aggregate.CounterSpansDropped, aggregate.CounterSpansErrors
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { run(ctx, finalContext(t)); close(done) }()
			waitShutdownSignal(t, started)
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("in-flight export did not abort within shutdown budget")
			}
			waitShutdownSignal(t, cancelled)
			require.Equal(t, 1, requests(), "an attempted export must not be retried")
			got := counters.snapshot()
			require.Equal(t, uint64(1), got[errors])
			require.Zero(t, got[sent])
			require.Zero(t, got[dropped])
		})
	}
}
