// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestFreshActivationWaitsForRuntimeRelease(t *testing.T) {
	for _, command := range []string{"enable", "dependent resume"} {
		for _, ordering := range []string{"release before resume", "release after resume", "disable while waiting"} {
			t.Run(command+"/"+ordering, func(t *testing.T) {
				var inits, checks, created atomic.Int32
				release := make(chan struct{})
				modules := collectorapi.Registry{
					"module": {
						CreateV2: func() collectorapi.CollectorV2 {
							collector := &handoffCollector{
								readinessIntegrationCollector: readinessIntegrationCollector{
									store: metrix.NewCollectorStore(),
								},
								inits:  &inits,
								checks: &checks,
							}
							if created.Add(1) == 1 {
								collector.cleanupGate = release
							}
							return collector
						},
						Config: func() any { return &collectorapi.MockConfiguration{} },
					},
				}
				cfg := confgroup.Config{
					"module":       "module",
					"name":         "receiver",
					"update_every": 1,
				}.
					SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
				output := newProcessSynchronizedBuffer()
				frames, err := lifecycle.NewFrameOwner(output)
				require.NoError(t, err)
				uids := lifecycle.NewUIDLedger()
				generation, err := newTestRunGeneration(t, runGenerationConfig{
					Secrets:         testRunSecrets(t),
					Generation:      1,
					ShutdownTimeout: time.Second,
					UIDs:            uids,
					Frames:          frames,
					Modules:         modules,
					Jobs:            testRunJobServices(t),
					Discovery:       testRunDiscoveryServices(t, cfg),
				})
				require.NoError(t, err)
				released := false
				t.Cleanup(func() {
					if !released {
						close(release)
					}
					generation.Stop()
					require.NoError(t, generation.Wait(context.Background()))
					closeRunTestUIDs(t, uids)
				})
				require.NoError(t, generation.start(context.Background()))
				status := func() string {
					record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
					return record.Status
				}
				require.Eventually(t, func() bool { return status() == dyncfg.StatusRunning.String() }, time.Second, time.Millisecond)
				submit := func(uid, cmd string, code string) {
					t.Helper()
					require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
						UID:      uid,
						Source:   lifecycle.SourceFunction,
						Route:    "config",
						Args:     []string{"go.d:collector:module:receiver", cmd},
						Deadline: time.Now().Add(5 * time.Second),
					}))
					output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" "+code+" ")
				}
				var resume func()
				if command == "enable" {
					submit("disable", "disable", "200")
					resume = func() {
						submit("enable", "enable", "202")
						if !released {
							submit("repeat-enable", "enable", "202")
						}
					}
				} else {
					plan, stopped, err := generation.dyncfg.PlanSecretDependentStop(cfg.FullName())
					require.NoError(t, err)
					require.NoError(t, generation.kernel.SubmitPreparedAndWait(t.Context(), jobmgr.Request{
						UID:     "dependent-stop",
						LaneKey: cfg.FullName(),
						Source:  lifecycle.SourceJobManager,
						Route:   "test/dependent-stop",
					}, plan))
					didStop, err := stopped.Stopped()
					require.NoError(t, err)
					require.True(t, didStop)
					resume = func() {
						plan, resumed, err := generation.dyncfg.PlanSecretDependentStart(cfg.FullName())
						require.NoError(t, err)
						require.NoError(t, generation.kernel.SubmitPreparedAndWait(t.Context(), jobmgr.Request{
							UID:     "dependent-resume",
							LaneKey: cfg.FullName(),
							Source:  lifecycle.SourceJobManager,
							Route:   "test/dependent-resume",
						}, plan))
						require.NoError(t, resumed.Err())
					}
				}
				if ordering == "release before resume" {
					close(release)
					released = true
					// Physical release alone cannot enable a disabled or paused job.
					require.Never(t, func() bool { return created.Load() != 1 }, 50*time.Millisecond, time.Millisecond)
				}
				resume()
				if !released {
					// Acceptance/resume completes while cleanup is blocked, without speculative probes.
					require.Equal(t, dyncfg.StatusAccepted.String(), status())
					require.Never(t, func() bool { return created.Load() != 1 }, 100*time.Millisecond, time.Millisecond)
					if ordering == "disable while waiting" {
						submit("revoke", "disable", "200")
					}
					close(release)
					released = true
				}
				if ordering == "disable while waiting" {
					require.Equal(t, dyncfg.StatusDisabled.String(), status())
					require.Never(t, func() bool { return created.Load() != 1 }, 50*time.Millisecond, time.Millisecond)
					return
				}
				require.Eventually(t, func() bool { return status() == dyncfg.StatusRunning.String() }, time.Second, time.Millisecond)
				require.EqualValues(t, 2, created.Load(), "one initial collector and one successor")
				require.EqualValues(t, 2, inits.Load())
				require.EqualValues(t, 2, checks.Load())
			})
		}
	}
}
