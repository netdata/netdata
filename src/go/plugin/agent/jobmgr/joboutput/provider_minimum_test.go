// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderDeadlineSurvivesSiblingPromotion(t *testing.T) {
	for name, cadence := range map[string]int64{"retryable": 1, "ordinary recovery disabled": 0} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wall := base
			c.wallNow = func() time.Time { return wall }
			inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"), candidateTestConfig("b", "b"))
			first := candidateInventory(t, c, inv, 0).Decision.Work
			second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{cadence: cadence, notBefore: base.Add(time.Minute)}}, 0).Decision.Work
			require.NotNil(t, second)
			candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{cadence: cadence, notBefore: base.Add(time.Minute)}}, 0)
			var retained []DiscoveredJobCandidate
			for _, member := range inv.Candidates {
				if member.Source == second.Config.Source() {
					retained = append(retained, member)
				}
			}
			inv.Version = 2
			inv.Candidates = retained
			delete(inv.Sources, first.Config.Source())
			promoted := candidateInventory(t, c, inv, 1)
			require.Nil(t, promoted.Decision.Work, "source promotion cannot bypass an unchanged candidate's provider minimum")
			assertDeferredRelease(t, c, inv.FullName, second.Token.Candidate, &wall)
		})
	}
}

func TestProviderDeadlineSurvivesUnrelatedSearchReopen(t *testing.T) {
	c := newCandidateController(1)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wall := base
	c.wallNow = func() time.Time { return wall }
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a").SetSourceType(confgroup.TypeStock), candidateTestConfig("b", "b").SetSourceType(confgroup.TypeStock))
	first := candidateInventory(t, c, inv, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{cadence: 1, notBefore: base.Add(time.Minute)}}, 0).Decision.Work
	require.NotNil(t, second)
	candidateResult(t, c, second, candidateOutcome{Kind: candidateAdmitted}, 0)
	ready, err := c.prepareReady(inv.FullName, second.Token)
	applyCandidateDelta(t, ready, err)
	require.Equal(t, base.Add(time.Minute), c.snapshot(inv.FullName).entry(first.Token.Candidate).NotBefore)
	replacement := candidateTestConfig("c", "replacement")
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: replacement, Source: "c", Incarnation: 3})
	inv.Sources["c"] = 1
	proposed := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposed)
	require.Equal(t, replacement, proposed.Config)
	candidateResult(t, c, proposed, candidateOutcome{Kind: candidateFailed, Listed: true}, 1)
	require.Equal(t, base.Add(time.Minute), c.snapshot(inv.FullName).entry(first.Token.Candidate).NotBefore)
	var retained []DiscoveredJobCandidate
	for _, member := range inv.Candidates {
		if member.Source == first.Config.Source() {
			retained = append(retained, member)
		}
	}
	inv.Version = 3
	inv.Candidates = retained
	delete(inv.Sources, second.Config.Source())
	delete(inv.Sources, "c")
	next := candidateInventory(t, c, inv, 2)
	require.Nil(t, next.Decision.Work, "unrelated source admission cannot erase the same candidate's provider minimum")
	assertDeferredRelease(t, c, inv.FullName, first.Token.Candidate, &wall)
}

// Drive a compressed early wake, then elapsed provider time. Every action uses
// prepared controller transitions; no test mutates candidate ledger state.
func assertDeferredRelease(t *testing.T, c *candidateController, name string, key candidateKey, wall *time.Time) {
	t.Helper()
	before := c.snapshot(name)
	require.NotNil(t, before.DeferredSource)
	require.NotNil(t, before.Wake)
	require.Nil(t, before.Proposal)
	oldWake := before.Wake
	early, err := c.prepareWake(name, oldWake.Token, oldWake.Due)
	applyCandidateDelta(t, early, err)
	require.Nil(t, early.Decision.Work)
	require.NotNil(t, early.Decision.Wake)
	require.Greater(t, early.Decision.Wake.Due, oldWake.Due)
	stale, err := c.prepareWake(name, oldWake.Token, oldWake.Due)
	assert.True(t, applyCandidateDelta(t, stale, err).Decision.Ignored)
	*wall = wall.Add(time.Minute)
	ready, err := c.prepareWake(name, early.Decision.Wake.Token, early.Decision.Wake.Due)
	applyCandidateDelta(t, ready, err)
	require.NotNil(t, ready.Decision.Work)
	assert.Equal(t, key, ready.Decision.Work.Token.Candidate)
	assert.True(t, ready.Decision.Work.Tentative)
	assert.Nil(t, c.snapshot(name).DeferredSource)
	assert.True(t, c.snapshot(name).entry(key).NotBefore.IsZero())
	duplicate, err := c.prepareWake(name, early.Decision.Wake.Token, early.Decision.Wake.Due)
	assert.True(t, applyCandidateDelta(t, duplicate, err).Decision.Ignored)
}

