// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"fmt"
	"math"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// No earlier Go StatsD implementation exists for a before/after baseline.
// These benchmarks cover the new component's input and actual native output;
// ns/op is a development-machine trend, never a CI timing gate.
func BenchmarkIngest(b *testing.B) {
	for name, line := range map[string]string{
		"counter":   "requests:2|c|@.5|#route:/api,region:eu",
		"gauge":     "connections:5|g|#pool:main",
		"timer":     "latency:15|ms|@.3|#route:/api",
		"histogram": "difference:-10|h|#queue:work",
		"set":       "members:user-125|s|#pool:main",
	} {
		b.Run(name, func(b *testing.B) {
			f := newCoreFixture(b, 1000, 5*time.Minute)
			f.ingest(b, line)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := f.c.receiver.ingest(line, f.time); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func mixedRecords(n int) []string {
	lines := make([]string, n)
	for i := range n {
		payload := []string{"2|c|@.5", "10|g", "15|ms|@.3", "-10|h", "member-a|s"}[i%5]
		lines[i] = fmt.Sprintf("metric%d:%s|#region:eu,pool:main", i, payload)
	}
	return lines
}

// Stable normal mix: one counter/gauge/timer/histogram/set per five identities.
// Parsing/update is O(record bytes + label sort + bounded estimator update).
// Publication is O(live identities + estimator bins log bins + native output).
func BenchmarkMixedPublication(b *testing.B) {
	for _, size := range []int{100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			f := newCoreFixture(b, size, 5*time.Minute)
			lines := mixedRecords(size)
			for range 3 {
				f.ingest(b, lines...)
				f.collect(b, false, false)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, line := range lines {
					if err := f.c.receiver.ingest(line, f.time); err != nil {
						b.Fatal(err)
					}
				}
				f.collect(b, false, false)
			}
		})
	}
}

// A rejected new identity at capacity scans the O(max_series) entries only once
// an entry without pending input can have expired; otherwise, and with idle
// expiry disabled, rejection is O(1).
func BenchmarkCapacityPressure(b *testing.B) {
	for name, tc := range map[string]struct {
		idle    time.Duration
		pending bool
	}{
		"idle expiry":               {idle: 5 * time.Minute},
		"idle expiry pending input": {idle: 5 * time.Minute, pending: true},
		"idle disabled":             {},
	} {
		b.Run(name, func(b *testing.B) {
			f := newCoreFixture(b, 1000, tc.idle)
			f.ingest(b, mixedRecords(1000)...)
			f.collect(b, false, false)
			if tc.pending {
				f.ingest(b, mixedRecords(1000)...)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := f.c.receiver.ingest("additional:1|c", f.time); err != rejectCapacity {
					b.Fatal(err)
				}
			}
		})
	}
}

// Run with -benchtime=1x. Measures retained Go heap, not process RSS or a peak
// allocation guarantee. Both receiving and detached windows remain reachable.
func BenchmarkOwnershipEnvelope(b *testing.B) {
	for _, shape := range []string{"mixed1000", "signed1000"} {
		b.Run(shape, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f := newCoreFixture(b, 1000, 5*time.Minute)
				lines := mixedRecords(1000)
				feed := func() { f.ingest(b, lines...) }
				if shape == "signed1000" {
					// Saturated sign stores are a capacity envelope, not a typical workload.
					feed = func() {
						for series := 0; series < 1000; series++ {
							for bin := 0; bin < 1024; bin++ {
								v := math.Exp(float64(bin) * .018001) // Below 1024 occupied log bins at .009 mapping.
								f.ingest(
									b,
									fmt.Sprintf("metric%d:%g|h", series, v),
									fmt.Sprintf("metric%d:%g|h", series, -v),
								)
							}
						}
					}
				}
				runtime.GC()
				var before, windows, published runtime.MemStats
				runtime.ReadMemStats(&before)
				feed()
				f.cycle.BeginCycle()
				cut, err := f.c.receiver.cut(f.time, 0)
				if err != nil {
					b.Fatal(err)
				}
				batch := cut.batch
				feed()
				runtime.GC()
				runtime.ReadMemStats(&windows)
				for _, m := range batch {
					f.c.writeMeasurement(m)
				}
				f.c.receiver.release(batch)
				f.finish(b, false, false)
				runtime.GC()
				runtime.ReadMemStats(&published)
				b.ReportMetric(float64(windows.HeapAlloc-before.HeapAlloc)/1e6, "two_windows_MB")
				b.ReportMetric(float64(published.HeapAlloc-before.HeapAlloc)/1e6, "with_native_MB")
				runtime.KeepAlive(f)
				runtime.KeepAlive(batch)
			}
		})
	}
}

