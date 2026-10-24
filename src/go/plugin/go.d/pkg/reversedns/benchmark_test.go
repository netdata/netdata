// SPDX-License-Identifier: GPL-3.0-or-later

package reversedns

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var (
	benchmarkResultSink   Result
	benchmarkScheduleSink ScheduleState
)

const benchmarkCacheTTL = time.Duration(1<<63 - 1)

func BenchmarkLookupCached(b *testing.B) {
	for _, state := range []State{StatePositive, StateNegative} {
		b.Run(fmt.Sprintf("state=%d", state), func(b *testing.B) {
			r, addr := newCachedBenchmarkResolver(state)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkResultSink = r.Lookup(addr)
			}
		})
	}
}

func BenchmarkScheduleNoWork(b *testing.B) {
	for _, state := range []State{StatePositive, StateNegative} {
		b.Run(fmt.Sprintf("cached-state=%d", state), func(b *testing.B) {
			r, addr := newCachedBenchmarkResolver(state)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkScheduleSink = r.Schedule(addr)
			}
		})
	}

	b.Run("invalid", func(b *testing.B) {
		r := New(Config{})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkScheduleSink = r.Schedule(netip.Addr{})
		}
	})

	for _, name := range []string{"pending", "saturated"} {
		b.Run(name, func(b *testing.B) {
			release := make(chan struct{})
			started := make(chan struct{})
			r := New(Config{
				LookupTimeout: time.Hour,
				MaxConcurrent: 1,
				Lookup: func(ctx context.Context, _ string) ([]string, error) {
					close(started)
					select {
					case <-release:
						return nil, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
			})
			active := netip.MustParseAddr("192.0.2.10")
			requireScheduledBenchmark(b, r, active)
			<-started
			r.mu.Lock()
			lookupDone := r.calls[active].done
			r.mu.Unlock()
			b.Cleanup(func() {
				close(release)
				<-lookupDone
			})
			addr := active
			if name == "saturated" {
				addr = netip.MustParseAddr("192.0.2.11")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkScheduleSink = r.Schedule(addr)
			}
		})
	}
}

func BenchmarkLookupCachedParallel(b *testing.B) {
	r := New(Config{})
	addr := netip.MustParseAddr("192.0.2.10")
	insertTestResult(r, addr, Result{State: StatePositive, Name: "cached.example.test"})
	r.Lookup(addr)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		var result Result
		for pb.Next() {
			result = r.Lookup(addr)
		}
		runtime.KeepAlive(result)
	})
}

func BenchmarkLookupAndInsertionParallel(b *testing.B) {
	const hotEntries = 256

	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	r := New(Config{
		MaxEntries: 10_000,
		Now:        func() time.Time { return now },
	})
	hot := make([]netip.Addr, hotEntries)
	for i := range hot {
		hot[i] = testAddr(i)
		insertTestResult(r, hot[i], Result{State: StatePositive, Name: "cached.example.test"})
		r.Lookup(hot[i])
	}

	var sequence atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var result Result
		for pb.Next() {
			n := sequence.Add(1)
			if n%4 != 0 {
				result = r.Lookup(hot[n%hotEntries])
				continue
			}

			result = Result{State: StateNegative}
			if n%8 == 0 {
				result = Result{State: StatePositive, Name: "new.example.test"}
			}
			insertTestResult(r, benchmarkAddr(n), result)
		}
		runtime.KeepAlive(result)
	})
}

func BenchmarkOverCapacityInsertion(b *testing.B) {
	for _, capacity := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("capacity=%d", capacity), func(b *testing.B) {
			r := New(Config{MaxEntries: capacity})
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				insertTestResult(r, testAddr(i), Result{State: StatePositive, Name: "host.example.test"})
			}
		})
	}
}

func requireScheduledBenchmark(b *testing.B, r *Resolver, addr netip.Addr) {
	b.Helper()
	if got := r.Schedule(addr); got != ScheduleScheduled {
		b.Fatalf("Schedule() = %d, want %d", got, ScheduleScheduled)
	}
}

func newCachedBenchmarkResolver(state State) (*Resolver, netip.Addr) {
	r := New(Config{
		PositiveTTL: benchmarkCacheTTL,
		NegativeTTL: benchmarkCacheTTL,
		Lookup: func(context.Context, string) ([]string, error) {
			panic("cached reverse DNS benchmark performed a lookup")
		},
	})
	addr := netip.MustParseAddr("192.0.2.10")
	result := Result{State: state}
	if state == StatePositive {
		result.Name = "cached.example.test"
	}
	insertTestResult(r, addr, result)
	if state == StatePositive {
		r.Lookup(addr)
	}
	return r, addr
}

func benchmarkAddr(sequence uint64) netip.Addr {
	var raw [16]byte
	raw[0], raw[1], raw[2], raw[3] = 0x20, 0x01, 0x0d, 0xb8
	binary.BigEndian.PutUint64(raw[8:], sequence)
	return netip.AddrFrom16(raw)
}
