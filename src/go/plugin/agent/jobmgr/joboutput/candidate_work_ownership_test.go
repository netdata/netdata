// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireCandidateWorkOwnership(t *testing.T, c *candidateController) *candidateRecord {
	t.Helper()
	r := c.snapshot("test_job")
	require.NotNil(t, r)
	for _, work := range []*candidateWork{r.Active, r.Proposal} {
		if work != nil {
			require.True(t, c.valid("test_job", work.Token), "retained work must retain exact authority")
		}
	}
	view := c.view("test_job")
	require.Equal(t, r.Active, view.Active)
	require.Equal(t, r.Proposal, view.Proposal)
	if r.Active != nil && r.Proposal != nil {
		require.NotEqual(
			t,
			r.Active.Token.Candidate,
			r.Proposal.Token.Candidate,
			"one membership cannot own both work roles",
		)
	}
	return r
}

func TestCandidateWorkOwnershipEnableDuringSourceProposal(t *testing.T) {
	c := newCandidateController(1)
	discovered := candidateTestConfig("discovered", "retry").SetSourceType(confgroup.TypeDiscovered)
	inv := candidateTestInventory(1, true, discovered)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, first, candidateOutcome{
		Kind:       candidateFailed,
		Listed:     true,
		RetryAfter: 10,
	}, 0)
	user := candidateTestConfig("user", "invalid-payload")
	inv.Version++
	inv.Sources[user.Source()] = 1
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{
		Config:      user,
		Source:      user.Source(),
		Incarnation: 2,
	})
	proposed := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposed)
	require.True(t, proposed.Tentative)
	active := candidateEnable(t, c, 2).Decision.Work
	require.NotNil(t, active)
	require.False(t, active.Tentative)
	require.Equal(t, proposed.Token.Candidate, active.Token.Candidate)
	requireCandidateWorkOwnership(t, c)

	stock := candidateTestConfig("stock", "masked").SetSourceType(confgroup.TypeStock)
	inv.Version++
	inv.Sources[stock.Source()] = 1
	inv.Candidates = append(
		inv.Candidates,
		DiscoveredJobCandidate{
			Config:      stock,
			Source:      stock.Source(),
			Incarnation: 3,
		},
	)
	observed := candidateInventory(t, c, inv, 3)
	require.Nil(t, observed.Decision.Work, "unrelated inventory cannot duplicate the selected Active")
	after := requireCandidateWorkOwnership(t, c)
	require.Equal(t, active, after.Active)
	require.Nil(t, after.Proposal)
	require.False(t, observed.Decision.Revoke)
	coalesced := candidateEnable(t, c, 3)
	require.True(t, coalesced.Decision.Ignored)
	require.Equal(t, active, c.snapshot("test_job").Active)

	rejected := candidateResult(t, c, active, candidateOutcome{
		Kind: candidateRejected,
	}, 4)
	require.NotNil(t, rejected.Decision.Work, "rejection must immediately recover the enabled predecessor")
	require.Equal(t, first.Token.Candidate, rejected.Decision.Work.Token.Candidate)
	after = requireCandidateWorkOwnership(t, c)
	require.True(t, after.Entries[active.Token.Candidate].Rejected)
	require.Equal(t, first.Token.Candidate, after.Binding.Key)
	require.Equal(t, dyncfg.StatusAccepted, after.Binding.Health)
	require.False(t, c.valid("test_job", active.Token))
	stale := candidateResult(t, c, active, candidateOutcome{
		Kind: candidateAdmitted,
	}, 5)
	require.True(t, stale.Decision.Ignored)
	failed := candidateResult(
		t,
		c,
		rejected.Decision.Work,
		candidateOutcome{
			Kind:       candidateFailed,
			Listed:     true,
			RetryAfter: 10,
		},
		5,
	)
	require.NotNil(t, failed.Decision.Wake)
	require.Equal(t, int64(15), failed.Decision.Wake.Due)
	fresh := candidateEnable(t, c, 6)
	require.False(t, fresh.Decision.Ignored)
	require.NotNil(t, fresh.Decision.Work)
	require.Equal(t, first.Token.Candidate, fresh.Decision.Work.Token.Candidate)
	requireCandidateWorkOwnership(t, c)
}

func TestCandidateWorkOwnershipSiblingSurvivesPredecessorRemoval(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "first"), candidateTestConfig("b", "invalid-payload")
	inv := candidateTestInventory(1, true, a, b)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	require.Equal(t, a, first.Config)
	sibling := candidateResult(
		t,
		c,
		first,
		candidateOutcome{
			Kind:       candidateFailed,
			Listed:     true,
			RetryAfter: 10,
		},
		1,
	).Decision.Work
	require.NotNil(t, sibling)
	require.Equal(t, b, sibling.Config)
	inv.Version++
	inv.Candidates = inv.Candidates[1:]
	delete(inv.Sources, a.Source())
	removed := candidateInventory(t, c, inv, 2)
	require.Nil(t, removed.Decision.Work, "removal cannot duplicate an already-selected sibling")
	require.False(t, removed.Decision.Revoke, "valid sibling still owns physical execution")
	after := requireCandidateWorkOwnership(t, c)
	require.Equal(t, sibling, after.Active)
	require.Nil(t, after.Binding)
	candidateResult(t, c, sibling, candidateOutcome{
		Kind: candidateRejected,
	}, 3)
	requireCandidateWorkOwnership(t, c)
	third := candidateTestConfig("c", "valid")
	inv.Version++
	inv.Sources[third.Source()] = 1
	inv.Candidates = append(
		inv.Candidates,
		DiscoveredJobCandidate{
			Config:      third,
			Source:      third.Source(),
			Incarnation: 3,
		},
	)
	added := candidateInventory(t, c, inv, 4)
	require.NotNil(t, added.Decision.Work)
	require.Equal(t, third, added.Decision.Work.Config)
	requireCandidateWorkOwnership(t, c)
}

