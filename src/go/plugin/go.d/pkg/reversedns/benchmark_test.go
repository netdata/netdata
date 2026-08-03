// SPDX-License-Identifier: GPL-3.0-or-later

package reversedns

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
)

var (
	benchmarkResultSink   Result
	benchmarkScheduleSink ScheduleState
)

func BenchmarkLookupCached(b *testing.B) {
	for _, state := range []State{StatePositive, StateNegative} {
		b.Run(fmt.Sprintf("state=%d", state), func(b *testing.B) {
			r := New(Config{})
			addr := netip.MustParseAddr("192.0.2.10")
			result := Result{State: state}
			if state == StatePositive {
				result.Name = "cached.example.test"
			}
			insertTestResult(r, addr, result)
			if state == StatePositive {
				r.Lookup(addr)
			}

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
			r := New(Config{})
			addr := netip.MustParseAddr("192.0.2.10")
			result := Result{State: state}
			if state == StatePositive {
				result.Name = "cached.example.test"
			}
			insertTestResult(r, addr, result)
			if state == StatePositive {
				r.Lookup(addr)
			}

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
			b.Cleanup(func() {
				close(release)
				waitForNoCallsBenchmark(r)
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
		for pb.Next() {
			benchmarkResultSink = r.Lookup(addr)
		}
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

func waitForNoCallsBenchmark(r *Resolver) {
	for {
		r.mu.Lock()
		done := len(r.calls) == 0
		r.mu.Unlock()
		if done {
			return
		}
	}
}
