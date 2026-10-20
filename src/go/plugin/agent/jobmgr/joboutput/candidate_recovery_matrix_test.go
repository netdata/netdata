// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Recovery belongs to the surviving continuation, independently of the source
// choice that masks it or the historical pipeline whose grant was withdrawn.
func TestCandidateRecoveryOwnershipMatrix(t *testing.T) {
	for _, ownership := range []string{"interactive DynCfg", "source DynCfg", "source", "masked source"} {
		for _, state := range []string{"active", "retry", "accepted wait", "quarantine", "framework retry", "disabled", "ready closed"} {
			t.Run(ownership+"/"+state, func(t *testing.T) {
				controller, inventory, old, work := recoveryMatrixContinuation(t, ownership)
				c := controller.candidates
				id := inventory.FullName
				outcome := candidateOutcome{Kind: candidateAdmitted, Listed: true}
				switch state {
				case "retry", "disabled":
					outcome = candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}
				case "accepted wait":
					outcome = candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockAcceptedWait, Listed: true}
				case "quarantine":
					outcome = candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockQuarantined, Listed: true}
				case "framework retry":
					outcome = candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, Listed: true, RetryAfter: 10}
				}
				delta, err := c.prepareOutcome(id, work.Token, outcome, 1)
				recoveryMatrixCommit(t, controller, delta, err)
				if state == "disabled" {
					delta, err = c.prepareDisable(id)
					recoveryMatrixCommit(t, controller, delta, err)
				}
				if state == "ready closed" {
					delta, err = c.prepareReady(id, work.Token)
					recoveryMatrixCommit(t, controller, delta, err)
					delta, err = c.prepareOutcome(id, work.Token, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10, AfterReady: true}, 1)
					recoveryMatrixCommit(t, controller, delta, err)
				}
				before := c.snapshot(id)
				require.NotNil(t, before.Binding)
				require.True(t, candidateAuthorized(before, old.Token.Candidate), "P must hold authority before withdrawal")
				require.Equal(t, ownership != "interactive DynCfg", before.Entries[before.Binding.Key].SourceOwned)
				if state == "retry" || state == "framework retry" {
					require.NotNil(t, before.Wake)
					require.Equal(t, int64(11), before.Wake.Due)
				}
				recoveryMatrixDispatcher(t, controller, id, before.Wake)

				inventory.Version++
				inventory.Candidates[0] = recoveryMatrixWithdrawal("a", true)
				require.Equal(t, id, inventory.Candidates[0].Config.FullName())
				require.Error(t, controller.validateCandidateMetadata(inventory.Candidates[0].Config))
				recoveryMatrixInventory(t, controller, inventory)
				after := c.snapshot(id)
				assert.Equal(t, before.Binding, after.Binding)
				assert.Equal(t, before.Active, after.Active, "unaffected work capability must survive")
				assert.Equal(t, before.Proposal, after.Proposal)
				assert.Equal(t, before.Phase, after.Phase)
				assert.Equal(t, before.Scope, after.Scope)
				assert.Equal(t, before.Pass, after.Pass, "withdrawal must not restart the finite pass")
				assert.Equal(t, before.PassCursor, after.PassCursor)
				assert.Equal(t, before.Wake, after.Wake, "unchanged deadline retains its wake capability")
				assert.Equal(t, before.Entries[work.Token.Candidate], after.Entries[work.Token.Candidate])
				assert.False(t, candidateAuthorized(after, old.Token.Candidate))
				assert.False(t, c.valid(id, old.Token))
				recoveryMatrixDispatcher(t, controller, id, before.Wake)

				// Exercise the surviving capability, rather than merely retaining a
				// timer-shaped policy value with no executable continuation.
				switch state {
				case "retry", "framework retry":
					delta, err = c.prepareWake(id, before.Wake.Token, before.Wake.Due)
					applyCandidateDelta(t, delta, err)
					require.NotNil(t, delta.Decision.Work)
					assert.Equal(t, work.Token.Candidate, delta.Decision.Work.Token.Candidate)
				case "accepted wait":
					delta, err = c.prepareResume(id, work.Token, 2)
					applyCandidateDelta(t, delta, err)
					require.NotNil(t, delta.Decision.Work)
					assert.Equal(t, work.Token.Candidate, delta.Decision.Work.Token.Candidate)
				case "quarantine":
					delta, err = c.prepareResume(id, work.Token, 2)
					assert.True(t, applyCandidateDelta(t, delta, err).Decision.Ignored)
				case "active":
					assert.True(t, c.valid(id, work.Token))
				case "disabled", "ready closed":
					assert.False(t, c.valid(id, work.Token))
				}
			})
		}
	}
}

