// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deferredTransitionFixture struct {
	controller *candidateController
	inventory  DiscoveredJobInventory
	configs    []confgroup.Config
	proposal   *candidateWork
	pending    candidateKey
	base       time.Time
	wall       time.Time
}

// All three alternatives fail in one unchanged source lifetime. Removing A
// selects B; removing B during its authorized preparation selects delayed C.
func newDeferredTransitionFixture(t *testing.T, cadence int64) *deferredTransitionFixture {
	t.Helper()
	f := &deferredTransitionFixture{
		controller: newCandidateController(1),
		configs:    sourceRevisionTestConfigs(t, candidateTestConfig("source", "endpoint")),
		base:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	f.wall = f.base
	f.controller.wallNow = func() time.Time { return f.wall }
	f.inventory = candidateTestInventory(1, true, f.configs...)
	c := f.controller
	first := candidateInventory(t, c, f.inventory, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true}, 0).Decision.Work
	require.NotNil(t, second)
	third := candidateResult(t, c, second, candidateOutcome{
		Kind: candidateFailed, Listed: true,
		Retry: candidateRetryPolicy{cadence: cadence, notBefore: f.base.Add(10 * time.Second)},
	}, 0).Decision.Work
	require.NotNil(t, third)
	candidateResult(t, c, third, candidateOutcome{
		Kind: candidateFailed, Listed: true,
		Retry: candidateRetryPolicy{cadence: cadence, notBefore: f.base.Add(time.Minute)},
	}, 0)
	f.pending = third.Token.Candidate
	f.inventory.Version++
	f.inventory.Candidates = f.inventory.Candidates[1:]
	require.Nil(t, candidateInventory(t, c, f.inventory, 1).Decision.Work)
	wake := c.snapshot(f.inventory.FullName).Wake
	require.NotNil(t, wake)
	f.wall = f.base.Add(10 * time.Second)
	released, err := c.prepareWake(f.inventory.FullName, wake.Token, wake.Due)
	applyCandidateDelta(t, released, err)
	f.proposal = released.Decision.Work
	require.NotNil(t, f.proposal)
	require.True(t, f.proposal.Tentative)
	require.Equal(t, second.Token.Candidate, f.proposal.Token.Candidate)
	f.inventory.Version++
	f.inventory.Candidates = f.inventory.Candidates[1:]
	require.Nil(t, candidateInventory(t, c, f.inventory, 12).Decision.Work)
	require.True(t, c.valid(f.inventory.FullName, f.proposal.Token), "historical admission still authorizes B's preparation")
	record := c.snapshot(f.inventory.FullName)
	require.NotNil(t, record.DeferredSource)
	require.Equal(t, f.pending, record.DeferredSource.Observation.Key)
	require.NotNil(t, record.Wake)
	return f
}

