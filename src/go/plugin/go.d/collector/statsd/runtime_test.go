// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const waitFor = 5 * time.Second

// freeAddress returns a loopback address currently free for both UDP and TCP.
func freeAddress(t testing.TB) string {
	t.Helper()
	for range 20 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		pc, err := net.ListenPacket("udp", addr)
		_ = ln.Close()
		if err == nil {
			_ = pc.Close()
			return addr
		}
	}
	t.Fatal("no loopback port free for both UDP and TCP")
	return ""
}

// requireFree proves an address is not held by binding and releasing it.
func requireFree(t testing.TB, protocol, addr string) {
	t.Helper()
	var closer io.Closer
	var err error
	if protocol == protocolUDP {
		closer, err = net.ListenPacket(protocolUDP, addr)
	} else {
		closer, err = net.Listen(protocolTCP, addr)
	}
	require.NoError(t, err, "%s %s must be free", protocol, addr)
	require.NoError(t, closer.Close())
}

type runtimeFixture struct {
	*coreFixture
	addr   string
	cancel context.CancelFunc
	done   chan error
}

// startRuntime prepares a collector on one UDP and one TCP loopback listener
// and runs it until readiness. configure may adjust configuration before Init.
func startRuntime(t testing.TB, configure func(*Collector)) *runtimeFixture {
	t.Helper()
	c := New()
	addr := freeAddress(t)
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	if configure != nil {
		configure(c)
	}
	f := &runtimeFixture{
		coreFixture: prepareFixture(t, c),
		addr:        addr,
		done:        make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	ready := make(chan struct{})
	go func() { f.done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-f.done:
		t.Fatalf("Run returned before readiness: %v", err)
	case <-time.After(waitFor):
		t.Fatal("Run did not become ready")
	}
	t.Cleanup(func() { f.stop(t) })
	return f
}

// stop cancels Run and waits for it to join every reader.
func (f *runtimeFixture) stop(t testing.TB) error {
	t.Helper()
	f.cancel()
	select {
	case err := <-f.done:
		f.done <- err // Keep the result for idempotent stop calls.
		return err
	case <-time.After(waitFor):
		t.Fatal("Run did not return after cancellation")
		return nil
	}
}

func (f *runtimeFixture) sendUDP(t testing.TB, payload string) {
	t.Helper()
	conn, err := net.Dial(protocolUDP, f.addr)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte(payload))
	require.NoError(t, err)
}

func (f *runtimeFixture) dialTCP(t testing.TB) net.Conn {
	t.Helper()
	conn, err := net.Dial(protocolTCP, f.addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

type receiverCounts struct {
	accepted uint64
	rejected map[rejection]uint64
}

func (f *coreFixture) counts() receiverCounts {
	r := f.c.receiver
	r.mu.Lock()
	defer r.mu.Unlock()
	out := receiverCounts{
		accepted: r.accepted,
		rejected: make(map[rejection]uint64),
	}
	for i, n := range r.rejects {
		if n != 0 {
			out.rejected[rejectReasons[i]] = n
		}
	}
	return out
}

// waitCounts waits until receiver totals reach exactly the wanted state.
func (f *coreFixture) waitCounts(t testing.TB, want receiverCounts) {
	t.Helper()
	if want.rejected == nil {
		want.rejected = map[rejection]uint64{}
	}
	require.Eventually(t, func() bool {
		got := f.counts()
		return got.accepted == want.accepted && fmt.Sprint(got.rejected) == fmt.Sprint(want.rejected)
	}, waitFor, time.Millisecond)
	// Settle: no further records arrive afterwards.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, want, f.counts())
}

func TestRunAcquiresOnlyAfterPreparation(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	f := prepareFixture(t, c)
	// Init and Check validated and prepared everything without binding.
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- c.Run(ctx, func() {
			// Readiness follows acquisition of every required listener, before traffic.
			for _, protocol := range []string{protocolUDP, protocolTCP} {
				var err error
				if protocol == protocolUDP {
					_, err = net.ListenPacket(protocol, addr)
				} else {
					_, err = net.Listen(protocol, addr)
				}
				assert.Error(t, err, "%s must be bound at readiness", protocol)
			}
			close(ready)
		})
	}()
	select {
	case <-ready:
	case <-time.After(waitFor):
		t.Fatal("not ready")
	}
	f.cycle.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	f.cycle.AbortCycle()

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "requested stop is a normal return")
	case <-time.After(waitFor):
		t.Fatal("Run did not return")
	}
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
}

