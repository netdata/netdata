// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCandidateResumeOwnershipWithBlockedPredecessor(t *testing.T) {
	for name, deliverPredecessor := range map[string]bool{
		"proposal resumes without admitting":           false,
		"delayed predecessor cannot displace proposal": true,
	} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			oldStore, newStore := &factoryTestAtomicScope{
				value: "old",
			}, &factoryTestAtomicScope{
				value: "new",
			}
			oldStore.current.Store(true)
			newStore.current.Store(true)
			controller.factory.config.ConfigModules.config.Configs = testConfigResolver(
				t,
				testAtomicResolver(t),
				func([]string) (secretresolver.AtomicScope, error) {
					if oldStore.current.Load() {
						return oldStore, nil
					}
					return newStore, nil
				},
			)
			incumbent := acceptedActivationTestConfig(
				confgroup.TypeUser,
			).Set("option_str", "incumbent").
				Set("autodetection_retry", 0)
			replacement := acceptedActivationTestConfig(
				confgroup.TypeUser,
			).Set("option_str", "${store:vault:test:key}").
				Set("autodetection_retry", 0)
			var constructions atomic.Int32
			var cut atomic.Bool
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				ordinal := constructions.Add(1)
				collector := &runtimeTestCollector{
					factoryTestV2: &factoryTestV2{},
					run:           func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() },
				}
				if ordinal == 1 {
					collector.check = func(ctx context.Context) error {
						cut.Store(
							controller.factory.config.Attempts.CutProcessAttempt(
								jobAttemptIdentity(jobmgr.ProcessAttemptJob, incumbent.FullName()),
								jobmgr.ErrProcessAttemptDeadline,
							),
						)
						<-ctx.Done()
						return context.Cause(ctx)
					}
				}
				return collector
			})
			commands := bindHandoffTestWorkers(t, controller)
			inventory := DiscoveredJobInventory{
				FullName:   incumbent.FullName(),
				Version:    1,
				AutoEnable: true,
				Sources:    map[string]uint64{"source": 1},
				Candidates: []DiscoveredJobCandidate{{Config: incumbent, Source: "source", Incarnation: 1}},
			}
			observe := func() {
				t.Helper()
				plan, err := planInventoryTest(controller, inventory)
				require.NoError(t, err)
				require.Nil(t, applyActivationTestSubmission(t, activationTestSubmission{
					plan: plan,
				}, nil, 0))
			}
			observe()
			require.Nil(
				t,
				applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1),
			)
			require.True(t, cut.Load(), "actual construction must time out before acceptance")
			blocked := controller.candidates.snapshot(incumbent.FullName())
			require.True(t, blocked.FrameworkBlocked)
			require.Equal(t, candidateBlockAcceptedWait, blocked.Block)
			require.Equal(t, incumbent, blocked.Binding.Config)
			require.NotNil(t, blocked.Active)
			require.Nil(t, blocked.Proposal)
			require.True(t, controller.candidates.valid(incumbent.FullName(), blocked.Active.Token))
			predecessorResume := commands.next(t, "internal/jobs/candidate-resume")

			inventory.Version++
			inventory.Candidates = []DiscoveredJobCandidate{{Config: replacement, Source: "source", Incarnation: 2}}
			observe()
			checkedReplacement := commands.next(t, "internal/jobs/accepted-activation")
			proposed := controller.candidates.snapshot(incumbent.FullName())
			require.NotNil(t, proposed.Proposal)
			require.True(t, proposed.Proposal.Tentative)
			require.Equal(t, blocked.Active, proposed.Active)
			require.Equal(t, blocked.Binding, proposed.Binding)
			require.True(t, proposed.FrameworkBlocked)
			oldStore.current.Store(false)
			require.Nil(t, applyActivationTestSubmission(t, checkedReplacement, nil, 2))
			proposalResume := commands.next(t, "internal/jobs/candidate-resume")
			waiting := controller.candidates.snapshot(incumbent.FullName())
			require.Equal(t, proposed.Binding, waiting.Binding, "stale checked proof cannot admit a proposal")
			require.Equal(t, proposed.Active, waiting.Active)
			require.Equal(t, proposed.Proposal, waiting.Proposal)
			require.True(t, waiting.FrameworkBlocked)
			executor, exists := controller.scheduler.accepted.currentToken(incumbent.FullName())
			require.True(t, exists)
			require.Equal(t, waiting.Proposal.Token, executor.work)

			if deliverPredecessor {
				require.Nil(t, applyActivationTestSubmission(t, predecessorResume, nil, 0))
				after := controller.candidates.snapshot(incumbent.FullName())
				require.Equal(
					t,
					waiting.Active,
					after.Active,
					"a delayed predecessor resume cannot consume its block while Proposal owns execution",
				)
				require.Equal(t, waiting.Proposal, after.Proposal)
				require.True(t, after.FrameworkBlocked)
				current, ok := controller.scheduler.accepted.currentToken(incumbent.FullName())
				require.True(t, ok)
				require.Equal(
					t,
					executor,
					current,
					"acknowledging predecessor resume cannot displace the proposal executor",
				)
			}
			require.Nil(t, applyActivationTestSubmission(t, proposalResume, nil, 0))
			resumed := controller.candidates.snapshot(incumbent.FullName())
			require.Equal(t, waiting.Binding, resumed.Binding, "resuming tentative work cannot admit its raw config")
			require.Equal(t, waiting.Active, resumed.Active)
			require.True(t, resumed.FrameworkBlocked, "the predecessor owns its framework block")
			require.Equal(t, candidateBlockAcceptedWait, resumed.Block)
			require.NotNil(t, resumed.Proposal)
			require.True(t, resumed.Proposal.Tentative)
			require.Equal(t, waiting.Proposal.Token.Candidate, resumed.Proposal.Token.Candidate)
			require.Equal(t, waiting.Proposal.Token.Intent, resumed.Proposal.Token.Intent)
			require.NotEqual(t, waiting.Proposal.Token.Attempt, resumed.Proposal.Token.Attempt)
			require.True(t, controller.candidates.valid(incumbent.FullName(), resumed.Active.Token))
			require.True(t, controller.candidates.valid(incumbent.FullName(), resumed.Proposal.Token))
			record, exists := graph.Lookup(incumbent.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			exposed, err := graphRecordConfig(record)
			require.NoError(t, err)
			require.Equal(t, incumbent, exposed)

			rebuiltReplacement := commands.next(t, "internal/jobs/accepted-activation")
			// A metadata-invalid successor withdraws B while retaining the source slot.
			// A's admitted grant survives; no replacement has crossed admission.
			invalid := acceptedActivationTestConfig(confgroup.TypeUser).SetSource("invalid'protocol")
			inventory.Version++
			inventory.Candidates = []DiscoveredJobCandidate{{Config: invalid, Source: "source", Incarnation: 3}}
			observe()
			require.Nil(t, applyActivationTestSubmission(t, rebuiltReplacement, nil, 3))
			recovered := controller.candidates.snapshot(incumbent.FullName())
			require.Equal(t, incumbent, recovered.Binding.Config)
			require.Nil(t, recovered.Proposal)
			require.True(t, controller.candidates.valid(incumbent.FullName(), recovered.Active.Token))
			require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/candidate-resume"), nil, 0))
			current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 4)
			require.NotNil(t, current, "withdrawal must restore actual predecessor execution")
			t.Cleanup(func() { stopRuntimeTestResource(t, current) })
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
			record, exists = graph.Lookup(incumbent.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			exposed, err = graphRecordConfig(record)
			require.NoError(t, err)
			require.Equal(t, incumbent, exposed)
		})
	}
}
