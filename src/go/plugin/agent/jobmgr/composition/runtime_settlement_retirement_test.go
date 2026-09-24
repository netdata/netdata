// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type retirementStartupCollector struct {
	readinessIntegrationCollector
	started chan struct{}
}

func (c *retirementStartupCollector) Run(ctx context.Context, _ func()) error {
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestRunGenerationStartupRetirementReleasesIdentity(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "rotation target cut"
		if shutdown {
			name = "process shutdown"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			modules := collectorapi.Registry{"module": {
				CreateV2: func() collectorapi.CollectorV2 {
					return &retirementStartupCollector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						started:                       started,
					}
				},
				Config: func() any { return &collectorapi.MockConfiguration{} },
			}}
			cfg := confgroup.Config{"module": "module", "name": "receiver", "update_every": 1}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			frames, err := lifecycle.NewFrameOwner(newProcessSynchronizedBuffer())
			require.NoError(t, err)
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames, Attempts: attempts,
				Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			metrics, err := newRunMetrics(attempts)
			require.NoError(t, err)
			require.NoError(t, generation.kernel.BindRuntimeObserver(metrics))
			t.Cleanup(func() {
				generation.Stop()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, generation.Wait(ctx))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("runtime startup did not begin")
			}
			require.Eventually(t, func() bool {
				return metrics.gaugeValues[lifecycle.RuntimeGaugeOperationsActive].Load() == 0
			}, time.Second, time.Millisecond)
			record, exists := generation.vnodes.graph.Lookup(cfg.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			admitted := metrics.counterValues[lifecycle.RuntimeCounterOperationsAdmitted].Load()

			// Process retirement cuts attempts before stopping run admission. Let
			// the resulting settlement complete in that production-reachable window.
			if shutdown {
				attempts.BeginShutdown()
			} else {
				attempts.CutTarget(1)
			}
			require.Eventually(t, func() bool {
				return generation.run.DirtyCause() != nil ||
					metrics.counterValues[lifecycle.RuntimeCounterOperationsAdmitted].Load() > admitted
			}, time.Second, time.Millisecond, "retirement settlement was not admitted")
			require.NoError(t, generation.run.DirtyCause())
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// The same-lane command runs only after the admitted settlement.
			require.NoError(t, generation.kernel.SubmitPreparedAndWait(ctx, jobmgr.Request{
				UID: "retirement-settlement-barrier", LaneKey: cfg.FullName(),
				Source: lifecycle.SourceJobManager, Route: "internal/test/settlement-barrier",
			}, jobmgr.WorkPlan{
				Work: func(context.Context) (lifecycle.TaskOutcome, error) {
					result, err := lifecycle.NewSealedResult(200, "application/json", nil)
					if err != nil {
						return lifecycle.TaskOutcome{}, err
					}
					return lifecycle.NewFrameOutcome(result)
				},
			}))
			require.NoError(t, generation.run.DirtyCause())
			generation.Stop()
			require.NoError(t, generation.Wait(ctx))
			require.Eventually(t, func() bool { return attempts.Census().Active == 0 }, time.Second, time.Millisecond,
				"startup retirement must release the process-owned identity")
		})
	}
}