// Profile preprocessing on the ingest path: owner matching over configured
// profiles, one replace pipeline and, until activation, root matching.
func BenchmarkIngestProfiles(b *testing.B) {
	for name, line := range map[string]string{
		"replace owner": "svc.a.size:5|g|#region:eu",
		"second owner":  "svc.other:2|c|@.5|#region:eu",
		"no owner":      "plain:15|ms|#region:eu",
	} {
		b.Run(name, func(b *testing.B) {
			f := newProfileFixture(b, testProfiles, "pools", "shadow", "app", "meta")
			f.ingest(b, line)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := f.c.receiver.ingest(line, f.time); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Complete TCP path: socket read, framing, parsing, admission and aggregation.
// Allocation counts include the server goroutines; ns/op is per record.
func BenchmarkTCPReceive(b *testing.B) {
	f := startRuntime(b, func(c *Collector) { c.MetricIdleTimeout = 0 })
	lines := mixedRecords(1000)
	conn := f.dialTCP(b)
	var payload []byte
	for _, line := range lines {
		payload = append(payload, line...)
		payload = append(payload, '\n')
	}
	b.ReportAllocs()
	b.ResetTimer()
	sent := 0
	for sent < b.N {
		n := min(len(lines), b.N-sent)
		chunk := payload
		if n < len(lines) {
			chunk = []byte(strings.Join(lines[:n], "\n") + "\n")
		}
		if _, err := conn.Write(chunk); err != nil {
			b.Fatal(err)
		}
		sent += n
	}
	deadline := time.Now().Add(time.Minute)
	for f.counts().accepted < uint64(b.N) {
		if time.Now().After(deadline) {
			b.Fatalf("accepted %d of %d", f.counts().accepted, b.N)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// datagrams packs records into LF-joined UDP payloads of perDatagram records each.
func datagrams(lines []string, perDatagram int) [][]byte {
	var out [][]byte
	for len(lines) > 0 {
		n := min(perDatagram, len(lines))
		out = append(out, []byte(strings.Join(lines[:n], "\n")))
		lines = lines[n:]
	}
	return out
}

// UDP framing and ingestion without the socket, for single-record and batched
// datagrams of admitted identities. ns/op and allocations are per record.
func BenchmarkDatagram(b *testing.B) {
	for _, perDatagram := range []int{1, 16} {
		b.Run(fmt.Sprint(perDatagram), func(b *testing.B) {
			f := newCoreFixture(b, 1000, 5*time.Minute)
			s := &server{
				receiver: f.c.receiver,
				stats:    f.c.diagnostics,
				now:      f.c.now,
			}
			// 960 identities divide evenly into datagrams and the five-type mix.
			payloads := datagrams(mixedRecords(960), perDatagram)
			for _, p := range payloads {
				s.datagram(p)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i += perDatagram {
				s.datagram(payloads[(i/perDatagram)%len(payloads)])
			}
			b.StopTimer()
			require.Empty(b, f.counts().rejected)
		})
	}
}

// Complete UDP path: socket read, datagram framing, parsing, admission and
// aggregation, 16 records per datagram. Sends pause every window datagrams until
// the receiver has admitted them, so loopback buffers never drop input.
// Allocation counts include the server goroutines; ns/op is per record.
func BenchmarkUDPReceive(b *testing.B) {
	const perDatagram, window = 16, 128
	f := startRuntime(b, func(c *Collector) { c.MetricIdleTimeout = 0 })
	payloads := datagrams(mixedRecords(960), perDatagram)
	conn, err := net.Dial(protocolUDP, f.addr)
	require.NoError(b, err)
	b.Cleanup(func() { _ = conn.Close() })
	b.ReportAllocs()
	b.ResetTimer()
	sent := 0
	deadline := time.Now().Add(time.Minute)
	for d := 0; sent < b.N; d++ {
		if _, err := conn.Write(payloads[d%len(payloads)]); err != nil {
			b.Fatal(err)
		}
		sent += perDatagram
		if d%window == window-1 || sent >= b.N {
			for f.counts().accepted < uint64(sent) {
				if time.Now().After(deadline) {
					b.Fatalf("accepted %d of %d", f.counts().accepted, sent)
				}
				time.Sleep(50 * time.Microsecond)
			}
		}
	}
}

// Normal mixed publication with profile entries active and one replace owner.
func BenchmarkMixedPublicationProfiles(b *testing.B) {
	f := newProfileFixture(b, testProfiles, "pools", "shadow", "app", "meta")
	lines := mixedRecords(1000)
	for i := range 100 {
		lines[i] = fmt.Sprintf("svc.p%d.size:%d|g", i, i)
	}
	for range 3 {
		f.ingest(b, lines...)
		f.collect(b, false, false)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			if err := f.c.receiver.ingest(line, f.time); err != nil {
				b.Fatal(err)
			}
		}
		f.collect(b, false, false)
	}
}

// Run with -benchtime=1x. Retained Go heap of a running receiver: listeners, a
// full TCP client cap with per-connection framing buffers, active profiles,
// mixed1000 receiving state and one publication. Not process RSS.
func BenchmarkRuntimeEnvelope(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		f := startRuntime(b, func(c *Collector) {
			c.profileDirs = writeProfiles(b, testProfiles)
			c.Profiles = []string{"pools", "shadow", "app", "meta"}
		})
		lines := mixedRecords(1000)
		for j := range 100 {
			lines[j] = fmt.Sprintf("svc.p%d.size:%d|g", j, j)
		}
		conns := make([]net.Conn, f.c.MaxTCPConnections)
		for j := range conns {
			conns[j] = f.dialTCP(b)
		}
		for j, line := range lines {
			if _, err := fmt.Fprintln(conns[j%len(conns)], line); err != nil {
				b.Fatal(err)
			}
		}
		deadline := time.Now().Add(time.Minute)
		for f.counts().accepted < uint64(len(lines)) {
			if time.Now().After(deadline) {
				b.Fatalf("accepted %d of %d", f.counts().accepted, len(lines))
			}
			time.Sleep(time.Millisecond)
		}
		f.collect(b, false, false)
		for j, line := range lines {
			if _, err := fmt.Fprintln(conns[j%len(conns)], line); err != nil {
				b.Fatal(err)
			}
		}
		for f.counts().accepted < uint64(2*len(lines)) {
			if time.Now().After(deadline) {
				b.Fatalf("accepted %d of %d", f.counts().accepted, 2*len(lines))
			}
			time.Sleep(time.Millisecond)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/1e6, "retained_MB")
		b.ReportMetric(float64(f.c.diagnostics.tcpConnections.Load()), "tcp_clients")
		runtime.KeepAlive(f)
		require.NoError(b, f.stop(b))
	}
}
