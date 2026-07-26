package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func BenchmarkBCommandKernelLaneOps(b *testing.B) {
	var ring readyQueue
	lane := &commandLane{}
	b.ReportAllocs()
	for b.Loop() {
		ring.push(lane)
		if ring.pop() != lane {
			require.FailNow(b, "benchmark failed", "ready lane identity changed")
		}
	}
}

func BenchmarkBKernelMixedTurn(b *testing.B) {
	kernel := &CommandKernel{
		nextSource: lifecycle.SourceJobManager,
	}
	jobManagerLane := &commandLane{}
	functionLane := &commandLane{}
	b.ReportAllocs()
	for b.Loop() {
		kernel.ready[sourceIndex(lifecycle.SourceJobManager)].push(jobManagerLane)
		kernel.ready[sourceIndex(lifecycle.SourceFunction)].push(functionLane)
		if kernel.nextReadyLane() == nil || kernel.nextReadyLane() == nil {
			require.FailNow(b, "benchmark failed", "mixed turn lost a ready lane")
		}
	}
}