func TestDeferredSourceSurvivesOlderProposalOutcome(t *testing.T) {
	for _, cadence := range []int64{0, 120} {
		for _, elapsed := range []bool{false, true} {
			for name, outcome := range map[string]candidateOutcome{
				"admitted":        {Kind: candidateAdmitted},
				"dependency wait": {Kind: candidateWaiting, AdmitWaiting: true},
				"accepted wait":   {Kind: candidateFrameworkBlocked, Block: candidateBlockAcceptedWait},
				"failed":          {Kind: candidateFailed, Listed: true},
				"blocked retry":   {Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, Retry: candidateRetryPolicy{cadence: 5}},
				"quarantined":     {Kind: candidateFrameworkBlocked, Block: candidateBlockQuarantined},
			} {
				t.Run(fmt.Sprintf("%s/cadence=%d/elapsed=%t", name, cadence, elapsed), func(t *testing.T) {
					f := newDeferredTransitionFixture(t, cadence)
					c, id := f.controller, f.inventory.FullName
					oldWake := c.snapshot(id).Wake
					f.wall = f.base.Add(20 * time.Second)
					if elapsed {
						f.wall = f.base.Add(2 * time.Minute)
					}
					if outcome.Kind == candidateFrameworkBlocked && outcome.Block == candidateBlockFailedRetry {
						// A tentative deadline is AcceptedWait. FailedRetry belongs
						// to the subsequent admitted activation's deadline instead.
						admitted := candidateResult(t, c, f.proposal, candidateOutcome{Kind: candidateAdmitted}, 13)
						require.Nil(t, admitted.Decision.Work)
					}
					accepted := candidateResult(t, c, f.proposal, outcome, 13)
					require.Nil(t, accepted.Decision.Work, "acceptance must not dispatch another candidate during checked-stage handoff")
					if outcome.Kind == candidateAdmitted {
						ready, err := c.prepareReady(id, f.proposal.Token)
						applyCandidateDelta(t, ready, err)
						require.True(t, c.snapshot(id).SearchClosed)
					}
					// An unchanged observation cannot manufacture the missing transition.
					f.inventory.Version++
					require.Nil(t, candidateInventory(t, c, f.inventory, 13).Decision.Work)
					record := c.snapshot(id)
					require.NotNil(t, record.DeferredSource, "older acceptance cannot consume the newer source observation")
					require.Equal(t, f.pending, record.DeferredSource.Observation.Key)
					require.Equal(t, f.proposal.Token.Intent, record.Intent)
					stale, err := c.prepareWake(id, oldWake.Token, oldWake.Due)
					require.True(t, applyCandidateDelta(t, stale, err).Decision.Ignored)
					if outcome.Kind == candidateFrameworkBlocked && outcome.Block == candidateBlockQuarantined {
						require.Nil(t, record.Wake, "source selection cannot bypass quarantine")
						return
					}
					if outcome.Kind == candidateFrameworkBlocked && outcome.Block == candidateBlockFailedRetry {
						require.NotNil(t, record.Wake)
						resumed, err := c.prepareWake(id, record.Wake.Token, record.Wake.Due)
						applyCandidateDelta(t, resumed, err)
						require.NotNil(t, resumed.Decision.Work)
						require.Equal(t, f.proposal.Token.Candidate, resumed.Decision.Work.Token.Candidate)
					}
					wake := c.snapshot(id).Wake
					require.NotNil(t, wake, "current source must retain an autonomous release path")
					if !elapsed {
						early, err := c.prepareWake(id, wake.Token, wake.Due)
						applyCandidateDelta(t, early, err)
						require.Nil(t, early.Decision.Work)
						require.NotNil(t, early.Decision.Wake)
						require.Greater(t, early.Decision.Wake.Due, wake.Due)
						wake = early.Decision.Wake
					}
					f.wall = f.base.Add(2 * time.Minute)
					released, err := c.prepareWake(id, wake.Token, wake.Due)
					applyCandidateDelta(t, released, err)
					require.NotNil(t, released.Decision.Work)
					require.Equal(t, f.pending, released.Decision.Work.Token.Candidate)
					require.True(t, released.Decision.Work.Tentative)
					duplicate, err := c.prepareWake(id, wake.Token, wake.Due)
					require.True(t, applyCandidateDelta(t, duplicate, err).Decision.Ignored)
					require.Nil(t, duplicate.Decision.Work)
				})
			}
		}
	}
}

func TestDeferredSourceSelectionSurvivesHistoricalCursorChanges(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%t", rejected), func(t *testing.T) {
			f := newDeferredTransitionFixture(t, 0)
			c, id := f.controller, f.inventory.FullName
			// This is a fresh membership, not reuse of the removed incarnation.
			// Its earlier key must not displace already-selected C at the same tier.
			f.inventory.Version++
			f.inventory.Candidates = append(f.inventory.Candidates, DiscoveredJobCandidate{
				Config: f.configs[0], Source: "source", Incarnation: 40,
			})
			require.Nil(t, candidateInventory(t, c, f.inventory, 12).Decision.Work)
			before := c.snapshot(id)
			require.Equal(t, f.pending, candidateSourceChoice(before).Observation.Key)
			kind := candidateAdmitted
			if rejected {
				kind = candidateRejected
			}
			result, err := c.prepareOutcome(id, f.proposal.Token, candidateOutcome{Kind: kind}, 13)
			require.NoError(t, err)
			require.Equal(t, before, c.snapshot(id), "unapplied outcome must not mutate the live observation or wake")
			applyCandidateDelta(t, result, err)
			after := c.snapshot(id)
			require.NotNil(t, after.DeferredSource)
			require.Equal(t, f.pending, after.DeferredSource.Observation.Key)
			unchanged := cloneCandidateRecord(after)
			require.Equal(t, f.pending, candidateSourceChoice(after).Observation.Key)
			assert.Equal(t, unchanged, after, "source selection must not cancel or alter pending state")
			if result.Decision.Work != nil {
				require.False(t, result.Decision.Work.Tentative, "ordinary sibling recovery is allowed; source selection remains C")
			}
			f.wall = f.base.Add(2 * time.Minute)
			require.NotNil(t, after.Wake)
			released, err := c.prepareWake(id, after.Wake.Token, after.Wake.Due)
			applyCandidateDelta(t, released, err)
			require.NotNil(t, released.Decision.Work)
			require.Equal(t, f.pending, released.Decision.Work.Token.Candidate)
			require.True(t, released.Decision.Work.Tentative)
		})
	}
}

