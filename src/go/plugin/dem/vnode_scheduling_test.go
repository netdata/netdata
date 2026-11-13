// SPDX-License-Identifier: GPL-3.0-or-later
package dem_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vnodeMonitor interface {
	collectorapi.CollectorV2
	collectorapi.CollectorV2Runner
	collectorapi.ConfiguredVnodeConsumer
}

type vnodeAttempt struct {
	request synthetic.Request
	state   func(string)
	finish  chan struct{}
}

type heldVnodeExecutor struct {
	vnodeExecutor
	calls chan vnodeAttempt
}

func (e heldVnodeExecutor) ExecuteScheduled(ctx context.Context, r synthetic.Request, state func(string), admit func(time.Time) synthetic.Schedule) synthetic.Execution {
	at := time.Now()
	schedule := admit(at)
	call := vnodeAttempt{request: r, state: state, finish: make(chan struct{})}
	state("running")
	e.calls <- call
	outcome := synthetic.Success
	select {
	case <-call.finish:
	case <-ctx.Done():
		outcome = synthetic.Cancelled
	}
	duration, performance := 123.0, 90.0
	return synthetic.Execution{Drained: true, Run: synthetic.Run{
		ID: "attempt", JobID: r.JobID(), Kind: r.Kind, Name: r.Name, VnodeGUID: r.VnodeGUID,
		Outcome: outcome, StartedUS: at.UnixMicro(), CompletedUS: time.Now().UnixMicro(),
		ScheduledUS: schedule.DueUS, QueuedUS: schedule.QueuedUS, DurationMS: &duration,
		Tests:   &synthetic.TestCounts{Declared: 1, Passed: 1},
		Metrics: &synthetic.LabMetrics{Performance: &performance},
	}}
}

func TestSyntheticVnodeChangeFencesBackgroundPublication(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	for _, kind := range []string{"journey", "lighthouse"} {
		for name, changes := range map[string][]string{"different": {b}, "aba": {b, a}, "metadata": {a}} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				t.Parallel()
				hub := registry.New()
				e := heldVnodeExecutor{calls: make(chan vnodeAttempt, 1)}
				var c vnodeMonitor
				if kind == "journey" {
					job := journey.New(journey.Dependencies{Executor: e, Registry: hub})
					job.Name, job.Script, job.CheckEvery = "check", "test('visit', async () => {});", 1
					c = job
				} else {
					job := lighthouse.New(lighthouse.Dependencies{Executor: e, Registry: hub})
					job.Name, job.URL, job.CheckEvery = "check", "https://example.org", 1
					c = job
				}
				c.SetConfiguredVnode(vnodes.VirtualNode{GUID: a, Hostname: "first"})
				require.NoError(t, c.Init(t.Context()))
				ctx, cancel := context.WithCancel(t.Context())
				ready, done := make(chan struct{}), make(chan error, 1)
				go func() { done <- c.Run(ctx, func() { close(ready) }) }()
				t.Cleanup(func() {
					cancel()
					select {
					case err := <-done:
						assert.NoError(t, err)
					case <-time.After(5 * time.Second):
						t.Error("monitor did not join its executor")
					}
					c.Cleanup(context.Background())
					assert.Empty(t, hub.Snapshot(time.Now()))
				})
				select {
				case <-ready:
				case <-time.After(5 * time.Second):
					t.Fatal("monitor did not become ready")
				}
				next := func() vnodeAttempt {
					t.Helper()
					select {
					case call := <-e.calls:
						return call
					case <-time.After(5 * time.Second):
						t.Fatal("monitor did not submit an attempt")
						return vnodeAttempt{}
					}
				}
				old := next()
				for _, guid := range changes {
					c.SetConfiguredVnode(vnodes.VirtualNode{GUID: guid, Hostname: "changed"})
				}
				assert.Equal(t, a, old.request.VnodeGUID)
				old.state("running") // A delayed callback must obey the same fence as completion.
				if name != "metadata" {
					assert.Equal(t, "unknown", hub.Snapshot(time.Now())[0].State)
				}
				close(old.finish)
				require.Eventually(t, func() bool {
					job := hub.Snapshot(time.Now())[0]
					return job.Schedule.Outcomes.Success == 1 && (name != "metadata" || job.Latest != nil)
				}, 5*time.Second, time.Millisecond)
				job := hub.Snapshot(time.Now())[0]
				assert.Equal(t, changes[len(changes)-1], job.VnodeGUID)
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.EqualValues(t, 1, values["attempts_success"])
				if name == "metadata" {
					require.NotNil(t, job.Latest)
					assert.EqualValues(t, 123, values["duration"])
				} else {
					assert.Nil(t, job.Latest)
					assert.Zero(t, job.LastSuccessUS)
					assert.Zero(t, job.LastFailureUS)
					assert.NotContains(t, values, "duration")
					assert.NotContains(t, values, "observation_age")
					assert.NotContains(t, values, "performance")
					assert.NotContains(t, values, "tests_passed")
					assert.EqualValues(t, 1, values[`execution_state{execution_state="unknown"}`])
				}
				current := next()
				assert.Equal(t, job.VnodeGUID, current.request.VnodeGUID)
				close(current.finish)
				require.Eventually(t, func() bool {
					job := hub.Snapshot(time.Now())[0]
					return job.Schedule.Outcomes.Success == 2 && job.Latest != nil
				}, 5*time.Second, time.Millisecond)
				job = hub.Snapshot(time.Now())[0]
				require.NotNil(t, job.Latest)
				assert.Equal(t, job.VnodeGUID, job.Latest.VnodeGUID)
				values, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.EqualValues(t, 123, values["duration"])
				c.SetConfiguredVnode(vnodes.VirtualNode{GUID: "", Hostname: "physical-agent"})
				values, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.NotContains(t, values, "duration", "completed observation clears on reassignment too")
				assert.Nil(t, hub.Snapshot(time.Now())[0].Latest)
			})
		}
	}
}
