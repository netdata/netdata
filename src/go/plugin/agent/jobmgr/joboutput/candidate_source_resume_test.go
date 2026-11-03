// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestSourceNonAdoptionResumesDependencyIncumbent(t *testing.T) {
	for _, withdrawn := range []bool{false, true} {
		for _, disable := range []bool{false, true} {
			name := map[bool]string{false: "rejected", true: "withdrawn"}[withdrawn]
			if disable {
				name += "/disabled before resume"
			}
			t.Run(name, func(t *testing.T) {
				controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error {
						ready()
						<-ctx.Done()
						return ctx.Err()
					}}
				})
				var available atomic.Bool
				controller.factory.config.Vnode = func(name string) (jobruntime.VnodeSnapshot, bool) {
					if !available.Load() {
						return jobruntime.VnodeSnapshot{}, false
					}
					return jobruntime.VnodeSnapshot{
						Vnode:    &vnodes.VirtualNode{Name: name, Hostname: "remote", GUID: "c61f5634-03c7-4b36-86ed-fd61000cd4d8"},
						Revision: 1, MetadataRevision: 1,
					}, true
				}
				commands := &activationTestCommands{queue: make(chan activationTestSubmission, 8), stop: make(chan struct{})}
				require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("background failure: %v", err) }))
				var current lifecycle.ReadyResource
				t.Cleanup(func() {
					close(commands.stop)
					controller.scheduler.StopBackgroundWorkers()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
					stopRuntimeTestResource(t, current)
				})
				sourceType := confgroup.TypeUser
				if withdrawn {
					sourceType = confgroup.TypeStock
				}
				incumbent := acceptedActivationTestConfig(sourceType).Set("vnode", "remote").Set("option_str", "incumbent")
				observation, err := planSourceTestConfig(controller, incumbent, true, false)
				require.NoError(t, err)
				applyActivationTestSubmission(t, activationTestSubmission{plan: observation}, nil, 0)
				require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1))
				record, exists := graph.Lookup(incumbent.FullName())
				require.True(t, exists)
				require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
				payload := record.Payload()

				proposal := acceptedActivationTestConfig(confgroup.TypeUser).Set("option_str", "${store:invalid}")
				if withdrawn {
					proposal.SetSource("replacement")
				}
				observation, err = planSourceTestConfig(controller, proposal, true, false)
				require.NoError(t, err)
				applyActivationTestSubmission(t, activationTestSubmission{plan: observation}, nil, 0)
				completion := commands.next(t, "internal/jobs/accepted-activation")
				if withdrawn {
					observation, err = planSourceTestConfig(controller, proposal, true, true)
					require.NoError(t, err)
					applyActivationTestSubmission(t, activationTestSubmission{plan: observation}, nil, 0)
				}
				require.Nil(t, applyActivationTestSubmission(t, completion, nil, 2))
				record, _ = graph.Lookup(incumbent.FullName())
				require.Equal(t, payload, record.Payload())
				require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
				resume := commands.next(t, "internal/jobs/candidate-resume")
				if disable {
					prepared, err := controller.Prepare(t.Context(), DynCfgJobRequest{
						Args: []string{controller.externalID(incumbent.FullName()), "disable"},
					}, nil, lifecycle.ResourceTransactionScope{ID: incumbent.FullName()}, lifecycle.LongLivedPermit{})
					require.NoError(t, err)
					_, err = prepared.Apply(t.Context())
					require.NoError(t, err)
				}
				available.Store(true)
				controller.NotifyDependencyChanged("vnode", "remote")
				require.Nil(t, applyActivationTestSubmission(t, resume, nil, 0))
				if disable {
					record, _ = graph.Lookup(incumbent.FullName())
					require.Equal(t, dyncfg.StatusDisabled.String(), record.Status)
					select {
					case call := <-commands.queue:
						t.Fatalf("disabled incumbent resumed through %s", call.request.Route)
					case <-time.After(30 * time.Millisecond):
					}
					return
				}
				current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
				require.NotNil(t, current)
				current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
				record, _ = graph.Lookup(incumbent.FullName())
				require.Equal(t, payload, record.Payload())
				require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			})
		}
	}
}