func TestDeferredSourceSurvivesUnacceptedWaitThenAdmission(t *testing.T) {
	f := newDeferredTransitionFixture(t, 0)
	c, id := f.controller, f.inventory.FullName
	before := c.snapshot(id)
	waiting := candidateResult(t, c, f.proposal, candidateOutcome{Kind: candidateWaiting}, 12)
	require.Nil(t, waiting.Decision.Work)
	require.Equal(t, before.Intent, c.snapshot(id).Intent)
	require.Equal(t, before.DeferredSource, c.snapshot(id).DeferredSource)
	f.wall = f.base.Add(2 * time.Minute)
	accepted := candidateResult(t, c, f.proposal, candidateOutcome{Kind: candidateAdmitted}, 13)
	require.Nil(t, accepted.Decision.Work)
	require.NotNil(t, accepted.Decision.Wake)
	require.EqualValues(t, 13, accepted.Decision.Wake.Due, "elapsed source is scheduled, not dispatched during admission")
	released, err := c.prepareWake(id, accepted.Decision.Wake.Token, 13)
	applyCandidateDelta(t, released, err)
	require.NotNil(t, released.Decision.Work)
	require.Equal(t, f.pending, released.Decision.Work.Token.Candidate)
}

func TestDeferredSourceInvalidationFallsBackToAdmissionHistory(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected replacement=%t", rejected), func(t *testing.T) {
			f := newDeferredTransitionFixture(t, 0)
			c, id := f.controller, f.inventory.FullName
			candidateResult(t, c, f.proposal, candidateOutcome{Kind: candidateAdmitted}, 13)
			require.NotNil(t, c.snapshot(id).DeferredSource)
			f.inventory.Version++
			f.inventory.Candidates = nil
			var invalid map[candidateKey]bool
			if rejected {
				replacement := DiscoveredJobCandidate{
					Config: candidateTestConfig("source", "rejected replacement"), Source: "source", Incarnation: 40,
				}
				f.inventory.Candidates = []DiscoveredJobCandidate{replacement}
				// Metadata rejection enters before source arbitration, as it does
				// in PlanInventory; no executable candidate grants this authority.
				invalid = map[candidateKey]bool{candidateIdentity(replacement): true}
			}
			delta, err := c.prepareInventoryValidated(f.inventory, invalid, 14)
			applyCandidateDelta(t, delta, err)
			record := c.snapshot(id)
			require.Nil(t, record.DeferredSource)
			require.Nil(t, record.Wake)
			require.Nil(t, delta.Decision.Work)
			if !rejected {
				require.Nil(t, record.Binding, "an empty source has no protected rejected revision")
				require.Nil(t, candidateSourceChoice(record))
				require.True(t, delta.Decision.Remove)
				return
			}
			require.NotNil(t, record.Binding)
			require.Equal(t, f.proposal.Token.Candidate, record.Binding.Key)
			choice := candidateSourceChoice(record)
			require.NotNil(t, choice, "the still-present source protects admitted history")
			require.Equal(t, record.Binding.Key, choice.Observation.Key)
		})
	}
}

// The real factory loads this field from each source config. The fake provider's
// response depends on those inputs and its request history, not controller state.
type deferredHandoffCollector struct {
	*runtimeTestCollector
	OptionStr string `yaml:"option_str"`
}