func TestRunStartupFailureReleasesAcquiredListeners(t *testing.T) {
	free := freeAddress(t)
	occupied, err := net.Listen(protocolTCP, "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()

	c := New()
	c.Listeners = []ListenerConfig{
		{Protocol: protocolUDP, Address: free},
		{Protocol: protocolTCP, Address: free},
		{Protocol: protocolTCP, Address: occupied.Addr().String()},
	}
	c.profileDirs = nil
	prepareFixture(t, c)
	readyCalled := false
	err = c.Run(context.Background(), func() { readyCalled = true })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listeners[2]")
	assert.False(t, readyCalled)
	requireFree(t, protocolUDP, free)
	requireFree(t, protocolTCP, free)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
}

func TestConfigurationTestDuringIncumbentReception(t *testing.T) {
	incumbent := startRuntime(t, nil)
	incumbent.sendUDP(t, "before:1|c")
	incumbent.waitCounts(t, receiverCounts{
		accepted: 1,
	})

	// DynCfg test constructs, configures, runs Init and Check, then Cleanup; never Run.
	probe := New()
	probe.Config = incumbent.c.Config
	probe.profileDirs = nil
	require.NoError(t, probe.Init(context.Background()))
	require.NoError(t, probe.Check(context.Background()))
	probe.Cleanup(context.Background())

	incumbent.sendUDP(t, "after:1|c")
	incumbent.waitCounts(t, receiverCounts{
		accepted: 2,
	})
	incumbent.collect(t, false, false)
	value(t, incumbent.c, "c.total.before", 1, nil)
	value(t, incumbent.c, "c.total.after", 1, nil)
}

func TestUDPFraming(t *testing.T) {
	f := startRuntime(t, func(c *Collector) { c.maxRecord = 64 })
	// LF, CRLF, and the datagram boundary each end a record; empty lines are not records.
	f.sendUDP(t, "a:1|c\nb:2|c\r\n\nc:3|c")
	f.waitCounts(t, receiverCounts{
		accepted: 3,
	})
	// Malformed neighbors reject independently. A bare CR is record content, not a terminator.
	f.sendUDP(t, "bad record\nd:4|c\ne:5|c\r")
	f.waitCounts(t, receiverCounts{
		accepted: 4,
		rejected: map[rejection]uint64{rejectSyntax: 2},
	})
	// A datagram beyond the record bound was truncated by the read: no prefix is admitted.
	f.sendUDP(t, strings.Repeat("f:1|c\n", 12))
	f.waitCounts(t, receiverCounts{
		accepted: 4,
		rejected: map[rejection]uint64{rejectSyntax: 2, rejectOversize: 1},
	})
	f.sendUDP(t, "g:1|c")
	f.waitCounts(t, receiverCounts{
		accepted: 5,
		rejected: map[rejection]uint64{rejectSyntax: 2, rejectOversize: 1},
	})
	f.collect(t, false, false)
	for name, want := range map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "g": 1} {
		value(t, f.c, "c.total."+name, want, nil)
	}
	_, ok := f.c.store.Read().Value("c.total.f", nil)
	assert.False(t, ok)
}

func TestTCPFraming(t *testing.T) {
	const bound = 32
	f := startRuntime(t, func(c *Collector) { c.maxRecord = bound })
	exact := strings.Repeat("x", bound-len(":1|c")) + ":1|c"
	require.Len(t, exact, bound)
	over := strings.Repeat("y", bound+1-len(":1|c")) + ":1|c"

	conn := f.dialTCP(t)
	for _, chunk := range []string{
		"a:1|c\nb:2", "|c\r\n", // A record split across reads.
		"bad record\nc:3|c\n",       // Malformed neighbor.
		exact + "\r\n", over + "\n", // Exact bound accepted; one byte over rejected.
		strings.Repeat("z", 3*bound), "z\nd:4|c\n", // Oversize discarded through its newline.
		"e:5|c", // Unterminated at EOF.
	} {
		_, err := conn.Write([]byte(chunk))
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, conn.(*net.TCPConn).CloseWrite())
	f.waitCounts(t, receiverCounts{
		accepted: 5,
		rejected: map[rejection]uint64{
			rejectSyntax:       1,
			rejectOversize:     2,
			rejectUnterminated: 1,
		},
	})
	f.collect(t, false, false)
	for name, want := range map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4} {
		value(t, f.c, "c.total."+name, want, nil)
	}
	value(t, f.c, "c.total."+exact[:len(exact)-len(":1|c")], 1, nil)
	_, ok := f.c.store.Read().Value("c.total.e", nil)
	assert.False(t, ok, "a fragment without newline at EOF is not a record")
}

