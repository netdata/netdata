// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSettlementRetirementPreservesOwnership(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		for _, duringPublication := range []bool{false, true} {
			name := "target cut"
			if shutdown {
				name = "process shutdown"
			}
			if duringPublication {
				name += "/during publication"
			} else {
				name += "/before readiness"
			}
			t.Run(name, func(t *testing.T) {
				controller, graph, _, output, _ := newDynCfgJobTestHarness(t)
				commands := dynCfgTestActivationCommands(t, controller)
				started := make(chan struct{})
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error {
						close(started)
						if duringPublication {
							ready()
						}
						<-ctx.Done()
						return ctx.Err()
					}}
				})
				cfg := factoryTestConfig(false)
				seedDynCfgJobGraphRecord(t, graph, cfg, dyncfg.StatusDisabled)
				applyAcceptedEnableForTest(t, controller, graph, cfg, 1)
				current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2).(*JobGeneration)
				t.Cleanup(func() { stopRuntimeTestResource(t, current) })
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("runtime did not start")
				}
				attempts := controller.factory.config.Attempts.(*containment.Authority)
				cut := func() {
					if shutdown {
						attempts.BeginShutdown()
					} else {
						attempts.CutTarget(9)
					}
				}
				if duringPublication {
					publish := current.resources.publishRuntime
					current.resources.publishRuntime = func() error {
						err := publish()
						// Containment can cut independently of the publication lock.
						cut()
						return err
					}
				} else {
					cut()
				}
				plan := commands.next(t, "internal/jobs/runtime-ready").plan
				scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Current: current.Identity()}
				prepared, err := plan.Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				applied, err := prepared.Apply(t.Context())
				require.NoError(t, err)
				actualScope, disposition, owned := applied.Ownership()
				require.Equal(t, scope, actualScope)
				require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
				require.Same(t, current, owned)
				record, exists := graph.Lookup(cfg.FullName())
				require.True(t, exists)
				require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
				require.NotContains(t, output.String(), "status running")
			})
		}
	}
}

func TestRuntimeSettlementPublicationErrorPreservesOwnership(t *testing.T) {
	structural := errors.New("publication failed")
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "structural", err: structural},
		{name: "mixed retirement", err: errors.Join(jobmgr.ErrProcessAttemptRetired, structural)},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			commands := dynCfgTestActivationCommands(t, controller)
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error {
					ready()
					<-ctx.Done()
					return ctx.Err()
				}}
			})
			cfg := factoryTestConfig(false)
			seedDynCfgJobGraphRecord(t, graph, cfg, dyncfg.StatusDisabled)
			applyAcceptedEnableForTest(t, controller, graph, cfg, 1)
			current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2).(*JobGeneration)
			t.Cleanup(func() { stopRuntimeTestResource(t, current) })
			plan := commands.next(t, "internal/jobs/runtime-ready").plan
			publish := current.resources.publishRuntime
			current.resources.publishRuntime = func() error { return errors.Join(publish(), test.err) }
			scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Current: current.Identity()}
			prepared, err := plan.Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.ErrorIs(t, err, structural, "structural errors must remain fail-closed")
			actualScope, disposition, owned := applied.Ownership()
			require.Equal(t, scope, actualScope)
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Same(t, current, owned)
			record, exists := graph.Lookup(cfg.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
		})
	}
}
