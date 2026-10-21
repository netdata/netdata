// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recoveryInventory(t *testing.T, c *candidateController, inv DiscoveredJobInventory, now int64, invalid ...DiscoveredJobCandidate) *candidateDelta {
	t.Helper()
	quarantined := make(map[candidateKey]bool, len(invalid))
	for _, candidate := range invalid {
		quarantined[candidateIdentity(candidate)] = true
	}
	delta, err := c.prepareInventoryValidated(inv, quarantined, now)
	return applyCandidateDelta(t, delta, err)
}

func TestCandidateRecoveryPendingProposalUsesPredecessorScope(t *testing.T) {
	for name, exact := range map[string]bool{"source alternatives": false, "exact source restart": true} {
		t.Run(name, func(t *testing.T) {
			c, inv, old, current := candidateHistoricalGrant(t, true)
			if exact {
				pinned, err := c.prepareExact(current.Config, true, 3)
				applyCandidateDelta(t, pinned, err)
				candidateResult(t, c, pinned.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 100}, 3)
			}
			user := DiscoveredJobCandidate{Config: candidateTestConfig("user", "pending"), Source: "user", Incarnation: 8}
			inv.Version = 3
			inv.Candidates = append(inv.Candidates, user)
			inv.Sources["user"] = 1
			proposal := candidateInventory(t, c, inv, 4).Decision.Work
			require.NotNil(t, proposal)
			require.Equal(t, user.Config, proposal.Config)
			before := c.snapshot(inv.FullName)
			inv.Version = 4
			inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false).SetDiscoveryPipelineID("pipeline-a"), Source: "a", Incarnation: 9}
			withdrawal := recoveryInventory(t, c, inv, 5, inv.Candidates[0])
			after := c.snapshot(inv.FullName)
			assert.Equal(t, before.Binding, after.Binding)
			assert.Equal(t, before.Proposal, after.Proposal)
			assert.False(t, withdrawal.Decision.Revoke)
			assert.False(t, candidateAuthorized(after, old.Token.Candidate))
			require.NotNil(t, after.Wake, "the pending higher tier does not own the predecessor's recovery")
			assert.Equal(t, int64(103), after.Wake.Due)
			if exact {
				assert.Equal(t, before.Wake, after.Wake, "unaffected exact recovery retains its schedule capability")
			} else {
				assert.NotEqual(t, before.Wake.Token, after.Wake.Token, "the revoked earliest candidate requires a new deadline")
			}

			// The heap consumes this capability once. Repeated inventory changes
			// cannot rearm a due predecessor while its proposal still blocks work.
			wake, err := c.prepareWake(inv.FullName, after.Wake.Token, 103)
			applyCandidateDelta(t, wake, err)
			assert.Nil(t, wake.Decision.Work)
			assert.Nil(t, wake.Decision.Wake)
			inv.Version = 5
			stock := candidateTestConfig("stock", "irrelevant").SetSourceType(confgroup.TypeStock)
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: stock, Source: "stock", Incarnation: 10})
			inv.Sources["stock"] = 1
			repeated := recoveryInventory(t, c, inv, 104, inv.Candidates[0])
			assert.Nil(t, repeated.Decision.Work)
			assert.Nil(t, repeated.Decision.Wake)
			resumed := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateRejected}, 105).Decision.Work
			require.NotNil(t, resumed)
			assert.Equal(t, current.Token.Candidate, resumed.Token.Candidate, "rejection resumes the already-due predecessor")
		})
	}
}

func TestCandidateRecoveryWithdrawalKeepsFinitePassOrder(t *testing.T) {
	for name, revokeActive := range map[string]bool{"historical grant": false, "active membership": true} {
		t.Run(name, func(t *testing.T) {
			c, inv, old, current := candidateHistoricalGrant(t, true)
			pipeline := "new-target"
			if revokeActive {
				pipeline = "pipeline-a"
			}
			initial := trustedCandidateConfig("initial", "initial", true).SetDiscoveryPipelineID(pipeline)
			inv.Version = 3
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: initial, Source: "initial", Incarnation: 8})
			inv.Sources["initial"] = 1
			work := candidateInventory(t, c, inv, 103).Decision.Work
			require.NotNil(t, work)
			require.Equal(t, initial, work.Config)
			// The due current candidate is frozen into the pass behind this initial
			// work. A later arrival must wait even if another member is revoked.
			later := trustedCandidateConfig("later", "later", true).SetDiscoveryPipelineID("later-target")
			inv.Version = 4
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: later, Source: "later", Incarnation: 9})
			inv.Sources["later"] = 1
			inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false).SetDiscoveryPipelineID("pipeline-a"), Source: "a", Incarnation: 10}
			changed := recoveryInventory(t, c, inv, 104, inv.Candidates[0])
			assert.False(t, candidateAuthorized(c.snapshot(inv.FullName), old.Token.Candidate))
			next := changed.Decision.Work
			if !revokeActive {
				assert.Nil(t, next)
				next = candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true}, 104).Decision.Work
			} else {
				assert.False(t, c.valid(inv.FullName, work.Token))
			}
			require.NotNil(t, next)
			assert.Equal(t, current.Token.Candidate, next.Token.Candidate, "the surviving finite pass precedes new arrivals")
			remaining := candidateResult(t, c, next, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 100}, 105).Decision.Work
			require.NotNil(t, remaining)
			assert.Equal(t, later, remaining.Config)
		})
	}
}

func TestCandidateRecoveryWithdrawalCancelsCurrentDeadline(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, trustedCandidateConfig("a", "trusted", true))
	old := candidateInventory(t, c, inv, 0).Decision.Work
	failure := candidateResult(t, c, old, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 1)
	require.NotNil(t, failure.Decision.Wake)
	inv.Version = 2
	inv.Candidates[0] = DiscoveredJobCandidate{Config: trustedCandidateConfig("a", "invalid", false), Source: "a", Incarnation: 2}
	changed := recoveryInventory(t, c, inv, 2, inv.Candidates[0])
	assert.Nil(t, changed.Decision.Work)
	assert.Nil(t, changed.Decision.Wake)
	assert.Nil(t, c.snapshot(inv.FullName).Wake)
	assert.Equal(t, candidateManualSourceRevoked, c.snapshot(inv.FullName).Manual)
	stale, err := c.prepareWake(inv.FullName, failure.Decision.Wake.Token, 11)
	assert.True(t, applyCandidateDelta(t, stale, err).Decision.Ignored)
}