func TestCandidateRecoveryExactValidWithdrawalAndPassiveControl(t *testing.T) {
	for _, passive := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid withdrawal retry", true: "passive"}[passive], func(t *testing.T) {
			controller, inventory, old, work := recoveryMatrixContinuation(t, "interactive DynCfg")
			c := controller.candidates
			var delta *candidateDelta
			var err error
			if passive {
				delta, err = c.prepareExact(work.Config, false, 1)
			} else {
				delta, err = c.prepareOutcome(inventory.FullName, work.Token, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 1)
			}
			recoveryMatrixCommit(t, controller, delta, err)
			before := c.snapshot(inventory.FullName)
			if passive {
				require.Equal(t, candidatePassive, before.Enablement)
				require.Nil(t, before.Wake)
			} else {
				require.NotNil(t, before.Wake)
			}
			inventory.Version++
			inventory.Candidates[0] = recoveryMatrixWithdrawal("a", false)
			require.NoError(t, controller.validateCandidateMetadata(inventory.Candidates[0].Config))
			recoveryMatrixInventory(t, controller, inventory)
			after := c.snapshot(inventory.FullName)
			assert.Equal(t, before.Binding, after.Binding)
			assert.Equal(t, before.Enablement, after.Enablement)
			assert.Equal(t, before.Wake, after.Wake)
			assert.Nil(t, after.Active)
			assert.False(t, candidateAuthorized(after, old.Token.Candidate))
			recoveryMatrixDispatcher(t, controller, inventory.FullName, before.Wake)
		})
	}
}

