// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

// Stable ticks are O(scheduled jobs + scheduled modules + process providers).
// Changed availability retains the existing asynchronous catalog rebuild cost.
// No-provider ticks add no allocations; timing is a local trend, not a CI gate.
func BenchmarkProcessFunctionTick(b *testing.B) {
	for _, jobs := range []int{0, 1, 128} {
		b.Run(fmt.Sprint(jobs), func(b *testing.B) {
			frames, err := lifecycle.NewFrameOwner(io.Discard)
			require.NoError(b, err)
			generation, err := newTestRunGeneration(b, runGenerationConfig{
				Generation:      1,
				ShutdownTimeout: time.Second,
				UIDs:            lifecycle.NewUIDLedger(),
				Frames:          frames,
				Modules: collectorapi.Registry{
					"module": {},
				},
				Jobs:      testRunJobServices(b),
				Discovery: testRunDiscoveryServices(b),
			})
			require.NoError(b, err)
			require.NoError(b, generation.start(context.Background()))
			b.Cleanup(func() { generation.Stop(); require.NoError(b, generation.Wait(context.Background())) })
			for index := range jobs {
				job := &benchmarkTickJob{
					name: fmt.Sprintf("module_%d", index),
				}
				require.NoError(
					b,
					generation.scheduler.Register(lifecycle.ResourceIdentity{
						ID:         job.FullName(),
						Generation: 1,
					}, job),
				)
			}
			require.Zero(b, testing.AllocsPerRun(100, func() {
				if err := generation.tick(context.Background(), 1); err != nil {
					b.Fatal(err)
				}
			}))
			b.ReportAllocs()
			for b.Loop() {
				if err := generation.tick(context.Background(), 1); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type benchmarkTickJob struct {
	assemblyTestJob
	name string
}

func (j *benchmarkTickJob) FullName() string { return j.name }