func TestCandidateWorkOwnershipPreservesDistinctRoles(t *testing.T) {
	for _, transition := range []string{"reject proposal", "remove proposal", "remove active"} {
		t.Run(transition, func(t *testing.T) {
			c := newCandidateController(1)
			discovered := candidateTestConfig("discovered", "incumbent").SetSourceType(confgroup.TypeDiscovered)
			inv := candidateTestInventory(1, true, discovered)
			active := candidateInventory(t, c, inv, 0).Decision.Work
			candidateResult(t, c, active, candidateOutcome{
				Kind: candidateAdmitted,
			}, 0)
			active = c.snapshot("test_job").Active
			user := candidateTestConfig("user", "successor")
			inv.Version++
			inv.Sources[user.Source()] = 1
			inv.Candidates = append(
				inv.Candidates,
				DiscoveredJobCandidate{
					Config:      user,
					Source:      user.Source(),
					Incarnation: 2,
				},
			)
			proposal := candidateInventory(t, c, inv, 1).Decision.Work
			require.NotNil(t, proposal)
			before := requireCandidateWorkOwnership(t, c)
			require.Equal(t, active, before.Active)
			require.Equal(t, proposal, before.Proposal)
			var delta *candidateDelta
			switch transition {
			case "reject proposal":
				delta = candidateResult(t, c, proposal, candidateOutcome{
					Kind: candidateRejected,
				}, 2)
			case "remove proposal":
				inv.Version++
				inv.Candidates = inv.Candidates[:1]
				delete(inv.Sources, user.Source())
				delta = candidateInventory(t, c, inv, 2)
			case "remove active":
				inv.Version++
				inv.Candidates = inv.Candidates[1:]
				delete(inv.Sources, discovered.Source())
				delta = candidateInventory(t, c, inv, 2)
			}
			after := requireCandidateWorkOwnership(t, c)
			require.Nil(t, delta.Decision.Work)
			if transition == "remove active" {
				assert.Nil(t, after.Active)
				assert.Equal(t, proposal, after.Proposal)
				assert.False(t, delta.Decision.Revoke, "valid Proposal owns the current executor")
			} else {
				assert.Equal(t, active, after.Active)
				assert.Nil(t, after.Proposal)
				assert.Equal(t, before.Binding, after.Binding)
			}
		})
	}
}

func TestCandidateWorkOwnershipScopesResumeToItsRole(t *testing.T) {
	for _, role := range []string{"proposal", "queued predecessor"} {
		t.Run(role, func(t *testing.T) {
			c := newCandidateController(1)
			old := candidateTestConfig("source", "old")
			inv := candidateTestInventory(1, true, old)
			first := candidateInventory(t, c, inv, 0).Decision.Work
			candidateResult(t, c, first, candidateOutcome{
				Kind:   candidateFrameworkBlocked,
				Block:  candidateBlockAcceptedWait,
				Listed: true,
			}, 0)
			blocked := requireCandidateWorkOwnership(t, c)
			require.True(t, blocked.FrameworkBlocked)
			next := candidateTestConfig("source", "replacement")
			inv.Version++
			inv.Candidates[0] = DiscoveredJobCandidate{
				Config:      next,
				Source:      next.Source(),
				Incarnation: 2,
			}
			proposal := candidateInventory(t, c, inv, 1).Decision.Work
			require.NotNil(t, proposal)
			candidateResult(t, c, proposal, candidateOutcome{
				Kind: candidateWaiting,
			}, 1)
			before := requireCandidateWorkOwnership(t, c)
			if role == "queued predecessor" {
				delta, err := c.prepareResume("test_job", first.Token, 2)
				applyCandidateDelta(t, delta, err)
				require.True(t, delta.Decision.Ignored)
				require.Nil(t, delta.Decision.Work, "queued predecessor cannot displace the proposal executor")
				require.Equal(t, before, requireCandidateWorkOwnership(t, c))
				return
			}
			delta, err := c.prepareResume("test_job", proposal.Token, 2)
			applyCandidateDelta(t, delta, err)
			after := requireCandidateWorkOwnership(t, c)
			require.True(t, after.FrameworkBlocked, "predecessor retains its own framework wait")
			require.Equal(t, before.Binding, after.Binding, "tentative resume is not admission")
			require.Equal(t, before.Active, after.Active)
			require.Nil(t, delta.Decision.Binding)
			require.NotNil(t, delta.Decision.Work)
			require.True(t, delta.Decision.Work.Tentative)
			require.NotEqual(t, proposal.Token.Attempt, delta.Decision.Work.Token.Attempt)
			candidateResult(t, c, delta.Decision.Work, candidateOutcome{
				Kind: candidateRejected,
			}, 3)
			restored, err := c.prepareResume("test_job", first.Token, 4)
			applyCandidateDelta(t, restored, err)
			after = requireCandidateWorkOwnership(t, c)
			require.False(t, after.FrameworkBlocked)
			require.Nil(t, after.Proposal)
			require.Equal(t, first.Token.Candidate, restored.Decision.Work.Token.Candidate)
		})
	}
}