func TestTCPConnectionCapKeepsQuietClients(t *testing.T) {
	f := startRuntime(t, func(c *Collector) { c.MaxTCPConnections = 2 })
	first, second := f.dialTCP(t), f.dialTCP(t)
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpConnections.Load() == 2 }, waitFor, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // Quiet clients are not timed out.

	refused := f.dialTCP(t)
	require.NoError(t, refused.SetReadDeadline(time.Now().Add(waitFor)))
	_, err := refused.Read(make([]byte, 1))
	require.Error(t, err, "a client beyond the cap is closed")
	f.waitCounts(t, receiverCounts{
		rejected: map[rejection]uint64{rejectConnectionLimit: 1},
	})

	for i, conn := range []net.Conn{first, second} {
		_, err := fmt.Fprintf(conn, "quiet%d:1|c\n", i)
		require.NoError(t, err)
	}
	f.waitCounts(t, receiverCounts{
		accepted: 2,
		rejected: map[rejection]uint64{rejectConnectionLimit: 1},
	})

	// A disconnect is routine: it frees a slot and the receiver keeps serving.
	require.NoError(t, first.Close())
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpConnections.Load() == 1 }, waitFor, time.Millisecond)
	next := f.dialTCP(t)
	_, err = next.Write([]byte("next:1|c\n"))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 3,
		rejected: map[rejection]uint64{rejectConnectionLimit: 1},
	})
	f.collect(t, false, false)
	value(t, f.c, "c.total.next", 1, nil)
	value(t, f.c, "receiver.tcp_connections", 2, nil)
	value(t, f.c, "receiver.tcp_connections_limit", 2, nil)
	select {
	case err := <-f.done:
		t.Fatalf("client disconnect ended Run: %v", err)
	default:
	}
}

func TestCancellationClosesClientsAndJoinsReaders(t *testing.T) {
	f := startRuntime(t, nil)
	conn := f.dialTCP(t)
	_, err := conn.Write([]byte("open:1|c\npartial"))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 1,
	})

	require.NoError(t, f.stop(t))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitFor)))
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err, "shutdown closes established clients")
	assert.Zero(t, f.c.diagnostics.tcpConnections.Load())
	requireFree(t, protocolUDP, f.addr)
	requireFree(t, protocolTCP, f.addr)
	require.ErrorIs(t, f.c.Collect(context.Background()), rejectUnavailable)
}

func TestPermanentListenerLossStopsReceiver(t *testing.T) {
	for name, lose := range map[string]func(*server) error{
		"udp": func(s *server) error { return s.udp[0].Close() },
		"tcp": func(s *server) error { return s.tcp[0].Close() },
	} {
		t.Run(name, func(t *testing.T) {
			addr := freeAddress(t)
			c := New()
			c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
			c.profileDirs = nil
			f := prepareFixture(t, c)
			s, err := c.listen(context.Background())
			require.NoError(t, err)
			done := make(chan error, 1)
			ready := make(chan struct{})
			go func() { done <- c.serve(context.Background(), s, func() { close(ready) }) }()
			<-ready
			client, err := net.Dial(protocolTCP, addr)
			require.NoError(t, err)
			defer client.Close()
			_, err = client.Write([]byte("held:10|g\n"))
			require.NoError(t, err)
			f.waitCounts(t, receiverCounts{
				accepted: 1,
			})

			require.NoError(t, lose(s))
			select {
			case err := <-done:
				require.Error(t, err)
				assert.Contains(t, err.Error(), name+" listener")
			case <-time.After(waitFor):
				t.Fatal("listener loss did not end the receiver")
			}
			// No held gauge or empty interval can be published after failure.
			f.cycle.BeginCycle()
			require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
			f.cycle.AbortCycle()
			require.NoError(t, client.SetReadDeadline(time.Now().Add(waitFor)))
			_, err = client.Read(make([]byte, 1))
			require.Error(t, err, "peers are closed with the failed receiver")
			requireFree(t, protocolUDP, addr)
			requireFree(t, protocolTCP, addr)
		})
	}
}

func TestListenerConfigurationValidation(t *testing.T) {
	for name, listeners := range map[string][]ListenerConfig{
		"none":           nil,
		"protocol":       {{Protocol: "unix", Address: "127.0.0.1:1"}},
		"uppercase":      {{Protocol: "UDP", Address: "127.0.0.1:1"}},
		"missing port":   {{Protocol: protocolUDP, Address: "127.0.0.1"}},
		"port zero":      {{Protocol: protocolUDP, Address: "127.0.0.1:0"}},
		"port too large": {{Protocol: protocolTCP, Address: "127.0.0.1:65536"}},
		"named port":     {{Protocol: protocolTCP, Address: "127.0.0.1:statsd"}},
		"duplicate":      {{Protocol: protocolUDP, Address: ":8"}, {Protocol: protocolUDP, Address: ":8"}},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Listeners = listeners
			require.Error(t, c.Init(context.Background()))
		})
	}
	t.Run("tcp cap", func(t *testing.T) {
		c := New()
		c.Listeners = testListeners
		c.MaxTCPConnections = 0
		require.Error(t, c.Init(context.Background()))
	})
	t.Run("same port on both transports and all interfaces", func(t *testing.T) {
		c := New()
		c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: ":18125"}, {Protocol: protocolTCP, Address: ":18125"}}
		require.NoError(t, c.Init(context.Background()))
	})
}

