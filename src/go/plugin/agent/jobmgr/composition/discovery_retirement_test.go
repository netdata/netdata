// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	jobmgrdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryPreparationProcessRetirement(t *testing.T) {
	tests := map[string]struct {
		cut func(*containment.Authority)
	}{
		"process shutdown": {cut: func(a *containment.Authority) { a.BeginShutdown() }},
		"run rotation":     {cut: func(a *containment.Authority) { a.CutTarget(1) }},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			modules := collectorapi.Registry{
				"module": {
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							CheckFunc: func(ctx context.Context) error {
								close(entered)
								<-ctx.Done()
								return ctx.Err()
							},
						}
					},
					Config: func() any { return &collectorapi.MockConfiguration{} },
				},
			}
			frames, err := lifecycle.NewFrameOwner(newProcessSynchronizedBuffer())
			require.NoError(t, err)
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:         testRunSecrets(t),
				Generation:      1,
				ShutdownTimeout: time.Second,
				UIDs:            uids,
				Frames:          frames,
				Attempts:        attempts,
				Modules:         modules,
				Jobs:            testRunJobServices(t),
				Discovery:       testRunDiscoveryServices(t),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				attempts.BeginShutdown()
				generation.Stop()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, generation.Wait(ctx))
				require.NoError(t, attempts.Shutdown(ctx))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			decisions, err := jobmgrdiscovery.NewDecisionIndex(jobmgrdiscovery.DecisionConfig{
				Generation: 1,
				AutoEnable: true,
				Commands:   generation.kernel,
				Plan: func(change jobmgrdiscovery.DiscoveredChange) (jobmgr.WorkPlan, error) {
					return generation.dyncfg.PlanDiscovered(joboutput.DiscoveredJobChange{
						Config: change.Config,
						Status: change.Status,
						Remove: change.Remove,
					})
				},
			})
			require.NoError(t, err)
			cfg := confgroup.Config{
				"module":       "module",
				"name":         "job",
				"update_every": 1,
			}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			applied := make(chan error, 1)
			go func() {
				applied <- decisions.Apply(ctx, []*confgroup.Group{{Source: "file=test", Configs: []confgroup.Config{cfg}}})
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("discovered collector did not enter Check")
			}
			// Production cuts process attempts before cancelling discovery. Keep
			// its context live until that reconciliation has actually returned.
			test.cut(attempts)
			select {
			case err := <-applied:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("retired discovery preparation did not settle")
			}
			require.NoError(t, ctx.Err())
			require.NoError(t, generation.run.DirtyCause())
			_, exists := generation.vnodes.graph.Lookup(cfg.FullName())
			require.False(t, exists, "retirement must not publish the candidate")
		})
	}
}
