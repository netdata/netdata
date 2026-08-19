// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var benchmarkLiveScopesSink map[string]metrix.HostScope

func BenchmarkBV2LiveScopeSetWarm(b *testing.B) {
	const seriesPerScope = 8

	for _, totalScopes := range []int{1, 8, 64, 512} {
		b.Run(fmt.Sprintf("scopes_%d/series_per_scope_%d", totalScopes, seriesPerScope), func(b *testing.B) {
			store, _ := benchmarkJobV2ScopedStore(b, totalScopes, seriesPerScope)
			job := &JobV2{store: store}
			live := job.liveScopeSet()
			if len(live) != totalScopes {
				b.Fatalf("expected %d live scopes, got %d", totalScopes, len(live))
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				live = job.liveScopeSet()
				if len(live) != totalScopes {
					b.Fatalf("expected %d live scopes, got %d", totalScopes, len(live))
				}
				benchmarkLiveScopesSink = live
			}
		})
	}
}

func BenchmarkBV2CollectAndEmitMultiScope(b *testing.B) {
	const seriesPerScope = 1

	for _, totalScopes := range []int{1, 8, 64} {
		b.Run(fmt.Sprintf("scopes_%d/series_per_scope_%d", totalScopes, seriesPerScope), func(b *testing.B) {
			store, handles := benchmarkJobV2ScopedStore(b, totalScopes, seriesPerScope)
			module := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				collectFunc: func(context.Context) error {
					for i, handle := range handles {
						handle.Observe(metrix.SampleValue(i + 1))
					}
					return nil
				},
			}
			job := NewJobV2(JobV2Config{
				PluginName:  pluginName,
				Name:        jobName,
				ModuleName:  modName,
				FullName:    modName + "_" + jobName,
				Module:      module,
				Out:         io.Discard,
				UpdateEvery: 1,
			})
			job.Mute()
			if err := job.postCheck(); err != nil {
				b.Fatalf("initialize JobV2: %v", err)
			}
			job.runOnce()
			if job.panicked.Load() || job.retries.Load() != 0 {
				b.Fatal("warm-up collection failed")
			}
			if len(job.scopeStates) != totalScopes {
				b.Fatalf("expected %d scope states, got %d", totalScopes, len(job.scopeStates))
			}
			b.Cleanup(job.Cleanup)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				job.runOnce()
				if job.panicked.Load() || job.retries.Load() != 0 {
					b.Fatal("collection failed")
				}
			}
		})
	}
}

func BenchmarkBV2CollectAndEmitRetainedScopes(b *testing.B) {
	const totalScopes = 64

	store, handles := benchmarkJobV2ScopedStore(b, totalScopes, 1)
	observeAll := true
	module := &mockModuleV2{
		store:    store,
		template: chartTemplateV2ExpireAfterOne(),
		collectFunc: func(context.Context) error {
			for i, handle := range handles {
				if observeAll || i%2 == 0 {
					handle.Observe(metrix.SampleValue(i + 1))
				}
			}
			observeAll = !observeAll
			return nil
		},
	}
	job := NewJobV2(JobV2Config{
		PluginName:  pluginName,
		Name:        jobName,
		ModuleName:  modName,
		FullName:    modName + "_" + jobName,
		Module:      module,
		Out:         io.Discard,
		UpdateEvery: 1,
	})
	job.Mute()
	if err := job.postCheck(); err != nil {
		b.Fatalf("initialize JobV2: %v", err)
	}
	job.runOnce()
	if job.panicked.Load() || job.retries.Load() != 0 {
		b.Fatal("warm-up collection failed")
	}
	b.Cleanup(job.Cleanup)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		job.runOnce()
		if job.panicked.Load() || job.retries.Load() != 0 {
			b.Fatal("collection failed")
		}
	}
}

func benchmarkJobV2ScopedStore(b *testing.B, totalScopes, seriesPerScope int) (metrix.CollectorStore, []metrix.SnapshotGauge) {
	b.Helper()
	if totalScopes < 1 || seriesPerScope < 1 {
		b.Fatalf("invalid JobV2 fixture: %d scopes with %d series each", totalScopes, seriesPerScope)
	}

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("store does not expose cycle control")
	}
	cycle := managed.CycleController()
	gaugeVec := store.Write().SnapshotMeter("apache").Vec("id").Gauge("workers_busy")
	handles := make([]metrix.SnapshotGauge, 0, totalScopes*seriesPerScope)

	for scopeIndex := range totalScopes {
		scope := benchmarkJobV2HostScope(scopeIndex)
		for seriesIndex := range seriesPerScope {
			labelValue := strconv.Itoa(scopeIndex) + "-" + strconv.Itoa(seriesIndex)
			var (
				handle metrix.SnapshotGauge
				err    error
			)
			if scope.IsDefault() {
				handle, err = gaugeVec.GetWithLabelValues(labelValue)
			} else {
				handle, err = gaugeVec.WithHostScope(scope).GetWithLabelValues(labelValue)
			}
			if err != nil {
				b.Fatalf("create JobV2 gauge handle: %v", err)
			}
			handles = append(handles, handle)
		}
	}

	cycle.BeginCycle()
	for i, handle := range handles {
		handle.Observe(metrix.SampleValue(i + 1))
	}
	if err := cycle.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit JobV2 fixture: %v", err)
	}
	return store, handles
}

func benchmarkJobV2HostScope(index int) metrix.HostScope {
	if index == 0 {
		return metrix.HostScope{}
	}
	value := strconv.Itoa(index)
	return metrix.HostScope{
		ScopeKey: "scope-" + value,
		GUID:     "guid-" + value,
		Hostname: "host-" + value,
		Labels:   map[string]string{"scope": value},
	}
}
