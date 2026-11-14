// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticjob_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeSyntheticVnodeAttributionAndChartRouting(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		e := syntheticjobtest.Executor{
			ExecuteFunc: func(_ context.Context, _ synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
				state(synthetic.JobRunning)
				return synthetic.Execution{
					Drained: true,
					Run: synthetic.Run{
						Outcome: synthetic.Success,
					},
				}
			},
		}
		collector := k.new(e, hub, settings{
			name:       "check",
			checkEvery: 1,
		})
		first := vnodes.VirtualNode{
			Name:     "site",
			GUID:     "11111111-1111-4111-8111-111111111111",
			Hostname: "storefront",
		}
		var snapshot atomic.Pointer[jobruntime.VnodeSnapshot]
		snapshot.Store(&jobruntime.VnodeSnapshot{
			Vnode:            &first,
			Revision:         1,
			MetadataRevision: 1,
		})
		out := &output{}
		job := jobruntime.NewJobV2(jobruntime.JobV2Config{
			PluginName:            "dem",
			ModuleName:            module,
			Name:                  "check",
			FullName:              module + "_check",
			Module:                collector,
			Out:                   out,
			UpdateEvery:           1,
			StoreFirst:            true,
			Vnode:                 first,
			VnodeName:             "site",
			VnodeRevision:         1,
			VnodeMetadataRevision: 1,
			VnodeLookup:           func(string) (jobruntime.VnodeSnapshot, bool) { return snapshot.Load().Copy(), true },
		})
		require.NoError(t, job.AutoDetectionManaged(context.Background()))
		run := jobruntime.NewManagedRun(context.Background(), nil)
		done := make(chan struct{})
		go func() { defer close(done); job.StartManaged(run) }()
		t.Cleanup(func() {
			job.Stop()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("job did not stop")
			}
			job.Cleanup()
			assert.Nil(t, run.Failure())
		})
		select {
		case <-run.StartupDone():
		case <-time.After(5 * time.Second):
			t.Fatal("job did not start")
		}
		require.NoError(t, run.StartupErr())
		tick := 0
		collect := func(guid string) {
			t.Helper()
			require.Eventually(t, func() bool {
				tick++
				job.Tick(tick)
				jobs := hub.Snapshot(time.Now())
				if len(jobs) != 1 || jobs[0].VnodeGUID != guid {
					return false
				}
				latest, ok := latestRun(hub, jobs[0])
				return ok && latest.VnodeGUID == guid && strings.Contains(out.String(), "HOST '"+guid+"'")
			}, 5*time.Second, 10*time.Millisecond)
		}
		collect(first.GUID)
		second := vnodes.VirtualNode{
			Name:     "site",
			GUID:     "22222222-2222-4222-8222-222222222222",
			Hostname: "documentation",
		}
		snapshot.Store(&jobruntime.VnodeSnapshot{
			Vnode:            &second,
			Revision:         2,
			MetadataRevision: 2,
		})
		collect(second.GUID)
		assert.Contains(t, out.String(), "CHART '"+module+"_check.")
		assert.NotContains(t, out.String(), "CLABEL 'vnode_guid'")
	})
}