// A manual exact activation of the lower source does not reset the higher
// source's separate identity. A subsequent inventory transition selects it.
func deferredSourceFixture(t *testing.T, pending confgroup.Config, disabled bool) (*candidateController, DiscoveredJobInventory, *candidateWork, candidateKey, *time.Time) {
	t.Helper()
	c := newCandidateController(1)
	wall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.wallNow = func() time.Time { return wall }
	lower := candidateTestConfig("lower", "lower").SetSourceType(confgroup.TypeStock)
	inv := candidateTestInventory(1, true, pending, lower)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	require.Equal(t, pending, first.Config)
	failed := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{notBefore: wall.Add(time.Minute)}}, 0)
	require.Nil(t, failed.Decision.Wake, "metadata alone cannot enable ordinary retry")
	exact, err := c.prepareExact(lower, true, 0)
	applyCandidateDelta(t, exact, err)
	predecessor := exact.Decision.Work
	require.NotNil(t, predecessor)
	candidateResult(t, c, predecessor, candidateOutcome{Kind: candidateAdmitted}, 0)
	if disabled {
		d, err := c.prepareDisable(inv.FullName)
		applyCandidateDelta(t, d, err)
	}
	inv.Version = 2
	other := candidateTestConfig("other", "other").SetSourceType(confgroup.TypeStock)
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: other, Source: "other", Incarnation: 3})
	inv.Sources["other"] = 1
	deferred := candidateInventory(t, c, inv, 1)
	require.Nil(t, deferred.Decision.Work)
	require.NotNil(t, c.snapshot(inv.FullName).DeferredSource)
	if !disabled {
		require.Equal(t, predecessor.Token, c.snapshot(inv.FullName).Active.Token)
		require.True(t, c.valid(inv.FullName, predecessor.Token), "delayed source owns no executor")
	}
	return c, inv, predecessor, first.Token.Candidate, &wall
}

func TestDeferredSourceSurvivesPredecessorSettlement(t *testing.T) {
	for _, outcome := range []string{"ready", "invalid", "accepted wait", "after-ready failure", "disabled before selection"} {
		t.Run(outcome, func(t *testing.T) {
			c, inv, predecessor, pending, wall := deferredSourceFixture(t, candidateTestConfig("higher", "higher"), outcome == "disabled before selection")
			switch outcome {
			case "ready":
				ready, err := c.prepareReady(inv.FullName, predecessor.Token)
				applyCandidateDelta(t, ready, err)
				assert.True(t, c.snapshot(inv.FullName).SearchClosed)
			case "invalid":
				candidateResult(t, c, predecessor, candidateOutcome{Kind: candidateAdmittedInvalid, Listed: true}, 2)
				assert.Equal(t, dyncfg.StatusFailed, c.snapshot(inv.FullName).Binding.Health)
			case "accepted wait":
				candidateResult(t, c, predecessor, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockAcceptedWait, Listed: true}, 2)
				assert.Equal(t, dyncfg.StatusAccepted, c.snapshot(inv.FullName).Binding.Health)
			case "after-ready failure":
				candidateResult(t, c, predecessor, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, AfterReady: true, Listed: true}, 2)
				assert.True(t, c.snapshot(inv.FullName).SearchClosed)
			case "disabled before selection":
				assert.Equal(t, candidateDisabled, c.snapshot(inv.FullName).Enablement)
			}
			assert.Equal(t, predecessor.Token.Candidate, c.snapshot(inv.FullName).Binding.Key)
			assertDeferredRelease(t, c, inv.FullName, pending, wall)
		})
	}
}