func TestReceiverDiagnostics(t *testing.T) {
	f := startRuntime(t, nil)
	udp := "a:1|c\nbad\n"
	f.sendUDP(t, udp)
	conn := f.dialTCP(t)
	tcp := "b:1|g\nb:1|c\nlatency:10|h\n"
	_, err := conn.Write([]byte(tcp))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 3,
		rejected: map[rejection]uint64{rejectSyntax: 1, rejectType: 1},
	})
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpBytes.Load() == uint64(len(tcp)) }, waitFor, time.Millisecond)
	f.collect(t, false, false)
	value(t, f.c, "receiver.bytes", float64(len(udp)), metrix.Labels{
		"transport": protocolUDP,
	})
	value(t, f.c, "receiver.bytes", float64(len(tcp)), metrix.Labels{
		"transport": protocolTCP,
	})
	value(t, f.c, "receiver.updates", 3, nil)
	for _, reason := range rejectReasons {
		want := map[rejection]float64{rejectSyntax: 1, rejectType: 1}[reason]
		value(t, f.c, "receiver.rejections", want, metrix.Labels{
			"reason": string(reason),
		})
	}
	value(t, f.c, "receiver.series", 3, nil)
	value(t, f.c, "receiver.series_limit", 1000, nil)
	value(t, f.c, "receiver.tcp_connections", 1, nil)

	// A quiet interval is zero activity, not failure; totals repeat.
	f.collect(t, false, false)
	value(t, f.c, "receiver.updates", 3, nil)
}

func TestWithheldPercentileDiagnostics(t *testing.T) {
	f := newCoreFixture(t, 3, time.Minute)
	f.ingest(t, "wide:1|h", "wide:1e20|h", "tiny:1|ms|@1e-40", "fine:5|ms")
	f.collect(t, false, false)
	for _, reason := range withheldReasons {
		want := map[string]float64{withheldSpan: 1, withheldNumericDomain: 1}[reason]
		value(t, f.c, "receiver.percentiles_withheld", want, metrix.Labels{
			"reason": reason,
		})
	}
	// Empty windows have no percentiles to withhold.
	f.collect(t, false, false)
	value(t, f.c, "receiver.percentiles_withheld", 1, metrix.Labels{
		"reason": withheldSpan,
	})
	// TCP-only diagnostics are not written for a UDP-only job.
	_, ok := f.c.store.Read().Value("receiver.tcp_connections", nil)
	assert.False(t, ok)
	_, ok = f.c.store.Read().Value("receiver.bytes", metrix.Labels{
		"transport": protocolTCP,
	})
	assert.False(t, ok)
}

func TestCollectorLifecycleIdempotentCleanup(t *testing.T) {
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolTCP, Address: freeAddress(t)}}
	c.profileDirs = nil
	// Cleanup is safe before Init, after a failed Init and repeatedly.
	c.Cleanup(context.Background())
	bad := New()
	bad.MaxSeries = 0
	require.Error(t, bad.Init(context.Background()))
	bad.Cleanup(context.Background())
	require.NoError(t, c.Init(context.Background()))
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	// A canceled context before acquisition binds nothing and is a normal stop.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, c.Run(ctx, func() { t.Error("ready after cancellation") }))
	requireFree(t, protocolTCP, c.Listeners[0].Address)
}

func TestDiagnosticsChartTemplate(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, string(chartsYAML))
	entry, err := diagnosticsEntry()
	require.NoError(t, err)
	assert.Equal(t, contextNamespace, entry.ContextNamespace)
	// Every diagnostic the collector writes is charted: a TCP job materializes all of them.
	f := startRuntime(t, nil)
	f.collect(t, false, false)
	collecttest.AssertChartCoverage(t, f.c, collecttest.ChartCoverageExpectation{})
}

func TestReaderPanicFailsOnlyTheReceiver(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	prepareFixture(t, c)
	s, err := c.listen(context.Background())
	require.NoError(t, err)
	s.receiver = nil // Any reader panic takes this path; a nil receiver is a deterministic trigger.
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- c.serve(context.Background(), s, func() { close(ready) }) }()
	<-ready
	conn, err := net.Dial(protocolTCP, addr)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("x:1|c\n"))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.ErrorContains(t, err, "receiver panic")
	case <-time.After(waitFor):
		t.Fatal("reader panic did not fail the receiver")
	}
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
}
