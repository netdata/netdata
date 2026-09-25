// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestDynCfgRestartUsesIndependentInvocationLane(t *testing.T) {
	route, err := newDynCfgJobInitialRoute(1, "go.d:collector:", &dynCfgJobBinding{})
	require.NoError(t, err)
	catalog, err := functionadapter.NewCatalog([]functionadapter.Declaration{route.Declaration})
	require.NoError(t, err)
	for _, command := range []string{"restart", "RESTART", "update"} {
		decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
			UID:   command,
			Route: "config",
			Args:  []string{"go.d:collector:module:job", command},
		})
		require.NoError(t, err)
		require.Zero(t, decision.Rejected)
		if command == "update" {
			require.Equal(t, "module_job", decision.ResourceID)
			require.NotNil(t, decision.Plan.Transaction)
		} else {
			require.Empty(t, decision.ResourceID, "RESTART must not hold the lane its inner mutation needs")
			require.Nil(t, decision.Plan.Transaction)
			require.NotNil(t, decision.Plan.Work)
		}
		_, err = catalog.ReleaseInvocation(decision.Lease)
		require.NoError(t, err)
	}
}

type restartIntegrationCollector struct {
	readinessIntegrationCollector
	run    func(context.Context, func()) error
	reject *atomic.Bool
}

func (collector *restartIntegrationCollector) Run(ctx context.Context, ready func()) error {
	return collector.run(ctx, ready)
}

func (collector *restartIntegrationCollector) Check(context.Context) error {
	if collector.reject.Load() {
		return collectorapi.PermanentError(errors.New("restart preflight rejected"))
	}
	return nil
}

func TestDynCfgRestartObservesExactActivationOutsideResourceLane(t *testing.T) {
	for _, outcome := range []string{"ready", "failure", "disable", "cancel", "deadline", "preflight"} {
		t.Run(outcome, func(t *testing.T) {
			var runs atomic.Int32
			var reject atomic.Bool
			started, release := make(chan struct{}), make(chan struct{})
			modules := collectorapi.Registry{"module": {
				CreateV2: func() collectorapi.CollectorV2 {
					return &restartIntegrationCollector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						reject:                        &reject,
						run: func(ctx context.Context, ready func()) error {
							if runs.Add(1) > 1 {
								close(started)
								select {
								case <-release:
								case <-ctx.Done():
									return ctx.Err()
								}
								if outcome == "failure" {
									return errors.New("restart listener bind failed")
								}
							}
							ready()
							<-ctx.Done()
							return ctx.Err()
						},
					}
				},
				Config: func() any { return &collectorapi.MockConfiguration{} },
			}}
			cfg := confgroup.Config{"module": "module", "name": "receiver", "update_every": 1}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			output := newProcessSynchronizedBuffer()
			frames, err := lifecycle.NewFrameOwner(output)
			require.NoError(t, err)
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:    testRunSecrets(t),
				Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
				Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				generation.Stop()
				require.NoError(t, generation.Wait(context.Background()))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			require.Eventually(t, func() bool {
				record, exists := generation.vnodes.graph.Lookup(cfg.FullName())
				return exists && record.Status == dyncfg.StatusRunning.String()
			}, time.Second, time.Millisecond)
			if outcome == "preflight" {
				reject.Store(true)
			}
			submit := func(uid, command string) {
				deadline := time.Now().Add(3 * time.Second)
				if outcome == "deadline" && uid == "restart" {
					deadline = time.Now().Add(300 * time.Millisecond)
				}
				require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
					UID: uid, Source: lifecycle.SourceFunction, Route: "config",
					Args:     []string{"go.d:collector:module:receiver", command},
					Deadline: deadline,
				}))
			}
			submit("restart", "restart")
			if outcome != "preflight" {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("restart did not begin runtime startup")
				}
				require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN restart ", "the old running generation cannot answer")
			}
			code := 200
			switch outcome {
			case "disable":
				submit("disable", "disable")
				output.waitContains(t, "FUNCTION_RESULT_BEGIN disable 200 ")
				code = 503
			case "cancel", "deadline":
				code = 504
				if outcome == "cancel" {
					code = 499
					require.NoError(t, generation.kernel.Cancel(t.Context(), "restart"))
				}
				output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN restart %d ", code))
				close(release)
				require.Eventually(t, func() bool {
					record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
					return record.Status == dyncfg.StatusRunning.String()
				}, time.Second, time.Millisecond, "cancelling observation must not cancel the adopted generation")
			case "preflight":
				code = 422
			case "failure":
				close(release)
				code = 503
			default:
				close(release)
			}
			output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN restart %d ", code))
			require.Equal(t, 1, strings.Count(output.String(), "FUNCTION_RESULT_BEGIN restart "))
			require.NoError(t, generation.run.DirtyCause())
			if outcome == "failure" {
				require.Contains(t, output.String(), "restart listener bind failed", "the specific runtime failure must win over generic retirement")
			}
			if outcome == "preflight" {
				require.Contains(t, output.String(), "restart preflight rejected")
				require.EqualValues(t, 1, runs.Load())
				record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
				require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			}
		})
	}
}
