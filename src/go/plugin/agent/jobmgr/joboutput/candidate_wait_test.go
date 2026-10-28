// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestCandidatePrivateDependencyWait(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for _, sourceType := range []string{confgroup.TypeStock, confgroup.TypeUser} {
			for name, remove := range map[string]bool{"dependency arrives": false, "source removed": true} {
				t.Run(version+"/"+sourceType+"/"+name, func(t *testing.T) {
					controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
					var checks atomic.Int32
					check := func(context.Context) error {
						if checks.Add(1) == 1 {
							return errors.New("first endpoint unavailable")
						}
						return nil
					}
					if version == "v2" {
						configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
							return &runtimeTestCollector{
								factoryTestV2: &factoryTestV2{check: check},
								run: func(ctx context.Context, ready func()) error {
									ready()
									<-ctx.Done()
									return ctx.Err()
								},
							}
						})
					} else {
						creator := controller.modules["module"]
						creator.Create = func() collectorapi.CollectorV1 {
							return &collectorapi.MockCollectorV1{
								CheckFunc:  check,
								ChartsFunc: func() *collectorapi.Charts { return &collectorapi.Charts{} },
							}
						}
						controller.modules["module"] = creator
					}
					events := []string{}
					hook := &failureLifecycleHook{
						recordingJobConfigLifecycle: &recordingJobConfigLifecycle{events: &events},
					}
					owner := &intentTestOwner{}
					creator := controller.modules["module"]
					creator.JobConfigLifecycle = hook
					creator.JobConfigIntent = owner
					controller.modules["module"] = creator
					var available atomic.Bool
					controller.factory.config.Vnode = func(name string) (jobruntime.VnodeSnapshot, bool) {
						if !available.Load() {
							return jobruntime.VnodeSnapshot{}, false
						}
						return jobruntime.VnodeSnapshot{
							Vnode: &vnodes.VirtualNode{
								Name:     name,
								Hostname: "remote",
								GUID:     "c61f5634-03c7-4b36-86ed-fd61000cd4d8",
							},
							Revision:         1,
							MetadataRevision: 1,
						}, true
					}
					commands := bindHandoffTestWorkers(t, controller)
					var current lifecycle.ReadyResource
					t.Cleanup(func() { stopRuntimeTestResource(t, current) })
					a := acceptedActivationTestConfig(sourceType).SetSource("a").Set("autodetection_retry", 0)
					b := acceptedActivationTestConfig(sourceType).SetSource("b").Set("vnode", "remote").Set("autodetection_retry", 0)
					require.Less(t, a.UID(), b.UID(), "the unavailable endpoint must precede the dependency wait")
					inventory := DiscoveredJobInventory{
						FullName:   a.FullName(),
						Version:    1,
						AutoEnable: true,
						Sources:    map[string]uint64{"a": 1, "b": 1},
						Candidates: []DiscoveredJobCandidate{
							{Config: a, Source: "a", Incarnation: 1},
							{Config: b, Source: "b", Incarnation: 1},
						},
					}
					plan, err := planInventoryTest(controller, inventory)
					require.NoError(t, err)
					applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
					require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1))
					before, listed := graph.Lookup(a.FullName())
					require.Equal(t, sourceType != confgroup.TypeStock, listed)
					failure, reconciliations := hook.observed, len(hook.reconciliations)
					prepared, committed, aborted, enabled := owner.snapshot()

					require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2))
					after, stillListed := graph.Lookup(a.FullName())
					require.Equal(t, listed, stillListed)
					require.Equal(t, before, after, "a private wait must not mutate the public binding")
					require.Equal(t, failure, hook.observed, "a sibling's failure must not replace the binding's diagnostic")
					require.Len(t, hook.reconciliations, reconciliations)
					p, c, aCount, e := owner.snapshot()
					require.Equal(t, []int{prepared, committed, aborted, enabled}, []int{p, c, aCount, e})
					require.EqualValues(t, 1, checks.Load(), "the missing vnode prevents the sibling Check")
					require.False(t, runtimeTestHasRetry(controller, a.FullName()), "dependency waits do not need operational retries")

					if remove {
						inventory.Version++
						inventory.Candidates = inventory.Candidates[:1]
						delete(inventory.Sources, "b")
						plan, err = planInventoryTest(controller, inventory)
						require.NoError(t, err)
						applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
					}
					available.Store(true)
					controller.NotifyDependencyChanged("vnode", "remote")
					if remove {
						require.Never(t, func() bool { return len(commands.queue) != 0 }, 30*time.Millisecond, time.Millisecond,
							"withdrawn private work must not resume")
						require.EqualValues(t, 1, checks.Load())
						return
					}
					require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/candidate-resume"), nil, 0))
					after, stillListed = graph.Lookup(a.FullName())
					require.Equal(t, listed, stillListed)
					require.Equal(t, before, after, "resuming a private trial still does not admit it")
					require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3),
						"the checked sibling is admitted before the retained stage is installed")
					current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 4)
					require.NotNil(t, current, "the checked sibling must install after the vnode arrives")
					current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
					after, stillListed = graph.Lookup(a.FullName())
					require.True(t, stillListed)
					require.Equal(t, dyncfg.StatusRunning.String(), after.Status)
					config, err := graphRecordConfig(after)
					require.NoError(t, err)
					require.Equal(t, b, config)
					require.EqualValues(t, 2, checks.Load())
				})
			}
		}
	}
}