func TestDeferredSourceInvalidation(t *testing.T) {
	for _, action := range []string{"disable", "exact restart", "source removal", "new incarnation", "higher selection", "trust withdrawal"} {
		t.Run(action, func(t *testing.T) {
			pending := candidateTestConfig("higher", "higher")
			if action == "trust withdrawal" {
				pending = trustedCandidateConfig("higher", "higher", true)
			}
			c, inv, predecessor, key, _ := deferredSourceFixture(t, pending, false)
			old := c.snapshot(inv.FullName).Wake
			require.NotNil(t, old)
			switch action {
			case "disable":
				d, err := c.prepareDisable(inv.FullName)
				applyCandidateDelta(t, d, err)
				assert.False(t, c.snapshot(inv.FullName).entry(key).NotBefore.IsZero())
			case "exact restart":
				d, err := c.prepareExact(predecessor.Config, true, 2)
				applyCandidateDelta(t, d, err)
				require.NotNil(t, d.Decision.Work)
				assert.Equal(t, predecessor.Token.Candidate, d.Decision.Work.Token.Candidate)
			case "source removal":
				inv.Version++
				inv.Candidates = inv.Candidates[1:]
				delete(inv.Sources, pending.Source())
				candidateInventory(t, c, inv, 2)
			case "new incarnation":
				inv.Version++
				inv.Candidates[0].Incarnation = 10
				d := candidateInventory(t, c, inv, 2)
				require.NotNil(t, d.Decision.Work)
				assert.NotEqual(t, key, d.Decision.Work.Token.Candidate)
			case "higher selection":
				inv.Version++
				next := candidateTestConfig("highest", "highest").SetSourceType(confgroup.TypeDyncfg)
				inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: next, Source: "highest", Incarnation: 10})
				inv.Sources["highest"] = 1
				d := candidateInventory(t, c, inv, 2)
				require.NotNil(t, d.Decision.Work)
				assert.Equal(t, next, d.Decision.Work.Config)
			case "trust withdrawal":
				inv.Version++
				inv.Candidates[0].Config = trustedCandidateConfig("higher", "literal", false)
				inv.Candidates[0].Incarnation = 10
				d := candidateInventory(t, c, inv, 2)
				if d.Decision.Work != nil {
					candidateResult(t, c, d.Decision.Work, candidateOutcome{Kind: candidateRejected}, 2)
				}
				assert.False(t, candidateAuthorized(c.snapshot(inv.FullName), key))
			}
			assert.Nil(t, c.snapshot(inv.FullName).DeferredSource)
			stale, err := c.prepareWake(inv.FullName, old.Token, old.Due)
			require.True(t, applyCandidateDelta(t, stale, err).Decision.Ignored)
			assert.Nil(t, stale.Decision.Work)
		})
	}
}

func TestDeferredSourceResumesAfterBlockedRetry(t *testing.T) {
	c, inv, predecessor, pending, wall := deferredSourceFixture(t, candidateTestConfig("higher", "higher"), false)
	blocked := candidateResult(t, c, predecessor, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, Listed: true, Retry: candidateRetryPolicy{cadence: 120}}, 2)
	require.NotNil(t, blocked.Decision.Wake)
	assert.EqualValues(t, 122, blocked.Decision.Wake.Due)
	*wall = wall.Add(2 * time.Minute)
	resumed, err := c.prepareWake(inv.FullName, blocked.Decision.Wake.Token, 122)
	applyCandidateDelta(t, resumed, err)
	require.NotNil(t, resumed.Decision.Work)
	assert.Equal(t, predecessor.Token.Candidate, resumed.Decision.Work.Token.Candidate)
	require.NotNil(t, resumed.Decision.Wake, "leaving blocked phase must rearm expired source selection")
	source, err := c.prepareWake(inv.FullName, resumed.Decision.Wake.Token, resumed.Decision.Wake.Due)
	applyCandidateDelta(t, source, err)
	require.NotNil(t, source.Decision.Work)
	assert.Equal(t, pending, source.Decision.Work.Token.Candidate)
}

