// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trustedCandidateConfig(source, value string, trusted bool) confgroup.Config {
	return candidateTestConfig(source, value).SetSourceType(confgroup.TypeDiscovered).
		SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(trusted)
}

func applyQuarantinedInventory(t *testing.T, c *candidateController, inv DiscoveredJobInventory, invalid ...DiscoveredJobCandidate) *candidateDelta {
	t.Helper()
	quarantined := make(map[candidateKey]bool, len(invalid))
	for _, candidate := range invalid {
		quarantined[candidateIdentity(candidate)] = true
	}
	delta, err := c.prepareInventoryValidated(inv, quarantined, 1)
	return applyCandidateDelta(t, delta, err)
}

func TestCandidateTrustWithdrawalFencesObservedMembership(t *testing.T) {
	c := newCandidateController(1)
	trusted := trustedCandidateConfig("a", "trusted", true)
	inv := candidateTestInventory(1, true, trusted)
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
	ready, err := c.prepareReady(inv.FullName, old.Token)
	applyCandidateDelta(t, ready, err)

	// Another target of the same pipeline withdraws permission. The old trusted
	// membership remains observed, so deleting only retained grants is insufficient.
	withdrawal := DiscoveredJobCandidate{Config: trustedCandidateConfig("b", "invalid", false), Source: "b", Incarnation: 2}
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, withdrawal)
	inv.Sources["b"] = 1
	revoked := applyQuarantinedInventory(t, c, inv, withdrawal)
	assert.True(t, revoked.Decision.Remove)
	assert.True(t, revoked.Decision.Revoke)
	assert.Nil(t, revoked.Decision.Binding)
	assert.Nil(t, revoked.Decision.Work)
	assert.Nil(t, revoked.Decision.Wake)
	assert.False(t, candidateAuthorized(c.snapshot(inv.FullName), old.Token.Candidate))
	assert.False(t, c.valid(inv.FullName, old.Token))

	// An unchanged member repeated by a later full snapshot cannot regain authority.
	inv.Version = 3
	stock := candidateTestConfig("stock", "lower tier").SetSourceType(confgroup.TypeStock)
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: stock, Source: "stock", Incarnation: 3})
	inv.Sources["stock"] = 1
	repeated := applyQuarantinedInventory(t, c, inv, withdrawal)
	assert.Nil(t, repeated.Decision.Work, "withdrawal is not permission to open a lower source tier")
	assert.False(t, candidateAuthorized(c.snapshot(inv.FullName), old.Token.Candidate))
	assert.Nil(t, c.snapshot(inv.FullName).Binding)
	for _, outcome := range []candidateOutcome{{Kind: candidateAdmitted}, {Kind: candidateFailed, Listed: true, RetryAfter: 10}} {
		assert.True(t, candidateResult(t, c, old, outcome, 2).Decision.Ignored)
	}
	lateReady, err := c.prepareReady(inv.FullName, old.Token)
	assert.True(t, applyCandidateDelta(t, lateReady, err).Decision.Ignored)
	resume, err := c.prepareResume(inv.FullName, old.Token, 2)
	assert.True(t, applyCandidateDelta(t, resume, err).Decision.Ignored)
	_, err = c.prepareExact(trusted, true, 2)
	require.Error(t, err, "an exact operation cannot readmit an observed revoked membership")

	// A real source removal can select the lower tier through ordinary arbitration.
	inv.Version = 4
	inv.Candidates = inv.Candidates[2:]
	delete(inv.Sources, "a")
	delete(inv.Sources, "b")
	removed := candidateInventory(t, c, inv, 3)
	require.NotNil(t, removed.Decision.Work)
	assert.Equal(t, stock, removed.Decision.Work.Config)
	assert.NotContains(t, c.snapshot(inv.FullName).Entries, old.Token.Candidate)
}

func TestCandidateTrustWithdrawalDoesNotReplayAgainstNewPermission(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "old", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
	withdrawal := DiscoveredJobCandidate{Config: trustedCandidateConfig("b", "invalid", false), Source: "b", Incarnation: 2}
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, withdrawal)
	inv.Sources["b"] = 1
	applyQuarantinedInventory(t, c, inv, withdrawal)

	// A fresh valid member in the protected source slot can be selected normally.
	// The other target's old opt-out remains in every complete snapshot.
	inv.Version = 3
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "new", true), Source: "a", Incarnation: 3}
	fresh := applyQuarantinedInventory(t, c, inv, withdrawal).Decision.Work
	require.NotNil(t, fresh)
	assert.Equal(t, inv.Candidates[0].Config, fresh.Config)
	candidateResult(t, c, fresh, candidateOutcome{Kind: candidateAdmitted}, 2)
	inv.Version = 4
	unchanged := applyQuarantinedInventory(t, c, inv, withdrawal)
	assert.False(t, unchanged.Decision.Revoke)
	assert.False(t, unchanged.Decision.Remove)
	assert.True(t, c.valid(inv.FullName, fresh.Token))
	assert.True(t, candidateAuthorized(c.snapshot(inv.FullName), fresh.Token.Candidate))

	// A genuine new false incarnation is a new withdrawal, including re-addition.
	inv.Version = 5
	inv.Candidates[1].Incarnation = 4
	applyQuarantinedInventory(t, c, inv, inv.Candidates[1])
	assert.False(t, c.valid(inv.FullName, fresh.Token))
	assert.Nil(t, c.snapshot(inv.FullName).Binding)
}

