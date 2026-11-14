// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticjob_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vnodeOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *vnodeOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}
func (o *vnodeOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

type vnodeExecutor struct{}

func (vnodeExecutor) Check(context.Context, synthetic.Kind) error       { return nil }
func (vnodeExecutor) Rehearse(context.Context, synthetic.Request) error { return nil }
func (vnodeExecutor) ExecuteScheduled(_ context.Context, r synthetic.Request, state func(string), admitted func(time.Time) synthetic.Schedule) synthetic.Execution {
	at := time.Now()
	schedule := admitted(at)
	state("running")
	return synthetic.Execution{Drained: true, Run: synthetic.Run{
		JobID: r.JobID(), Name: r.Name, Kind: r.Kind, VnodeGUID: r.VnodeGUID,
		Outcome: synthetic.Success, StartedUS: at.UnixMicro(), CompletedUS: time.Now().UnixMicro(),
		ScheduledUS: schedule.DueUS, QueuedUS: schedule.QueuedUS,
	}}
}

func TestNativeSyntheticVnodeAttributionAndChartRouting(t *testing.T) {
	for _, kind := range []string{"journey", "lighthouse"} {
		t.Run(kind, func(t *testing.T) {
			hub := registry.New()
			var collector collectorapi.CollectorV2
			if kind == "journey" {
				c := journey.New(journey.Dependencies{Executor: vnodeExecutor{}, Registry: hub})
				c.Name, c.Script, c.Vnode = "check", "test('visit', async () => {});", "site"
				c.CheckEvery = 1
				collector = c
			} else {
				c := lighthouse.New(lighthouse.Dependencies{Executor: vnodeExecutor{}, Registry: hub})
				c.Name, c.URL, c.Vnode = "check", "https://example.org", "site"
				c.CheckEvery = 1
				collector = c
			}
			first := vnodes.VirtualNode{Name: "site", GUID: "11111111-1111-4111-8111-111111111111", Hostname: "storefront"}
			var snapshot atomic.Pointer[jobruntime.VnodeSnapshot]
			snapshot.Store(&jobruntime.VnodeSnapshot{Vnode: &first, Revision: 1, MetadataRevision: 1})
			out := &vnodeOutput{}
			job := jobruntime.NewJobV2(jobruntime.JobV2Config{
				PluginName: "dem", ModuleName: kind, Name: "check", FullName: kind + "_check",
				Module: collector, Out: out, UpdateEvery: 1, StoreFirst: true,
				Vnode: first, VnodeName: "site", VnodeRevision: 1, VnodeMetadataRevision: 1,
				VnodeLookup: func(string) (jobruntime.VnodeSnapshot, bool) { return snapshot.Load().Copy(), true },
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
					return len(jobs) == 1 && jobs[0].VnodeGUID == guid && jobs[0].Latest != nil &&
						jobs[0].Latest.VnodeGUID == guid && strings.Contains(out.String(), "HOST '"+guid+"'")
				}, 5*time.Second, 10*time.Millisecond)
			}
			collect(first.GUID)
			second := vnodes.VirtualNode{Name: "site", GUID: "22222222-2222-4222-8222-222222222222", Hostname: "documentation"}
			snapshot.Store(&jobruntime.VnodeSnapshot{Vnode: &second, Revision: 2, MetadataRevision: 2})
			collect(second.GUID)
			assert.Contains(t, out.String(), "CHART '"+kind+"_check.")
			assert.NotContains(t, out.String(), "CLABEL 'vnode_guid'")
		})
	}
}