func TestDeferredSourceDoesNotBlockEligibleSibling(t *testing.T) {
	c := newCandidateController(1)
	wall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.wallNow = func() time.Time { return wall }
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"), candidateTestConfig("b", "b"), candidateTestConfig("c", "c"))
	first := candidateInventory(t, c, inv, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true}, 0).Decision.Work
	third := candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{notBefore: wall.Add(time.Minute)}}, 0).Decision.Work
	candidateResult(t, c, third, candidateOutcome{Kind: candidateFailed, Listed: true, Retry: candidateRetryPolicy{cadence: 1}}, 0)
	inv.Version = 2
	inv.Candidates = inv.Candidates[1:]
	delete(inv.Sources, first.Config.Source())
	d := candidateInventory(t, c, inv, 1)
	require.NotNil(t, d.Decision.Work)
	assert.Equal(t, third.Token.Candidate, d.Decision.Work.Token.Candidate)
	require.NotNil(t, c.snapshot(inv.FullName).DeferredSource)
	require.NotNil(t, d.Decision.Wake)
	candidateResult(t, c, d.Decision.Work, candidateOutcome{Kind: candidateAdmitted}, 1)
	ready, err := c.prepareReady(inv.FullName, d.Decision.Work.Token)
	applyCandidateDelta(t, ready, err)
	assert.Nil(t, c.snapshot(inv.FullName).DeferredSource, "observed healthy sibling is sticky at the same priority")
	assert.Nil(t, c.snapshot(inv.FullName).Wake)
	assert.Equal(t, wall.Add(time.Minute), c.snapshot(inv.FullName).entry(second.Token.Candidate).NotBefore)
}

func TestProviderMinimumManualResetBoundaries(t *testing.T) {
	for _, command := range []string{"enable idempotent", "fresh enable", "exact selected"} {
		t.Run(command, func(t *testing.T) {
			c, inv, predecessor, key, _ := deferredSourceFixture(t, candidateTestConfig("higher", "higher"), false)
			deadline := c.snapshot(inv.FullName).entry(key).NotBefore
			switch command {
			case "enable idempotent":
				d, err := c.prepareEnable(inv.FullName, 2)
				applyCandidateDelta(t, d, err)
				assert.True(t, d.Decision.Ignored)
				assert.Equal(t, deadline, c.snapshot(inv.FullName).entry(key).NotBefore)
				require.NotNil(t, c.snapshot(inv.FullName).DeferredSource)
			case "fresh enable":
				d, err := c.prepareDisable(inv.FullName)
				applyCandidateDelta(t, d, err)
				enabled, err := c.prepareEnable(inv.FullName, 2)
				applyCandidateDelta(t, enabled, err)
				assert.True(t, c.snapshot(inv.FullName).entry(key).NotBefore.IsZero())
			case "exact selected":
				d, err := c.prepareExact(predecessor.Config, true, 2)
				applyCandidateDelta(t, d, err)
				assert.Equal(t, deadline, c.snapshot(inv.FullName).entry(key).NotBefore, "manual reset affects only the selected candidate")
			}
		})
	}
}

func TestDeferredSourceUnchangedDeadlineKeepsWake(t *testing.T) {
	c, inv, _, _, wall := deferredSourceFixture(t, candidateTestConfig("higher", "higher"), false)
	before := c.snapshot(inv.FullName).Wake
	*wall = wall.Add(time.Second)
	inv.Version++
	inv.Candidates[2].Config = candidateTestConfig("other", "changed").SetSourceType(confgroup.TypeStock)
	inv.Candidates[2].Incarnation = 4
	candidateInventory(t, c, inv, 2)
	assert.Equal(t, before, c.snapshot(inv.FullName).Wake, "unchanged estimate retains its capability")
}