func TestCandidateTrustWithdrawalPreservesUnrelatedProposal(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
	user := DiscoveredJobCandidate{Config: candidateTestConfig("user", "replacement"), Source: "user", Incarnation: 2}
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, user)
	inv.Sources["user"] = 1
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposal)
	require.Equal(t, user.Config, proposal.Config)

	inv.Version = 3
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 3}
	revoked := applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
	assert.True(t, revoked.Decision.Remove)
	assert.False(t, revoked.Decision.Revoke, "a retained successor must keep its executor")
	assert.Equal(t, proposal, c.snapshot(inv.FullName).Proposal)
	assert.True(t, c.valid(inv.FullName, proposal.Token))
	assert.False(t, c.valid(inv.FullName, old.Token))
	accepted := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateAdmitted}, 2)
	assert.Equal(t, user.Config, accepted.Decision.Binding.Config)
	assert.Equal(t, candidateManualNone, c.snapshot(inv.FullName).Manual)
}

func TestCandidateTrustWithdrawalPreservesUnrelatedRecovery(t *testing.T) {
	for name, blocked := range map[string]bool{"ordinary retry": false, "framework retry": true} {
		t.Run(name, func(t *testing.T) {
			c, inv, old, sibling := candidateHistoricalGrant(t, true)
			if blocked {
				disabled, err := c.prepareDisable(inv.FullName)
				applyCandidateDelta(t, disabled, err)
				work := candidateEnable(t, c, 4).Decision.Work
				require.NotNil(t, work)
				require.Equal(t, sibling.Token.Candidate, work.Token.Candidate)
				candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, RetryAfter: 20, Listed: true}, 4)
			}
			before := c.snapshot(inv.FullName)
			inv.Version = 3
			inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false).SetDiscoveryPipelineID("pipeline-a"), Source: "a", Incarnation: 8}
			changed := applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
			after := c.snapshot(inv.FullName)
			assert.Equal(t, before.Binding, after.Binding)
			assert.Equal(t, before.Entries[sibling.Token.Candidate], after.Entries[sibling.Token.Candidate])
			assert.False(t, candidateAuthorized(after, old.Token.Candidate))
			assert.False(t, changed.Decision.Revoke)
			assert.False(t, changed.Decision.Remove)
			assert.Nil(t, changed.Decision.Binding)
			require.NotNil(t, after.Wake)
			if blocked {
				assert.Equal(t, before.Wake, after.Wake)
				assert.Equal(t, before.Active, after.Active)
				assert.Equal(t, before.Phase, after.Phase)
			} else {
				assert.Equal(t, int64(103), after.Wake.Due, "the surviving candidate keeps its original cooldown")
			}
		})
	}
}

func TestCandidateTrustWithdrawalPreservesUnadmittedSibling(t *testing.T) {
	c := newCandidateController(1)
	sibling := trustedCandidateConfig("b", "sibling", true).SetDiscoveryPipelineID("other")
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "old", true), sibling)
	old := candidateInventory(t, c, inv, 0).Decision.Work
	pending := candidateResult(t, c, old, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0).Decision.Work
	require.NotNil(t, pending)
	require.Equal(t, sibling, pending.Config)
	inv.Version = 2
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 3}
	revoked := applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
	assert.True(t, revoked.Decision.Remove)
	assert.False(t, revoked.Decision.Revoke, "the unadmitted sibling already owns the executor")
	assert.True(t, c.valid(inv.FullName, pending.Token))
	assert.Equal(t, pending, c.snapshot(inv.FullName).Active)
	accepted := candidateResult(t, c, pending, candidateOutcome{Kind: candidateAdmitted}, 2)
	assert.Equal(t, sibling, accepted.Decision.Binding.Config)
	assert.Equal(t, candidateManualNone, c.snapshot(inv.FullName).Manual)
}