func TestDeferredSourceSurvivesCheckedStageHandoff(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var wall atomic.Int64
	wall.Store(base.UnixNano())
	controller.candidates.wallNow = func() time.Time { return time.Unix(0, wall.Load()) }
	configs := sourceRevisionTestConfigs(t, acceptedActivationTestConfig(confgroup.TypeUser).SetSource("source").Set("autodetection_retry", 0))
	options := []string{configs[0].Get("option_str").(string), configs[1].Get("option_str").(string), configs[2].Get("option_str").(string)}
	deadlines := map[string]time.Time{
		options[1]: base.Add(10 * time.Second),
		options[2]: base.Add(time.Minute),
	}
	var checksMu sync.Mutex
	checks := make(map[string]int)
	runs := make(chan string, 4)
	creator := controller.modules["module"]
	creator.Create = nil
	creator.CreateV2 = func() collectorapi.CollectorV2 {
		collector := &deferredHandoffCollector{
			runtimeTestCollector: &runtimeTestCollector{
				factoryTestV2: &factoryTestV2{store: metrix.NewCollectorStore(), template: factoryTestChartTemplate},
			},
		}
		collector.check = func(context.Context) error {
			checksMu.Lock()
			defer checksMu.Unlock()
			checks[collector.OptionStr]++
			if checks[collector.OptionStr] == 1 {
				return collectorapi.WithRetryNotBefore(errors.New("provider unavailable"), deadlines[collector.OptionStr])
			}
			return nil
		}
		collector.run = func(ctx context.Context, ready func()) error {
			runs <- collector.OptionStr
			ready()
			<-ctx.Done()
			return ctx.Err()
		}
		return collector
	}
	controller.modules["module"] = creator
	commands := bindHandoffTestWorkers(t, controller)
	controller.scheduler.retries.advance(0)
	var current lifecycle.ReadyResource
	t.Cleanup(func() { stopRuntimeTestResource(t, current) })
	inv := DiscoveredJobInventory{
		FullName: configs[0].FullName(), Version: 1, AutoEnable: true,
		Sources: map[string]uint64{"source": 1},
	}
	for i, config := range configs {
		inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: config, Source: "source", Incarnation: uint64(i + 1)})
	}
	observe := func() {
		t.Helper()
		plan, err := planInventoryTest(controller, inv)
		require.NoError(t, err)
		current = applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, current, 0)
	}
	observe()
	for generation := uint64(1); generation <= 3; generation++ {
		current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), current, generation)
		require.Nil(t, current)
	}
	inv.Version++
	inv.Candidates = inv.Candidates[1:]
	observe()
	wall.Store(base.Add(10 * time.Second).UnixNano())
	controller.scheduler.retries.advance(10)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/autodetection-retry"), current, 0)
	preparedB := commands.next(t, "internal/jobs/accepted-activation")
	inv.Version++
	inv.Candidates = inv.Candidates[1:]
	observe()
	pending := controller.candidates.snapshot(inv.FullName)
	require.NotNil(t, pending.DeferredSource)
	require.Equal(t, configs[2].UID(), pending.DeferredSource.Observation.Key.UID)
	// B's successful Check was completed before C was selected. Its stage must
	// be handed to B exactly once, while C keeps an independent future wake.
	current = applyActivationTestSubmission(t, preparedB, current, 4)
	require.Nil(t, current)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), current, 5)
	require.NotNil(t, current)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
	require.Equal(t, options[1], <-runs, "the retained B stage must not be attached to C")
	record := controller.candidates.snapshot(inv.FullName)
	require.Equal(t, dyncfg.StatusRunning, record.Binding.Health)
	require.False(t, record.entry(record.Binding.Key).Observed)
	require.NotNil(t, record.Wake, "readiness must preserve current C's scheduled source transition")
	recoveryMatrixDispatcher(t, controller, inv.FullName, record.Wake)
	checksMu.Lock()
	beforeRelease := map[string]int{options[0]: checks[options[0]], options[1]: checks[options[1]], options[2]: checks[options[2]]}
	checksMu.Unlock()
	require.Equal(t, map[string]int{options[0]: 1, options[1]: 2, options[2]: 1}, beforeRelease)
	wall.Store(base.Add(2 * time.Minute).UnixNano())
	controller.scheduler.retries.advance(int(record.Wake.Due))
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/autodetection-retry"), current, 0)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for generation := uint64(6); ; generation++ {
		binding, exists := graph.Lookup(inv.FullName)
		if exists && binding.Status == dyncfg.StatusRunning.String() {
			config, err := graphRecordConfig(binding)
			require.NoError(t, err)
			if config.UID() == configs[2].UID() {
				break
			}
		}
		select {
		case call := <-commands.queue:
			current = applyActivationTestSubmission(t, call, current, generation)
		case <-deadline.C:
			t.Fatal("current source C did not become Running after its provider minimum")
		}
	}
	require.Equal(t, options[2], <-runs)
	checksMu.Lock()
	defer checksMu.Unlock()
	require.Equal(t, map[string]int{options[0]: 1, options[1]: 2, options[2]: 2}, checks, "each checked stage is consumed once without a repeated probe")
}
