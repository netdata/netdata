// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vnodeAttempt struct {
	request synthetic.Request
	state   func(synthetic.JobState)
	finish  chan struct{}
}

// heldExecutor hands each admitted attempt to the test and holds it until finish or cancellation.
func heldExecutor(calls chan<- vnodeAttempt) syntheticjobtest.Executor {
	return syntheticjobtest.Executor{
		ExecuteFunc: func(ctx context.Context, r synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
			call := vnodeAttempt{
				request: r,
				state:   state,
				finish:  make(chan struct{}),
			}
			state(synthetic.JobRunning)
			calls <- call
			outcome := synthetic.Success
			select {
			case <-call.finish:
			case <-ctx.Done():
				outcome = synthetic.Cancelled
			}
			return synthetic.Execution{
				Drained: true,
				Run: synthetic.Run{
					ID:         "attempt",
					Outcome:    outcome,
					DurationMS: new(123.0),
					Tests: &synthetic.TestCounts{
						Declared: 1,
						Passed:   1,
					},
					Metrics: &synthetic.LabMetrics{
						Performance: new(90.0),
					},
				},
			}
		},
	}
}

func TestSyntheticVnodeChangeFencesBackgroundPublication(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	forEachKind(t, func(t *testing.T, _ string, k kind) {
		for name, changes := range map[string][]string{"different": {b}, "aba": {b, a}, "metadata": {a}} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				hub := registry.New()
				calls := make(chan vnodeAttempt, 1)
				c := k.new(heldExecutor(calls), hub, settings{
					name:       "check",
					checkEvery: 1,
				})
				c.SetConfiguredVnode(vnodes.VirtualNode{
					GUID:     a,
					Hostname: "first",
				})
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
					case call := <-calls:
						return call
					case <-time.After(5 * time.Second):
						t.Fatal("monitor did not submit an attempt")
						return vnodeAttempt{}
					}
				}
				old := next()
				for _, guid := range changes {
					c.SetConfiguredVnode(vnodes.VirtualNode{
						GUID:     guid,
						Hostname: "changed",
					})
				}
				assert.Equal(t, a, old.request.VnodeGUID)
				old.state(synthetic.JobRunning) // A delayed callback must obey the same fence as completion.
				if name != "metadata" {
					assert.Equal(t, synthetic.JobUnknown, hub.Snapshot(time.Now())[0].State)
				}
				close(old.finish)
				require.Eventually(t, func() bool {
					job := hub.Snapshot(time.Now())[0]
					return job.Outcomes.Success == 1 && (name != "metadata" || job.HasObservation)
				}, 5*time.Second, time.Millisecond)
				job := hub.Snapshot(time.Now())[0]
				assert.Equal(t, changes[len(changes)-1], job.VnodeGUID)
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.EqualValues(t, 1, values["attempts_success"])
				if name == "metadata" {
					require.True(t, job.HasObservation)
					assert.EqualValues(t, 123, values["duration"])
				} else {
					assert.False(t, job.HasObservation)
					assert.Zero(t, job.LastSuccessUS)
					assert.Zero(t, job.LastFailureUS)
					assert.NotContains(t, values, "duration")
					assert.NotContains(t, values, "observation_age")
					for _, name := range measurementNames() {
						assert.NotContains(t, values, name)
					}
					assert.EqualValues(t, 1, values[`execution_state{execution_state="unknown"}`])
				}
				current := next()
				assert.Equal(t, job.VnodeGUID, current.request.VnodeGUID)
				close(current.finish)
				require.Eventually(t, func() bool {
					job := hub.Snapshot(time.Now())[0]
					return job.Outcomes.Success == 2 && job.HasObservation
				}, 5*time.Second, time.Millisecond)
				job = hub.Snapshot(time.Now())[0]
				latest, ok := latestRun(hub, job)
				require.True(t, ok)
				assert.Equal(t, job.VnodeGUID, latest.VnodeGUID)
				values, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.EqualValues(t, 123, values["duration"])
				c.SetConfiguredVnode(vnodes.VirtualNode{
					GUID:     "",
					Hostname: "physical-agent",
				})
				values, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.NotContains(t, values, "duration", "completed observation clears on reassignment too")
				assert.False(t, hub.Snapshot(time.Now())[0].HasObservation)
			})
		}
	})
}