func recoveryMatrixContinuation(t *testing.T, ownership string) (*DynCfgJobController, DiscoveredJobInventory, *candidateWork, *candidateWork) {
	t.Helper()
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	c := controller.candidates
	p := confgroup.Config{"module": "module", "name": "job", "option_str": "trusted"}.
		SetSource("a").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
		SetDiscoveryPipelineID("pipeline-p").SetTrustDiscoveredTargets(true)
	inv := DiscoveredJobInventory{FullName: p.FullName(), Version: 1, AutoEnable: true,
		Sources: map[string]uint64{"a": 1}, Candidates: []DiscoveredJobCandidate{{Config: p, Source: "a", Incarnation: 1}}}
	delta, err := c.prepareInventory(inv, 0)
	old := applyCandidateDelta(t, delta, err).Decision.Work
	require.NotNil(t, old)
	delta, err = c.prepareOutcome(inv.FullName, old.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, 0)
	applyCandidateDelta(t, delta, err)
	q := confgroup.Config{"module": "module", "name": "job", "option_str": "survivor"}.
		SetSource("b").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
		SetDiscoveryPipelineID("pipeline-q").SetTrustDiscoveredTargets(true)
	var work *candidateWork
	if ownership == "interactive DynCfg" {
		q.SetSourceType(confgroup.TypeDyncfg)
		delta, err = c.prepareExact(q, true, 0)
		work = applyCandidateDelta(t, delta, err).Decision.Work
	} else {
		if ownership == "source DynCfg" {
			q.SetSourceType(confgroup.TypeDyncfg)
		}
		inv.Version++
		inv.Sources["b"] = 1
		inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: q, Source: "b", Incarnation: 2})
		delta, err = c.prepareInventory(inv, 0)
		work = applyCandidateDelta(t, delta, err).Decision.Work
		if ownership != "source DynCfg" {
			require.Nil(t, work, "same-tier observation keeps its active incumbent")
			delta, err = c.prepareOutcome(inv.FullName, old.Token, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 100}, 0)
			work = applyCandidateDelta(t, delta, err).Decision.Work
		}
	}
	require.NotNil(t, work)
	require.Equal(t, q, work.Config)
	delta, err = c.prepareOutcome(inv.FullName, work.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, 0)
	applyCandidateDelta(t, delta, err)
	if ownership == "masked source" {
		// Observe a higher proposal, then quarantine its replacement. The
		// consumed source choice retains its historical priority over Q.
		high := recoveryMatrixWithdrawal("higher", false)
		high.Config.SetSourceType(confgroup.TypeUser).SetDiscoveryPipelineID("")
		inv.Version++
		inv.Sources["higher"] = 1
		inv.Candidates = append(inv.Candidates, high)
		recoveryMatrixInventory(t, controller, inv)
		high = recoveryMatrixWithdrawal("higher", true)
		high.Config.SetSourceType(confgroup.TypeUser).SetDiscoveryPipelineID("")
		high.Incarnation++
		require.Equal(t, inv.FullName, high.Config.FullName())
		require.Error(t, controller.validateCandidateMetadata(high.Config))
		inv.Version++
		inv.Candidates[len(inv.Candidates)-1] = high
		recoveryMatrixInventory(t, controller, inv)
		before := c.snapshot(inv.FullName)
		require.Equal(t, q, before.Binding.Config)
		choice := candidateSourceChoice(before)
		require.NotNil(t, choice)
		require.Greater(t, choice.Observation.Priority, q.SourceTypePriority())
		require.Nil(t, choice.Member)
	}
	return controller, inv, old, work
}

func recoveryMatrixWithdrawal(source string, invalid bool) DiscoveredJobCandidate {
	config := confgroup.Config{"module": "module", "name": "job", "option_str": "literal"}
	if invalid {
		config = confgroup.Config{"module": "module_job", "name": "module_job", "option_str": "literal"}
	}
	config.SetSource(source).SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
		SetDiscoveryPipelineID("pipeline-p").SetTrustDiscoveredTargets(false)
	return DiscoveredJobCandidate{Config: config, Source: source, Incarnation: 50}
}

func recoveryMatrixInventory(t *testing.T, controller *DynCfgJobController, inventory DiscoveredJobInventory) {
	t.Helper()
	plan, err := planInventoryTest(controller, inventory)
	require.NoError(t, err)
	tx, err := plan.Transaction.Prepare(t.Context(), nil, lifecycle.ResourceTransactionScope{ID: inventory.FullName}, lifecycle.LongLivedPermit{})
	require.NoError(t, err)
	_, err = tx.Apply(t.Context())
	require.NoError(t, err)
}

func recoveryMatrixCommit(t *testing.T, controller *DynCfgJobController, delta *candidateDelta, err error) {
	t.Helper()
	require.NoError(t, err)
	commit, err := controller.prepareCandidateCommit(delta, false)
	require.NoError(t, err)
	commit()
}

func recoveryMatrixDispatcher(t *testing.T, controller *DynCfgJobController, id string, expected *candidateWake) {
	t.Helper()
	index := controller.scheduler.retries
	index.mu.Lock()
	defer index.mu.Unlock()
	actual := index.entries[id]
	if expected == nil {
		assert.Nil(t, actual, "no retry dispatcher entry without an authorized deadline")
		return
	}
	if assert.NotNil(t, actual, "surviving recovery must remain registered in the real dispatcher") {
		assert.Equal(t, expected.Token, actual.token)
		assert.Equal(t, expected.Due, actual.due)
	}
}