func TestCandidateTrustWithdrawalOrderedBatch(t *testing.T) {
	for name, trustedLast := range map[string]bool{"withdraw then grant": true, "grant then withdraw": false} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			initial := candidateTestInventory(1, true, trustedCandidateConfig("a", "old", true))
			old := candidateInventory(t, c, initial, 0).Decision.Work
			candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
			first := candidateTestInventory(2, true, trustedCandidateConfig("a", "first", !trustedLast))
			first.Candidates[0].Incarnation = 2
			last := candidateTestInventory(3, true, trustedCandidateConfig("a", "last", trustedLast))
			last.Candidates[0].Incarnation = 3
			invalid := first.Candidates[0]
			if !trustedLast {
				invalid = last.Candidates[0]
			}
			batch := DiscoveredJobInventoryBatch{FullName: initial.FullName, Observations: []DiscoveredJobInventory{first, last}}
			delta, err := c.prepareInventoryBatchValidated(batch, map[candidateKey]bool{candidateIdentity(invalid): true}, 1)
			require.NoError(t, err)
			assert.True(t, c.valid(initial.FullName, old.Token), "preparation cannot mutate committed authority")
			applyCandidateDelta(t, delta, nil)
			assert.False(t, c.valid(initial.FullName, old.Token))
			if trustedLast {
				require.NotNil(t, delta.Decision.Work)
				assert.Equal(t, last.Candidates[0].Config, delta.Decision.Work.Config)
				candidateResult(t, c, delta.Decision.Work, candidateOutcome{Kind: candidateAdmitted}, 1)
			} else {
				assert.Nil(t, delta.Decision.Work)
				assert.Nil(t, c.snapshot(initial.FullName).Binding)
			}
			replay, err := c.prepareInventoryBatchValidated(batch, nil, 2)
			assert.True(t, applyCandidateDelta(t, replay, err).Decision.Ignored)
		})
	}
}

func TestCandidateTrustWithdrawalAbandonedPreparation(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
	before := c.snapshot(inv.FullName)
	inv.Version = 2
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 2}
	prepared, err := c.prepareInventoryValidated(inv, map[candidateKey]bool{candidateIdentity(inv.Candidates[0]): true}, 1)
	require.NoError(t, err)
	assert.Equal(t, before, c.snapshot(inv.FullName))
	ready, err := c.prepareReady(inv.FullName, old.Token)
	applyCandidateDelta(t, ready, err)
	assert.False(t, prepared.apply(), "an abandoned predecessor cannot revoke a later committed state")
	assert.Equal(t, dyncfg.StatusRunning, c.snapshot(inv.FullName).Binding.Health)
}

func TestCandidateTrustWithdrawalFailureProvenance(t *testing.T) {
	for name, removed := range map[string]bool{"successive false revision": false, "new source lifetime": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
			old := candidateInventory(t, c, inv, 0).Decision.Work
			candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
			inv.Version = 2
			inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid metadata", false), Source: "a", Incarnation: 2}
			applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
			if removed {
				candidateInventory(t, c, DiscoveredJobInventory{FullName: inv.FullName, Version: 3, AutoEnable: true}, 2)
				inv.Sources["a"] = 2
			}
			inv.Version = 4
			inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid literal", false), Source: "a", Incarnation: 3}
			proposal := candidateInventory(t, c, inv, 3).Decision.Work
			require.NotNil(t, proposal)
			failed := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateRejected}, 3)
			if removed {
				assert.Nil(t, failed.Decision.Binding, "a removed source cannot confer old Failed-adoption provenance")
				assert.Nil(t, c.snapshot(inv.FullName).Binding)
			} else {
				require.NotNil(t, failed.Decision.Binding)
				assert.Equal(t, inv.Candidates[0].Config, failed.Decision.Binding.Config)
				assert.Equal(t, dyncfg.StatusFailed, failed.Decision.Binding.Health)
			}
		})
	}
}

func TestCandidateTrustWithdrawalPreservesDynCfgOwnership(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateAdmitted}, 0)
	override := candidateTestConfig("override", "operator").SetSourceType(confgroup.TypeDyncfg)
	exact, err := c.prepareExact(override, true, 0)
	applyCandidateDelta(t, exact, err)
	before := c.snapshot(inv.FullName)
	inv.Version = 2
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 2}
	withdrawn := applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
	after := c.snapshot(inv.FullName)
	assert.Equal(t, before.Binding, after.Binding)
	assert.Equal(t, before.Active, after.Active)
	assert.False(t, withdrawn.Decision.Remove)
	assert.False(t, withdrawn.Decision.Revoke)
	assert.True(t, c.valid(inv.FullName, exact.Decision.Work.Token))
	assert.False(t, candidateAuthorized(after, old.Token.Candidate))
}

func TestCandidateTrustWithdrawalCutsFrameworkWait(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, old, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockQuarantined, Listed: true}, 0)
	inv.Version = 2
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 2}
	applyQuarantinedInventory(t, c, inv, inv.Candidates[0])
	assert.False(t, c.snapshot(inv.FullName).FrameworkBlocked)
	assert.Nil(t, c.snapshot(inv.FullName).Active)
	inv.Version = 3
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "new", true), Source: "a", Incarnation: 3}
	replacement := candidateInventory(t, c, inv, 2).Decision.Work
	require.NotNil(t, replacement, "a revoked framework wait cannot suppress a new valid source membership")
	assert.Equal(t, inv.Candidates[0].Config, replacement.Config)
}
