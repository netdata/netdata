// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/stretchr/testify/require"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

func TestIngestDispositionAndOverflow(t *testing.T) {
	c := newRecCounters()
	l := newExporter(t, "127.0.0.1:1", c, nil)
	tr := newTraceExporter(t, "127.0.0.1:1", "s1", c, nil)
	// Measured but not investigated: nothing is exported.
	measured := aggregate.Result{
		Observation: tracedBeacon(),
		PageView:    true,
	}
	l.Ingest(measured)
	tr.Ingest(measured)
	require.Empty(t, l.ch)
	require.Empty(t, tr.spanCh)
	require.Empty(t, c.snapshot())
	// Exercise the actual bounded queues, without replacing their capacities.
	for range queueCap + 1 {
		l.Ingest(accepted(tracedBeacon()))
	}
	for range spanQueueCap + 1 {
		tr.Ingest(accepted(tracedBeacon()))
	}
	require.Len(t, l.ch, queueCap)
	require.Len(t, tr.spanCh, spanQueueCap)
	require.EqualValues(t, 1, c.snapshot()[CounterLogsDropped])
	require.EqualValues(t, 1, c.snapshot()[CounterSpansDropped])
}

func TestIngestDropsOnFullQueue(t *testing.T) {
	counters := newRecCounters()
	e := newExporter(t, "127.0.0.1:1", counters, nil) // unreachable; nothing drains e.ch
	e.ch = make(chan *logspb.LogRecord, 1)

	b1 := mkBeacon() // One pageview fills the queue.
	e.Ingest(aggregate.Result{
		Observation:  b1,
		Investigated: true,
		PageView:     true,
	})
	b2 := mkBeacon()
	b2.SessionID = "sess2"
	e.Ingest(aggregate.Result{
		Observation:  b2,
		Investigated: true,
		PageView:     true,
	})

	want := map[string]uint64{"otlp_dropped": 1}
	if got := counters.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("counters = %#v, want %#v", got, want)
	}
}

func TestSharedShutdownDrainsOrDropsUnattempted(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "drain", true: "expired"}[expired], func(t *testing.T) {
			c := newRecCounters()
			ls, ts := &fakeLogsService{}, &fakeTraceService{}
			l := newExporter(t, startServer(t, ls), c, nil)
			tr := newTraceExporter(t, startTraceServer(t, ts), "s1", c, nil)
			for range 300 {
				l.Ingest(accepted(tracedBeacon()))
				tr.Ingest(accepted(tracedBeacon()))
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			final, cancelFinal := context.WithTimeout(context.Background(), time.Second)
			defer cancelFinal()
			if expired {
				cancelFinal()
			}
			var wg sync.WaitGroup
			wg.Go(func() { l.Run(ctx, func() context.Context { return final }) })
			wg.Go(func() { tr.Run(ctx, func() context.Context { return final }) })
			wg.Wait()
			require.Empty(t, l.ch)
			require.Empty(t, tr.spanCh)
			if expired {
				require.EqualValues(t, 300, c.snapshot()[CounterLogsDropped])
				require.EqualValues(t, 300, c.snapshot()[CounterSpansDropped])
				require.Empty(t, ls.requests())
				require.Zero(t, ts.count())
			} else {
				require.EqualValues(t, 300, c.snapshot()[CounterLogsSent])
				require.EqualValues(t, 300, c.snapshot()[CounterSpansSent])
			}
		})
	}
}

// TestShutdownFlushIsBounded: with a black-holed endpoint and a queued
// record, cancelling Run must return within the shutdown flush budget, not
// the regular export timeout (the agent's SIGTERM grace is 3s).
func TestShutdownFlushIsBounded(t *testing.T) {
	e := newExporter(t, "192.0.2.1:4317", newRecCounters(), nil)
	e.Ingest(aggregate.Result{
		Observation:  mkBeacon(),
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
					Investigated: true,
					PageView:     true,
				}
				if signal == "logs" {
					remote := &fakeLogsService{}
					e := newExporter(t, startServer(t, remote), counters, nil)
					for i := 0; i < maxBatch+3; i++ {
						e.Ingest(result)
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
					sent, dropped, errors = CounterLogsSent, CounterLogsDropped, CounterLogsErrors
				} else {
					remote := &fakeTraceService{}
					e := newTraceExporter(t, startTraceServer(t, remote), "s1", counters, nil)
					b := tracedBeacon()
					b.Spans = append(b.Spans, b.Spans[0])
					result.Observation = b
					for i := 0; i < maxSpanBatch+3; i++ {
						e.Ingest(result)
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
					sent, dropped, errors = CounterSpansSent, CounterSpansDropped, CounterSpansErrors
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
				Investigated: true,
				PageView:     true,
			}
			if signal == "logs" {
				remote := &blockedLogsService{
					started:   started,
					cancelled: cancelled,
				}
				e := newExporter(t, startServer(t, remote), counters, nil)
				e.Ingest(result)
				run = e.Run
				requests = func() int { return len(remote.requests()) }
				sent, dropped, errors = CounterLogsSent, CounterLogsDropped, CounterLogsErrors
			} else {
				remote := &blockedTraceService{
					started:   started,
					cancelled: cancelled,
				}
				e := newTraceExporter(t, startTraceServer(t, remote), "s1", counters, nil)
				e.Ingest(result)
				run = e.Run
				requests = remote.count
				sent, dropped, errors = CounterSpansSent, CounterSpansDropped, CounterSpansErrors
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
