package jobruntime

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type benchmarkRuntimeSupport struct{}

func (benchmarkRuntimeSupport) Start(context.Context) error   { return nil }
func (benchmarkRuntimeSupport) Stop(context.Context) error    { return nil }
func (benchmarkRuntimeSupport) Release(context.Context) error { return nil }

func BenchmarkBV1Cycle(b *testing.B) {
	benchmarkRuntimeCycle(b, func(support []Support) Runtime {
		return NewV1Runtime(support)
	})
}

func BenchmarkBV2Cycle(b *testing.B) {
	benchmarkRuntimeCycle(b, func(support []Support) Runtime {
		return NewV2Runtime(support)
	})
}

func benchmarkRuntimeCycle(
	b *testing.B,
	create func([]Support) Runtime,
) {
	support := []Support{
		benchmarkRuntimeSupport{},
		benchmarkRuntimeSupport{},
	}
	b.ReportAllocs()
	for b.Loop() {
		runtime := create(support)
		if err := runtime.Start(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := runtime.Stop(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := runtime.ReleaseAfterCleanup(
			context.Background(),
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBV2ScopeRegistry(b *testing.B) {
	scope := metrix.HostScope{ScopeKey: "scope"}
	job := &JobV2{
		scopeStates: map[string]*jobV2ScopeState{
			scope.ScopeKey: {
				scopeKey: scope.ScopeKey,
				scope:    scope,
			},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		state, err := job.ensureScopeState(scope)
		if err != nil || state == nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBV2HostState(b *testing.B) {
	state := &jobV2HostState{}
	vnode := vnodes.VirtualNode{
		GUID: "11111111-1111-1111-1111-111111111111",
	}
	b.ReportAllocs()
	for b.Loop() {
		decision, err := state.prepareEmission(vnode)
		if err != nil {
			b.Fatal(err)
		}
		if !decision.targetHost.isVnode() {
			b.Fatal("vnode host decision was lost")
		}
	}
}

func BenchmarkBJobVNodeSnapshot(b *testing.B) {
	snapshot := VnodeSnapshot{
		Vnode: &vnodes.VirtualNode{
			Name:     "node",
			Hostname: "node",
			GUID:     "11111111-1111-1111-1111-111111111111",
		},
		Revision:         1,
		MetadataRevision: 1,
	}
	job := &JobV2{vnodeRevision: snapshot.Revision}
	b.ReportAllocs()
	for b.Loop() {
		job.applyVnodeSnapshot(snapshot)
	}
}
