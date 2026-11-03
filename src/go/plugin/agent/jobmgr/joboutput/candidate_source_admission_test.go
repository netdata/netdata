// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sourceRevisionTestConfigs(t *testing.T, base confgroup.Config) []confgroup.Config {
	t.Helper()
	configs := make([]confgroup.Config, 3)
	for i, option := range []string{"a", "b", "c"} {
		config, err := base.Clone()
		require.NoError(t, err)
		configs[i] = config.Set("option_str", option)
	}
	slices.SortFunc(configs, func(a, b confgroup.Config) int { return cmp.Compare(a.UID(), b.UID()) })
	return configs
}

func sourceRevisionTestTrial(t *testing.T) (*candidateController, *candidateBinding, *candidateWork) {
	t.Helper()
	c := newCandidateController(1)
	configs := sourceRevisionTestConfigs(t, candidateTestConfig("source", "endpoint"))
	inv := candidateTestInventory(1, true, configs[:2]...)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0).Decision.Work
	require.NotNil(t, second)
	binding := c.snapshot(inv.FullName).Binding

	// B survives the source update and keeps its active work; C joins the pass privately.
	inv.Version++
	inv.Candidates = []DiscoveredJobCandidate{
		inv.Candidates[1],
		{Config: configs[2], Source: "source", Incarnation: 3},
	}
	observed := candidateInventory(t, c, inv, 1)
	require.Nil(t, observed.Decision.Work)
	third := candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true}, 1).Decision.Work
	require.NotNil(t, third)
	require.False(t, third.Tentative)
	require.Equal(t, configs[2], third.Config)
	return c, binding, third
}

func TestCandidatePrivateSourceRevisionPreservesBinding(t *testing.T) {
	for name, outcome := range map[string]candidateOutcome{
		"failed":          {Kind: candidateFailed, Listed: true, RetryAfter: 5},
		"dependency wait": {Kind: candidateWaiting, Listed: true, AdmitWaiting: true},
		"framework wait":  {Kind: candidateFrameworkBlocked, Listed: true, Block: candidateBlockAcceptedWait},
		"framework retry": {Kind: candidateFrameworkBlocked, Listed: true, Block: candidateBlockFailedRetry, RetryAfter: 5},
		"quarantined":     {Kind: candidateFrameworkBlocked, Listed: true, Block: candidateBlockQuarantined},
	} {
		t.Run(name, func(t *testing.T) {
			c, binding, third := sourceRevisionTestTrial(t)
			name := binding.Config.FullName()
			result := candidateResult(t, c, third, outcome, 2)

			assertPrivate := func() {
				t.Helper()
				record := c.snapshot(name)
				assert.Equal(t, binding, record.Binding)
				require.True(t, candidateAuthorized(record, binding.Key), "private work cannot revoke the unchanged public binding")
				assert.True(t, candidateAuthorized(record, third.Token.Candidate), "the private trial retains its own recovery grant")
			}
			assertPrivate()
			if outcome.Kind == candidateWaiting || outcome.Kind == candidateFrameworkBlocked && outcome.Block == candidateBlockAcceptedWait {
				resumed, err := c.prepareResume(name, third.Token, 3)
				applyCandidateDelta(t, resumed, err)
				require.NotNil(t, resumed.Decision.Work)
				assertPrivate()
			} else if outcome.Kind == candidateFrameworkBlocked && outcome.Block == candidateBlockFailedRetry {
				require.NotNil(t, result.Decision.Wake)
				wake := result.Decision.Wake
				resumed, err := c.prepareWake(name, wake.Token, wake.Due)
				applyCandidateDelta(t, resumed, err)
				require.NotNil(t, resumed.Decision.Work)
				assertPrivate()
			}

			disabled, err := c.prepareDisable(name)
			applyCandidateDelta(t, disabled, err)
			enabled := candidateEnable(t, c, 8)
			require.NotNil(t, enabled.Decision.Work, "the exposed job must remain enableable")
		})
	}
}

func TestCandidatePublicSourceRevisionReleasesBindingGrant(t *testing.T) {
	c, binding, third := sourceRevisionTestTrial(t)
	waiting := candidateResult(t, c, third, candidateOutcome{Kind: candidateWaiting, Listed: true, AdmitWaiting: true}, 2)
	require.Nil(t, waiting.Decision.Binding)
	require.True(t, candidateAuthorized(c.snapshot(binding.Config.FullName()), binding.Key))

	admitted := candidateResult(t, c, third, candidateOutcome{Kind: candidateAdmitted, Listed: true}, 3)
	require.Equal(t, third.Token.Candidate, admitted.Decision.Binding.Key)
	assert.False(t, candidateAuthorized(c.snapshot(binding.Config.FullName()), binding.Key),
		"the replaced binding becomes historical and is subject to the source revision cut")
}

func TestCandidateSourceRevisionEnableThroughWorkers(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			check := func(context.Context) error { return errors.New("endpoint unavailable") }
			if version == "v2" {
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{factoryTestV2: &factoryTestV2{check: check}}
				})
			} else {
				creator := controller.modules["module"]
				creator.Create = func() collectorapi.CollectorV1 { return &collectorapi.MockCollectorV1{CheckFunc: check} }
				controller.modules["module"] = creator
			}
			commands := bindHandoffTestWorkers(t, controller)
			configs := sourceRevisionTestConfigs(t, acceptedActivationTestConfig(confgroup.TypeUser).SetSource("common").Set("autodetection_retry", 0))
			inv := DiscoveredJobInventory{
				FullName: configs[0].FullName(), Version: 1, AutoEnable: true,
				Sources: map[string]uint64{"common": 1},
				Candidates: []DiscoveredJobCandidate{
					{Config: configs[0], Source: "common", Incarnation: 1},
					{Config: configs[1], Source: "common", Incarnation: 2},
				},
			}
			plan, err := planInventoryTest(controller, inv)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1)
			pending := commands.next(t, "internal/jobs/accepted-activation")

			inv.Version++
			inv.Candidates = []DiscoveredJobCandidate{
				inv.Candidates[1],
				{Config: configs[2], Source: "common", Incarnation: 3},
			}
			plan, err = planInventoryTest(controller, inv)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			applyActivationTestSubmission(t, pending, nil, 2)
			applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
			record, ok := graph.Lookup(inv.FullName)
			require.True(t, ok)
			config, err := graphRecordConfig(record)
			require.NoError(t, err)
			require.Equal(t, configs[0], config, "unsuccessful private revisions must retain the public config")

			for _, command := range []string{"disable", "enable"} {
				transaction, err := controller.Prepare(t.Context(), DynCfgJobRequest{
					Args: []string{controller.externalID(inv.FullName), command},
				}, nil, lifecycle.ResourceTransactionScope{ID: inv.FullName}, lifecycle.LongLivedPermit{})
				require.NoError(t, err, "public %s must remain usable", command)
				applied, err := transaction.Apply(t.Context())
				require.NoError(t, err)
				want := 200
				if command == "enable" {
					want = 202
				}
				assert.Equal(t, want, applied.ResultStatus())
			}
			record, ok = graph.Lookup(inv.FullName)
			require.True(t, ok)
			assert.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
		})
	}
}
