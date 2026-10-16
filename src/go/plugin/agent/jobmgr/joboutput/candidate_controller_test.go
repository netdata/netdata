// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func candidateTestConfig(source, value string) confgroup.Config {
	return confgroup.Config{"name": "job", "module": "test", "value": value}.SetSource(source).SetSourceType(confgroup.TypeUser).SetProvider("test")
}

func candidateTestInventory(version uint64, auto bool, configs ...confgroup.Config) DiscoveredJobInventory {
	inv := DiscoveredJobInventory{FullName: "test_job", Version: version, AutoEnable: auto, Sources: make(map[string]uint64)}
	for i, config := range configs {
		inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: config, Source: config.Source(), Incarnation: uint64(i + 1)})
		inv.Sources[config.Source()] = 1
	}
	return inv
}

func applyCandidateDelta(t *testing.T, delta *candidateDelta, err error) *candidateDelta {
	t.Helper()
	require.NoError(t, err)
	require.True(t, delta.apply())
	return delta
}

func candidateInventory(t *testing.T, c *candidateController, inv DiscoveredJobInventory, now int64) *candidateDelta {
	t.Helper()
	delta, err := c.prepareInventory(inv, now)
	return applyCandidateDelta(t, delta, err)
}

func candidateEnable(t *testing.T, c *candidateController, now int64) *candidateDelta {
	t.Helper()
	delta, err := c.prepareEnable("test_job", now)
	return applyCandidateDelta(t, delta, err)
}

func candidateResult(t *testing.T, c *candidateController, work *candidateWork, outcome candidateOutcome, now int64) *candidateDelta {
	t.Helper()
	require.NotNil(t, work)
	delta, err := c.prepareOutcome("test_job", work.Token, outcome, now)
	return applyCandidateDelta(t, delta, err)
}

func TestCandidateControllerFallback(t *testing.T) {
	for name, auto := range map[string]bool{"passive": false, "automatic": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
			inventory := candidateInventory(t, c, candidateTestInventory(1, auto, a, b), 0)
			work := inventory.Decision.Work
			if !auto {
				require.Nil(t, work)
				require.Equal(t, dyncfg.StatusAccepted, inventory.Decision.Binding.Health)
				work = candidateEnable(t, c, 0).Decision.Work
			}
			require.NotNil(t, work)
			first := work.Token.Candidate
			failure := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
			require.NotNil(t, failure.Decision.Work)
			second := failure.Decision.Work
			assert.NotEqual(t, first, second.Token.Candidate)
			assert.Equal(t, dyncfg.StatusFailed, failure.Decision.Binding.Health)
			accepted := candidateResult(t, c, second, candidateOutcome{Kind: candidateAdmitted}, 1)
			assert.Nil(t, accepted.Decision.Work)
			assert.Equal(t, second.Config, accepted.Decision.Binding.Config)
			ready, err := c.prepareReady("test_job", second.Token)
			applyCandidateDelta(t, ready, err)
			snapshot := c.snapshot("test_job")
			assert.True(t, snapshot.SearchClosed)
			assert.Nil(t, snapshot.Wake)
			assert.Equal(t, dyncfg.StatusRunning, snapshot.Binding.Health)
			assert.True(t, c.valid("test_job", second.Token))
			late := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true}, 2)
			assert.True(t, late.Decision.Ignored)
		})
	}
}

func TestCandidateControllerFiniteDueUnion(t *testing.T) {
	c := newCandidateController(1)
	a, b, d := candidateTestConfig("a", "a"), candidateTestConfig("b", "b"), candidateTestConfig("d", "d")
	inv := candidateTestInventory(1, false, a, b)
	candidateInventory(t, c, inv, 0)
	first := candidateEnable(t, c, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0).Decision.Work
	exhausted := candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 60}, 1)
	require.NotNil(t, exhausted.Decision.Wake)
	assert.Equal(t, int64(10), exhausted.Decision.Wake.Due)
	// A retry becoming due and a new initial candidate are frozen into one pass.
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: d, Source: d.Source(), Incarnation: 3})
	inv.Sources[d.Source()] = 1
	added := candidateInventory(t, c, inv, 10)
	require.NotNil(t, added.Decision.Work)
	assert.Equal(t, d, added.Decision.Work.Config)
	// A later arrival cannot displace the already-due member of that finite pass.
	e := candidateTestConfig("e", "e")
	inv.Version = 3
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: e, Source: e.Source(), Incarnation: 4})
	inv.Sources[e.Source()] = 1
	candidateInventory(t, c, inv, 11)
	due := candidateResult(t, c, added.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true}, 12)
	require.NotNil(t, due.Decision.Work)
	assert.Equal(t, first.Token.Candidate, due.Decision.Work.Token.Candidate)
	next := candidateResult(t, c, due.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 13)
	require.NotNil(t, next.Decision.Work)
	assert.Equal(t, e, next.Decision.Work.Config)
	final := candidateResult(t, c, next.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true}, 14)
	require.NotNil(t, final.Decision.Wake)
	assert.Equal(t, int64(23), final.Decision.Wake.Due)
	// The slower sibling is not retried early by its sibling's wake.
	assert.Equal(t, int64(61), c.snapshot("test_job").Entries[second.Token.Candidate].Due)
}

func TestCandidateControllerRetainedAdmissions(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "old"), candidateTestConfig("b", "sibling")
	inv := candidateTestInventory(1, true, a, b)
	initial := candidateInventory(t, c, inv, 0).Decision.Work
	accepted := candidateResult(t, c, initial, candidateOutcome{Kind: candidateAdmitted}, 0)
	require.NotNil(t, accepted.Decision.Binding)
	oldKey := initial.Token.Candidate
	// The admitted source replaces only its old record. Its old grant survives
	// pending validation and exact rejection of the replacement.
	replacement := candidateTestConfig(initial.Config.Source(), "replacement")
	inv.Version = 2
	for i := range inv.Candidates {
		if inv.Candidates[i].Source == initial.Config.Source() {
			inv.Candidates[i].Config = replacement
			inv.Candidates[i].Incarnation = 7
		}
	}
	proposed := candidateInventory(t, c, inv, 1)
	require.NotNil(t, proposed.Decision.Work)
	assert.True(t, proposed.Decision.Work.Tentative)
	assert.True(t, c.valid("test_job", initial.Token))
	rejected := candidateResult(t, c, proposed.Decision.Work, candidateOutcome{Kind: candidateRejected}, 2)
	assert.Nil(t, rejected.Decision.Binding)
	assert.True(t, c.valid("test_job", initial.Token))
	assert.True(t, c.snapshot("test_job").Entries[oldKey].Admitted)
	sibling := candidateResult(t, c, initial, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 3).Decision.Work
	require.NotNil(t, sibling)
	assert.NotEqual(t, oldKey, sibling.Token.Candidate)
	candidateResult(t, c, sibling, candidateOutcome{Kind: candidateAdmitted}, 3)
	assert.True(t, c.snapshot("test_job").Entries[oldKey].Admitted, "sibling adoption must not erase retained admission")
	end := candidateResult(t, c, sibling, candidateOutcome{Kind: candidateFailed, Listed: true}, 4)
	require.NotNil(t, end.Decision.Wake)
	assert.Equal(t, int64(13), end.Decision.Wake.Due)
	disable, err := c.prepareDisable("test_job")
	applyCandidateDelta(t, disable, err)
	assert.False(t, c.valid("test_job", initial.Token))
	assert.True(t, c.snapshot("test_job").Entries[oldKey].Admitted)
	reenabled := candidateEnable(t, c, 5)
	require.NotNil(t, reenabled.Decision.Work)
	assert.NotEqual(t, initial.Token.Intent, reenabled.Decision.Work.Token.Intent)
	// Genuine removal revokes the old retained source grant even while another
	// admitted source remains the public binding.
	inv.Version = 3
	var kept []DiscoveredJobCandidate
	for _, candidate := range inv.Candidates {
		if candidate.Source != oldKey.Source {
			kept = append(kept, candidate)
		}
	}
	inv.Candidates = kept
	delete(inv.Sources, oldKey.Source)
	candidateInventory(t, c, inv, 6)
	assert.NotContains(t, c.snapshot("test_job").Entries, oldKey)
}

func TestCandidateControllerReadinessClosure(t *testing.T) {
	for name, readyNotification := range map[string]bool{"ready already applied": true, "terminal arrives first": false} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(9)
			a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
			inv := candidateTestInventory(1, false, a, b)
			candidateInventory(t, c, inv, 0)
			work := candidateEnable(t, c, 0).Decision.Work
			candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
			if readyNotification {
				delta, err := c.prepareReady("test_job", work.Token)
				applyCandidateDelta(t, delta, err)
			}
			failure := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10, AfterReady: true}, 1)
			assert.Nil(t, failure.Decision.Work)
			assert.Nil(t, failure.Decision.Wake)
			assert.True(t, c.snapshot("test_job").SearchClosed)
			inv.Version = 2
			d := candidateTestConfig("d", "d")
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: d, Source: d.Source(), Incarnation: 3})
			inv.Sources[d.Source()] = 1
			addition := candidateInventory(t, c, inv, 2)
			assert.Nil(t, addition.Decision.Work)
			assert.Nil(t, addition.Decision.Wake)
		})
	}
}

func TestCandidateControllerInventoryReplay(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
	first := candidateInventory(t, c, inv, 0)
	require.NotNil(t, first.Decision.Work)
	for name, version := range map[string]uint64{"same": 1, "older": 0, "identical newer": 2} {
		t.Run(name, func(t *testing.T) {
			replay := inv
			replay.Version = version
			if version == 0 {
				replay.Version = 1
				newer := inv
				newer.Version = 2
				candidateInventory(t, c, newer, 0)
			}
			delta := candidateInventory(t, c, replay, 2)
			assert.Nil(t, delta.Decision.Work)
			assert.True(t, c.valid("test_job", first.Decision.Work.Token))
		})
	}
	conflict := inv
	conflict.Version = c.snapshot("test_job").Inventory.Version
	conflict.AutoEnable = false
	_, err := c.prepareInventory(conflict, 3)
	require.ErrorContains(t, err, "conflicting")
}

func TestCandidateControllerRevocationAndResume(t *testing.T) {
	for name, kind := range map[string]string{"disable": "disable", "remove": "remove", "retire": "retire", "source removal": "inventory"} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
			work := candidateInventory(t, c, inv, 0).Decision.Work
			resumed, err := c.prepareResume("test_job", work.Token, 0)
			applyCandidateDelta(t, resumed, err)
			require.NotNil(t, resumed.Decision.Work)
			assert.False(t, c.valid("test_job", work.Token))
			assert.True(t, c.valid("test_job", resumed.Decision.Work.Token))
			duplicate, err := c.prepareResume("test_job", work.Token, 0)
			applyCandidateDelta(t, duplicate, err)
			assert.True(t, duplicate.Decision.Ignored)
			switch kind {
			case "disable":
				delta, err := c.prepareDisable("test_job")
				applyCandidateDelta(t, delta, err)
			case "remove":
				delta, err := c.prepareRemove("test_job")
				applyCandidateDelta(t, delta, err)
			case "retire":
				c.retire()
			case "inventory":
				inv.Version = 2
				inv.Candidates = nil
				inv.Sources = nil
				candidateInventory(t, c, inv, 1)
			}
			assert.False(t, c.valid("test_job", resumed.Decision.Work.Token))
		})
	}
}

func TestCandidateControllerMaskedCursor(t *testing.T) {
	c := newCandidateController(1)
	override := candidateTestConfig("override", "override").SetSourceType(confgroup.TypeDyncfg)
	exact, err := c.prepareAdoptExact(override, false, dyncfg.StatusAccepted, true, 0, 0)
	applyCandidateDelta(t, exact, err)
	a := candidateTestConfig("a", "a")
	inv := candidateTestInventory(1, true, a)
	masked := candidateInventory(t, c, inv, 0)
	assert.Nil(t, masked.Decision.Work)
	require.NotNil(t, c.snapshot("test_job").Cursor)
	assert.Equal(t, a.UID(), c.snapshot("test_job").Cursor.UID)
	remove, err := c.prepareRemove("test_job")
	applyCandidateDelta(t, remove, err)
	inv.Version = 2
	b := candidateTestConfig("z", "z")
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: b, Source: b.Source(), Incarnation: 2})
	inv.Sources[b.Source()] = 1
	unchanged := candidateInventory(t, c, inv, 1)
	assert.Nil(t, unchanged.Decision.Work, "irrelevant sibling must not restore the consumed lower source choice")
	assert.Nil(t, c.snapshot("test_job").Binding)
	inv.Version = 3
	inv.Candidates = inv.Candidates[1:]
	delete(inv.Sources, "a")
	changed := candidateInventory(t, c, inv, 2)
	require.NotNil(t, changed.Decision.Work)
	assert.Equal(t, b, changed.Decision.Work.Config)
}

func TestCandidateControllerExactFailure(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	candidateInventory(t, c, candidateTestInventory(1, false, a, b), 0)
	delta, err := c.prepareAdoptExact(a, true, dyncfg.StatusFailed, true, 7, 1)
	exact := applyCandidateDelta(t, delta, err)
	assert.Nil(t, exact.Decision.Work)
	require.NotNil(t, exact.Decision.Wake)
	assert.Equal(t, int64(8), exact.Decision.Wake.Due)
	wake, err := c.prepareWake("test_job", exact.Decision.Wake.Token, 8)
	applyCandidateDelta(t, wake, err)
	require.NotNil(t, wake.Decision.Work)
	assert.Equal(t, a, wake.Decision.Work.Config)
	failed := candidateResult(t, c, wake.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true}, 8)
	assert.Nil(t, failed.Decision.Work)
	assert.Equal(t, candidateExactConfig, c.snapshot("test_job").Scope)
}

func TestCandidateControllerPreparedDeltaIsolation(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, false, candidateTestConfig("a", "a"))
	one, err := c.prepareInventory(inv, 0)
	require.NoError(t, err)
	two, err := c.prepareInventory(inv, 0)
	require.NoError(t, err)
	assert.Nil(t, c.snapshot("test_job"))
	require.True(t, one.apply())
	assert.False(t, two.apply())
	assert.False(t, one.apply())
}

func TestCandidateControllerEnableAfterExactAndRetryWait(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	candidateInventory(t, c, candidateTestInventory(1, false, a, b), 0)
	delta, err := c.prepareAdoptExact(a, true, dyncfg.StatusFailed, true, 30, 0)
	exact := applyCandidateDelta(t, delta, err)
	require.NotNil(t, exact.Decision.Wake)
	before := c.snapshot("test_job")
	require.Len(t, before.Entries, 2)
	fresh := candidateEnable(t, c, 1)
	require.NotNil(t, fresh.Decision.Work)
	assert.NotEqual(t, before.Intent, fresh.Decision.Work.Token.Intent)
	assert.Equal(t, candidateSourceAlternatives, c.snapshot("test_job").Scope)
	sibling := candidateResult(t, c, fresh.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true}, 1)
	require.NotNil(t, sibling.Decision.Work)
	assert.NotEqual(t, fresh.Decision.Work.Token.Candidate, sibling.Decision.Work.Token.Candidate)
	assert.Equal(t, b, sibling.Decision.Work.Config)
}

func TestCandidateControllerRejectedFirstProposal(t *testing.T) {
	c := newCandidateController(1)
	user := candidateTestConfig("user", "user")
	stock := candidateTestConfig("stock", "stock").SetSourceType(confgroup.TypeStock)
	inv := candidateTestInventory(1, true, user, stock)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	require.NotNil(t, first)
	assert.Equal(t, user, first.Config)
	rejected := candidateResult(t, c, first, candidateOutcome{Kind: candidateRejected}, 0)
	require.NotNil(t, rejected.Decision.Work)
	assert.Equal(t, stock, rejected.Decision.Work.Config)
}

func TestCandidateControllerPriorityFailureMasksLower(t *testing.T) {
	c := newCandidateController(1)
	user := candidateTestConfig("user", "user")
	stock := candidateTestConfig("stock", "stock").SetSourceType(confgroup.TypeStock)
	first := candidateInventory(t, c, candidateTestInventory(1, true, user, stock), 0).Decision.Work
	failed := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true}, 0)
	assert.Nil(t, failed.Decision.Work)
	assert.Nil(t, failed.Decision.Wake)
	assert.Equal(t, user, c.snapshot("test_job").Binding.Config)
}

func TestCandidateControllerWakeProjectionAndStaleWake(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
	work := candidateInventory(t, c, inv, 0).Decision.Work
	failed := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
	require.NotNil(t, failed.Decision.Wake)
	inv.Version = 2
	replay := candidateInventory(t, c, inv, 1)
	assert.Equal(t, failed.Decision.Wake, replay.Decision.Wake)
	wake, err := c.prepareWake("test_job", failed.Decision.Wake.Token, 10)
	applyCandidateDelta(t, wake, err)
	require.NotNil(t, wake.Decision.Work)
	duplicate, err := c.prepareWake("test_job", failed.Decision.Wake.Token, 10)
	applyCandidateDelta(t, duplicate, err)
	assert.True(t, duplicate.Decision.Ignored)
	assert.Nil(t, duplicate.Decision.Work)
}

func TestCandidateControllerGenerationOverflow(t *testing.T) {
	c := newCandidateController(1)
	first := candidateInventory(t, c, candidateTestInventory(1, true, candidateTestConfig("a", "a")), 0).Decision.Work
	c.sequence = ^uint64(0)
	_, err := c.prepareOutcome("test_job", first.Token, candidateOutcome{Kind: candidateAdmitted}, 0)
	require.ErrorContains(t, err, "wrapped")
	assert.Nil(t, c.snapshot("test_job").Binding)
	assert.True(t, c.valid("test_job", first.Token))
}

func TestCandidateControllerRemovedOverrideCannotReenterSourcePass(t *testing.T) {
	c := newCandidateController(1)
	override := candidateTestConfig("override", "override").SetSourceType(confgroup.TypeDyncfg)
	delta, err := c.prepareAdoptExact(override, false, dyncfg.StatusAccepted, true, 0, 0)
	applyCandidateDelta(t, delta, err)
	a := candidateTestConfig("a", "a")
	inv := candidateTestInventory(1, true, a)
	candidateInventory(t, c, inv, 0)
	remove, err := c.prepareRemove("test_job")
	applyCandidateDelta(t, remove, err)
	b := candidateTestConfig("b", "b")
	inv.Version = 2
	inv.Candidates = []DiscoveredJobCandidate{{Config: b, Source: "b", Incarnation: 2}}
	inv.Sources = map[string]uint64{"b": 1}
	source := candidateInventory(t, c, inv, 0).Decision.Work
	require.NotNil(t, source)
	failed := candidateResult(t, c, source, candidateOutcome{Kind: candidateFailed, Listed: true}, 0)
	assert.Nil(t, failed.Decision.Work)
	assert.Equal(t, b, c.snapshot("test_job").Binding.Config)
}

func TestCandidateControllerRetainedGrantAfterSiblingSourceRemoved(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "old"), candidateTestConfig("b", "b")
	inv := candidateTestInventory(1, true, a, b)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, first, candidateOutcome{Kind: candidateAdmitted}, 0)
	replacement := candidateTestConfig(first.Config.Source(), "replacement")
	inv.Version = 2
	for i := range inv.Candidates {
		if inv.Candidates[i].Source == first.Config.Source() {
			inv.Candidates[i].Config = replacement
			inv.Candidates[i].Incarnation = 7
		}
	}
	proposal := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, proposal, candidateOutcome{Kind: candidateRejected}, 0)
	sibling := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 1}, 0).Decision.Work
	candidateResult(t, c, sibling, candidateOutcome{Kind: candidateAdmitted}, 0)
	inv.Version = 3
	var kept []DiscoveredJobCandidate
	for _, observed := range inv.Candidates {
		if observed.Source != sibling.Config.Source() {
			kept = append(kept, observed)
		}
	}
	inv.Candidates = kept
	delete(inv.Sources, sibling.Config.Source())
	removed := candidateInventory(t, c, inv, 1)
	require.NotNil(t, removed.Decision.Work)
	assert.Equal(t, first.Token.Candidate, removed.Decision.Work.Token.Candidate)
}

func TestCandidateControllerEnableProjectsAccepted(t *testing.T) {
	for name, health := range map[string]dyncfg.Status{"passive": dyncfg.StatusAccepted, "disabled": dyncfg.StatusDisabled, "failed": dyncfg.StatusFailed} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			config := candidateTestConfig("a", "a")
			candidateInventory(t, c, candidateTestInventory(1, false, config), 0)
			switch health {
			case dyncfg.StatusDisabled:
				delta, err := c.prepareDisable("test_job")
				applyCandidateDelta(t, delta, err)
			case dyncfg.StatusFailed:
				work := candidateEnable(t, c, 0).Decision.Work
				candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true}, 0)
			}
			enabled := candidateEnable(t, c, 1)
			require.NotNil(t, enabled.Decision.Work)
			require.NotNil(t, enabled.Decision.Binding)
			assert.Equal(t, dyncfg.StatusAccepted, enabled.Decision.Binding.Health)
			assert.Equal(t, config, enabled.Decision.Binding.Config)
		})
	}
}

func TestCandidateControllerEnableTentativePreservesBinding(t *testing.T) {
	c := newCandidateController(1)
	a := candidateTestConfig("a", "a")
	inv := candidateTestInventory(1, true, a)
	work := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
	disable, err := c.prepareDisable("test_job")
	applyCandidateDelta(t, disable, err)
	replacement := candidateTestConfig("a", "replacement")
	inv.Version = 2
	inv.Candidates[0].Config = replacement
	inv.Candidates[0].Incarnation = 2
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposal)
	disable, err = c.prepareDisable("test_job")
	applyCandidateDelta(t, disable, err)
	before := c.snapshot("test_job").Binding
	enabled := candidateEnable(t, c, 2)
	require.NotNil(t, enabled.Decision.Work)
	assert.True(t, enabled.Decision.Work.Tentative)
	assert.Nil(t, enabled.Decision.Binding)
	assert.Equal(t, before, c.snapshot("test_job").Binding)
}

func TestCandidateControllerFrameworkBlockPolicies(t *testing.T) {
	for name, policy := range map[string]candidateBlockPolicy{"source deadline": candidateBlockAcceptedWait, "accepted deadline": candidateBlockFailedRetry, "quarantine": candidateBlockQuarantined} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
			inv := candidateTestInventory(1, true, a, b)
			work := candidateInventory(t, c, inv, 0).Decision.Work
			if policy != candidateBlockAcceptedWait {
				candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
			}
			blocked := candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked, Block: policy, Listed: true, RetryAfter: 10}, 2)
			require.NotNil(t, blocked.Decision.Binding)
			expectedHealth := dyncfg.StatusFailed
			if policy == candidateBlockAcceptedWait {
				expectedHealth = dyncfg.StatusAccepted
			}
			assert.Equal(t, expectedHealth, blocked.Decision.Binding.Health)
			assert.Nil(t, blocked.Decision.Work)
			snapshot := c.snapshot("test_job")
			require.NotNil(t, snapshot.Active)
			assert.False(t, snapshot.Active.Tentative)
			// No inventory addition can route around a blocked process identity.
			third := candidateTestConfig("c", "c")
			inv.Version = 2
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: third, Source: "c", Incarnation: 3})
			inv.Sources["c"] = 1
			added := candidateInventory(t, c, inv, 3)
			assert.Nil(t, added.Decision.Work)
			// Late duplicate classification must not reset Failed or its original due.
			duplicate := candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked, Block: policy, Listed: true, RetryAfter: 99}, 3)
			assert.True(t, duplicate.Decision.Ignored)
			resume, err := c.prepareResume("test_job", work.Token, 3)
			applyCandidateDelta(t, resume, err)
			switch policy {
			case candidateBlockAcceptedWait:
				require.NotNil(t, resume.Decision.Work)
				assert.Equal(t, work.Token.Candidate, resume.Decision.Work.Token.Candidate)
				assert.NotEqual(t, work.Token.Attempt, resume.Decision.Work.Token.Attempt)
				assert.Nil(t, resume.Decision.Wake)
			case candidateBlockFailedRetry:
				assert.True(t, resume.Decision.Ignored)
				require.NotNil(t, blocked.Decision.Wake)
				assert.Equal(t, int64(12), blocked.Decision.Wake.Due)
				wake, err := c.prepareWake("test_job", blocked.Decision.Wake.Token, 12)
				applyCandidateDelta(t, wake, err)
				require.NotNil(t, wake.Decision.Work)
				assert.Equal(t, work.Token.Candidate, wake.Decision.Work.Token.Candidate)
				assert.NotEqual(t, work.Token.Attempt, wake.Decision.Work.Token.Attempt)
				assert.Equal(t, dyncfg.StatusAccepted, wake.Decision.Binding.Health)
			case candidateBlockQuarantined:
				assert.True(t, resume.Decision.Ignored)
				assert.Nil(t, blocked.Decision.Wake)
				enabled := candidateEnable(t, c, 50)
				assert.True(t, enabled.Decision.Ignored)
			}
		})
	}
}

func TestCandidateControllerFrameworkBlockAfterReadyClosesSearch(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	work := candidateInventory(t, c, candidateTestInventory(1, true, a, b), 0).Decision.Work
	candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
	blocked := candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, Listed: true, RetryAfter: 10, AfterReady: true}, 0)
	assert.Equal(t, dyncfg.StatusFailed, blocked.Decision.Binding.Health)
	assert.True(t, c.snapshot("test_job").SearchClosed)
	assert.Nil(t, blocked.Decision.Work)
	assert.Nil(t, blocked.Decision.Wake)
	assert.False(t, c.valid("test_job", work.Token))
}

func TestCandidateControllerPassiveInventoryValidation(t *testing.T) {
	for name, incumbent := range map[string]bool{"first observation": false, "invalid replacement": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			high := candidateTestConfig("user", "old")
			low := candidateTestConfig("stock", "stock").SetSourceType(confgroup.TypeStock)
			inv := candidateTestInventory(1, false, high, low)
			if incumbent {
				candidateInventory(t, c, inv, 0)
				inv.Version = 2
				inv.Candidates[0].Config = candidateTestConfig("user", "invalid")
				inv.Candidates[0].Incarnation = 3
			}
			invalid := map[candidateKey]bool{candidateIdentity(inv.Candidates[0]): true}
			delta, err := c.prepareInventoryValidated(inv, invalid, 0)
			applied := applyCandidateDelta(t, delta, err)
			assert.Nil(t, applied.Decision.Work)
			assert.Equal(t, uint64(1), c.snapshot("test_job").Inventory.Sources["user"])
			if incumbent {
				assert.Equal(t, high, c.snapshot("test_job").Binding.Config)
				assert.Nil(t, applied.Decision.Binding)
			} else {
				require.NotNil(t, applied.Decision.Binding)
				assert.Equal(t, low, applied.Decision.Binding.Config)
			}
		})
	}
}

func TestCandidateControllerRepeatedAdmittedPreservesPass(t *testing.T) {
	c := newCandidateController(1)
	a, b, d := candidateTestConfig("a", "a"), candidateTestConfig("b", "b"), candidateTestConfig("d", "d")
	candidateInventory(t, c, candidateTestInventory(1, false, a, b, d), 0)
	first := candidateEnable(t, c, 0).Decision.Work
	second := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 30}, 0).Decision.Work
	candidateResult(t, c, second, candidateOutcome{Kind: candidateAdmitted}, 0)
	before := c.snapshot("test_job")
	candidateResult(t, c, second, candidateOutcome{Kind: candidateAdmitted}, 1)
	after := c.snapshot("test_job")
	assert.Equal(t, before.Pass, after.Pass)
	assert.Equal(t, before.PassCursor, after.PassCursor)
	assert.Equal(t, before.Entries[first.Token.Candidate], after.Entries[first.Token.Candidate])
	next := candidateResult(t, c, second, candidateOutcome{Kind: candidateFailed, Listed: true}, 1)
	require.NotNil(t, next.Decision.Work)
	assert.NotEqual(t, first.Token.Candidate, next.Decision.Work.Token.Candidate)
	assert.NotEqual(t, second.Token.Candidate, next.Decision.Work.Token.Candidate)
}

func TestCandidateControllerBlockedRetrySourceRemovalRevokesWake(t *testing.T) {
	c := newCandidateController(1)
	a := candidateTestConfig("a", "a")
	inv := candidateTestInventory(1, true, a)
	work := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
	blocked := candidateResult(t, c, work, candidateOutcome{Kind: candidateFrameworkBlocked, Block: candidateBlockFailedRetry, Listed: true, RetryAfter: 10}, 0)
	require.NotNil(t, blocked.Decision.Wake)
	inv.Version = 2
	inv.Candidates = nil
	inv.Sources = nil
	removed := candidateInventory(t, c, inv, 1)
	assert.Nil(t, removed.Decision.Wake)
	assert.Nil(t, c.snapshot("test_job").Wake)
	assert.False(t, c.valid("test_job", work.Token))
	wake, err := c.prepareWake("test_job", blocked.Decision.Wake.Token, 10)
	applyCandidateDelta(t, wake, err)
	assert.True(t, wake.Decision.Ignored)
	assert.Nil(t, wake.Decision.Work)
}

func TestCandidateControllerHiddenStockNewInitial(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a").SetSourceType(confgroup.TypeStock), candidateTestConfig("b", "b").SetSourceType(confgroup.TypeStock)
	inv := candidateTestInventory(1, true, a, b)
	work := candidateInventory(t, c, inv, 0).Decision.Work
	next := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed}, 0).Decision.Work
	require.NotNil(t, next)
	exhausted := candidateResult(t, c, next, candidateOutcome{Kind: candidateFailed}, 0)
	assert.True(t, exhausted.Decision.Remove)
	assert.Nil(t, exhausted.Decision.Work)
	added := candidateTestConfig("new", "new-stock").SetSourceType(confgroup.TypeStock)
	inv.Version = 2
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: added, Source: "new", Incarnation: 3})
	inv.Sources["new"] = 1
	discovered := candidateInventory(t, c, inv, 0)
	require.NotNil(t, discovered.Decision.Work)
	assert.Equal(t, added, discovered.Decision.Work.Config)
}

func TestCandidateControllerTrustedSourceRevoked(t *testing.T) {
	c := newCandidateController(1)
	trusted := candidateTestConfig("pipeline-target", "trusted").SetSourceType(confgroup.TypeDiscovered).SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(true)
	sibling := candidateTestConfig("other-target", "other").SetSourceType(confgroup.TypeDiscovered).SetDiscoveryPipelineID("other")
	inv := candidateTestInventory(1, true, trusted)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, first, candidateOutcome{Kind: candidateAdmitted}, 0)
	literal := candidateTestConfig("pipeline-target", "invalid-literal").SetSourceType(confgroup.TypeDiscovered).SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(false)
	inv.Version = 2
	inv.Candidates[0].Config = literal
	inv.Candidates[0].Incarnation = 2
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposal)
	revoked := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateSourceRevoked, Listed: true}, 1)
	require.NotNil(t, revoked.Decision.Binding)
	assert.Equal(t, literal, revoked.Decision.Binding.Config)
	assert.Equal(t, dyncfg.StatusFailed, revoked.Decision.Binding.Health)
	assert.True(t, revoked.Decision.Revoke)
	assert.Nil(t, revoked.Decision.Work)
	assert.Nil(t, revoked.Decision.Wake)
	snapshot := c.snapshot("test_job")
	assert.Equal(t, candidateManualSourceRevoked, snapshot.Manual)
	assert.False(t, snapshot.SearchClosed, "trust opt-out is not runtime readiness")
	assert.False(t, snapshot.FrameworkBlocked, "trust opt-out is not process quarantine")
	assert.NotContains(t, snapshot.Entries, first.Token.Candidate)
	assert.False(t, c.valid("test_job", first.Token))
	assert.False(t, c.valid("test_job", proposal.Token))
	inv.Version = 3
	inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: sibling, Source: sibling.Source(), Incarnation: 3})
	inv.Sources[sibling.Source()] = 1
	addition := candidateInventory(t, c, inv, 2)
	assert.Nil(t, addition.Decision.Work, "proposal error cannot open operational sibling search")
	explicit := candidateEnable(t, c, 3)
	require.NotNil(t, explicit.Decision.Work)
	assert.Equal(t, literal, explicit.Decision.Work.Config)
	assert.Equal(t, candidateManualNone, c.snapshot("test_job").Manual)
}

func TestCandidateControllerSiblingAdmissionUpdatesSourceCursor(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	inv := candidateTestInventory(1, true, a, b)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	sibling := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true}, 0).Decision.Work
	require.NotNil(t, sibling)
	candidateResult(t, c, sibling, candidateOutcome{Kind: candidateAdmitted}, 0)
	assert.Equal(t, sibling.Token.Candidate, *c.snapshot("test_job").Cursor)
	ready, err := c.prepareReady("test_job", sibling.Token)
	applyCandidateDelta(t, ready, err)
	inv.Version = 2
	var kept []DiscoveredJobCandidate
	for _, candidate := range inv.Candidates {
		if candidate.Source != sibling.Config.Source() {
			kept = append(kept, candidate)
		}
	}
	inv.Candidates = kept
	delete(inv.Sources, sibling.Config.Source())
	removed := candidateInventory(t, c, inv, 1)
	require.NotNil(t, removed.Decision.Work, "actual admitted source removal must reconcile even after readiness")
	assert.True(t, removed.Decision.Work.Tentative)
	assert.Equal(t, first.Token.Candidate, removed.Decision.Work.Token.Candidate)
}

func TestCandidateControllerDueRetryExposure(t *testing.T) {
	for name, sibling := range map[string]bool{"same admitted candidate": false, "different admitted candidate": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			configs := []confgroup.Config{candidateTestConfig("a", "a")}
			if sibling {
				configs = append(configs, candidateTestConfig("b", "b"))
			}
			first := candidateInventory(t, c, candidateTestInventory(1, true, configs...), 0).Decision.Work
			failed := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
			if sibling {
				require.NotNil(t, failed.Decision.Work)
				failed = candidateResult(t, c, failed.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 60}, 1)
			}
			require.NotNil(t, failed.Decision.Wake)
			assert.Equal(t, int64(10), failed.Decision.Wake.Due)
			before := c.snapshot("test_job")
			wake, err := c.prepareWake("test_job", failed.Decision.Wake.Token, 10)
			applied := applyCandidateDelta(t, wake, err)
			require.NotNil(t, applied.Decision.Work)
			assert.Equal(t, first.Token.Candidate, applied.Decision.Work.Token.Candidate)
			assert.Equal(t, before.Intent, applied.Decision.Work.Token.Intent)
			assert.NotEqual(t, first.Token.Attempt, applied.Decision.Work.Token.Attempt)
			if sibling {
				assert.Nil(t, applied.Decision.Binding)
				assert.Equal(t, before.Binding, c.snapshot("test_job").Binding)
			} else {
				require.NotNil(t, applied.Decision.Binding)
				assert.Equal(t, dyncfg.StatusAccepted, applied.Decision.Binding.Health)
				assert.Equal(t, before.Binding.Key, applied.Decision.Binding.Key)
				assert.Equal(t, before.Binding.Config, applied.Decision.Binding.Config)
			}
			duplicate, err := c.prepareWake("test_job", failed.Decision.Wake.Token, 10)
			applyCandidateDelta(t, duplicate, err)
			assert.True(t, duplicate.Decision.Ignored)
			assert.Nil(t, duplicate.Decision.Work)
			next := candidateResult(t, c, applied.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 12)
			require.NotNil(t, next.Decision.Wake)
			assert.Equal(t, int64(22), next.Decision.Wake.Due)
			assert.Nil(t, next.Decision.Work)
		})
	}
}

func TestCandidateControllerSourceWaitAdmission(t *testing.T) {
	for name, withIncumbent := range map[string]bool{"initial source": false, "replacement source": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			initial := candidateTestConfig("a", "a")
			inv := candidateTestInventory(1, true, initial)
			work := candidateInventory(t, c, inv, 0).Decision.Work
			var predecessor candidateWorkToken
			if withIncumbent {
				candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
				ready, err := c.prepareReady("test_job", work.Token)
				applyCandidateDelta(t, ready, err)
				predecessor = work.Token
				inv.Version = 2
				inv.Candidates[0].Config = candidateTestConfig("a", "replacement")
				inv.Candidates[0].Incarnation = 2
				work = candidateInventory(t, c, inv, 1).Decision.Work
			}
			require.NotNil(t, work)
			require.True(t, work.Tentative)
			waiting := candidateResult(t, c, work, candidateOutcome{Kind: candidateWaiting, AdmitWaiting: true}, 2)
			require.NotNil(t, waiting.Decision.Binding)
			assert.Equal(t, work.Config, waiting.Decision.Binding.Config)
			assert.Equal(t, dyncfg.StatusAccepted, waiting.Decision.Binding.Health)
			assert.Nil(t, waiting.Decision.Work)
			assert.Nil(t, waiting.Decision.Wake)
			snapshot := c.snapshot("test_job")
			assert.Equal(t, candidateWaitingPhase, snapshot.Phase)
			assert.False(t, snapshot.FrameworkBlocked)
			assert.False(t, snapshot.SearchClosed)
			assert.Nil(t, snapshot.Proposal)
			require.NotNil(t, snapshot.Active)
			assert.Equal(t, work.Token, snapshot.Active.Token)
			assert.False(t, snapshot.Active.Tentative)
			assert.True(t, c.valid("test_job", work.Token))
			if withIncumbent {
				assert.False(t, c.valid("test_job", predecessor))
			}
			disable, err := c.prepareDisable("test_job")
			applyCandidateDelta(t, disable, err)
			assert.False(t, c.valid("test_job", work.Token))
			resume, err := c.prepareResume("test_job", work.Token, 3)
			applyCandidateDelta(t, resume, err)
			assert.True(t, resume.Decision.Ignored)
			assert.Nil(t, resume.Decision.Work)
		})
	}
}

func TestCandidateControllerTentativePlainWaitRetainsIncumbent(t *testing.T) {
	c := newCandidateController(1)
	initial := candidateTestConfig("a", "a")
	inv := candidateTestInventory(1, true, initial)
	work := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
	ready, err := c.prepareReady("test_job", work.Token)
	applyCandidateDelta(t, ready, err)
	incumbent := c.snapshot("test_job").Binding
	inv.Version = 2
	inv.Candidates[0].Config = candidateTestConfig("a", "replacement")
	inv.Candidates[0].Incarnation = 2
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	waiting := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateWaiting}, 2)
	assert.Nil(t, waiting.Decision.Binding)
	assert.Equal(t, incumbent, c.snapshot("test_job").Binding)
	assert.True(t, c.valid("test_job", work.Token))
	assert.True(t, c.valid("test_job", proposal.Token))
	assert.NotNil(t, c.snapshot("test_job").Proposal)
	assert.Nil(t, waiting.Decision.Wake)
}

func TestCandidateControllerAcceptedWaitRetainsPassAndCapability(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	candidateInventory(t, c, candidateTestInventory(1, false, a, b), 0)
	work := candidateEnable(t, c, 0).Decision.Work
	before := c.snapshot("test_job")
	waiting := candidateResult(t, c, work, candidateOutcome{Kind: candidateWaiting}, 1)
	assert.Nil(t, waiting.Decision.Binding)
	assert.Equal(t, before.Binding, c.snapshot("test_job").Binding)
	assert.Equal(t, before.Pass, c.snapshot("test_job").Pass)
	assert.Equal(t, before.PassCursor, c.snapshot("test_job").PassCursor)
	assert.Equal(t, work.Token, c.snapshot("test_job").Active.Token)
	assert.Equal(t, dyncfg.StatusAccepted, c.snapshot("test_job").Binding.Health)
	assert.True(t, c.valid("test_job", work.Token))
}

func TestCandidateControllerRevocationEffectsMatchWakeState(t *testing.T) {
	actions := map[string]func(*candidateController, DiscoveredJobInventory) (*candidateDelta, error){
		"disable": func(c *candidateController, _ DiscoveredJobInventory) (*candidateDelta, error) {
			return c.prepareDisable("test_job")
		},
		"remove": func(c *candidateController, _ DiscoveredJobInventory) (*candidateDelta, error) {
			return c.prepareRemove("test_job")
		},
		"fresh enable": func(c *candidateController, _ DiscoveredJobInventory) (*candidateDelta, error) {
			return c.prepareEnable("test_job", 1)
		},
		"exact passive": func(c *candidateController, _ DiscoveredJobInventory) (*candidateDelta, error) {
			return c.prepareExact(candidateTestConfig("override", "exact").SetSourceType(confgroup.TypeDyncfg), false, 1)
		},
		"exact disabled adoption": func(c *candidateController, _ DiscoveredJobInventory) (*candidateDelta, error) {
			return c.prepareAdoptExact(candidateTestConfig("override", "exact").SetSourceType(confgroup.TypeDyncfg), false, dyncfg.StatusDisabled, true, 0, 1)
		},
		"passive source replacement": func(c *candidateController, inv DiscoveredJobInventory) (*candidateDelta, error) {
			inv.Version++
			inv.AutoEnable = false
			inv.Candidates[0].Config = candidateTestConfig("a", "new")
			inv.Candidates[0].Incarnation++
			return c.prepareInventory(inv, 1)
		},
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			inv := candidateTestInventory(1, true, candidateTestConfig("a", "a"))
			work := candidateInventory(t, c, inv, 0).Decision.Work
			failed := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
			require.NotNil(t, failed.Decision.Wake)
			delta, err := action(c, inv)
			require.NoError(t, err)
			assert.Nil(t, delta.after.Wake)
			assert.Nil(t, delta.Decision.Wake, "revoked timer must not be rescheduled by Apply effects")
			applyCandidateDelta(t, delta, nil)
			assert.Equal(t, c.snapshot("test_job").Wake, delta.Decision.Wake)
		})
	}
}

func TestCandidateControllerObservedDynCfgAuthority(t *testing.T) {
	for name, auto := range map[string]bool{"passive": false, "automatic": true} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			override := candidateTestConfig("source-override", "override").SetSourceType(confgroup.TypeDyncfg)
			lower := candidateTestConfig("user", "lower")
			inv := candidateTestInventory(1, auto, lower, override)
			delta := candidateInventory(t, c, inv, 0)
			var work *candidateWork
			if auto {
				work = delta.Decision.Work
				require.NotNil(t, work)
				assert.Equal(t, override, work.Config)
				candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
			} else {
				require.NotNil(t, delta.Decision.Binding)
				assert.Equal(t, override, delta.Decision.Binding.Config)
				work = candidateEnable(t, c, 0).Decision.Work
			}
			require.NotNil(t, work)
			assert.Equal(t, candidateExactConfig, c.snapshot("test_job").Scope)
			failed := candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true}, 1)
			assert.Nil(t, failed.Decision.Work, "source-owned DynCfg cannot fall through to user alternatives")
			assert.Equal(t, override, c.snapshot("test_job").Binding.Config)
			remove, err := c.prepareRemove("test_job")
			applyCandidateDelta(t, remove, err)
			replay := candidateInventory(t, c, inv, 2)
			assert.True(t, replay.Decision.Ignored)
			assert.Nil(t, replay.Decision.Work)
			assert.Nil(t, c.snapshot("test_job").Binding)
			// An unrelated lower-source change still cannot restore unchanged DynCfg.
			inv.Version++
			extra := candidateTestConfig("extra", "extra")
			inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: extra, Source: "extra", Incarnation: 3})
			inv.Sources["extra"] = 1
			unchanged := candidateInventory(t, c, inv, 3)
			assert.Nil(t, unchanged.Decision.Work)
			assert.Nil(t, c.snapshot("test_job").Binding)
			inv.Version++
			var kept []DiscoveredJobCandidate
			for _, candidate := range inv.Candidates {
				if candidate.Source != override.Source() {
					kept = append(kept, candidate)
				}
			}
			inv.Candidates = kept
			delete(inv.Sources, override.Source())
			candidateInventory(t, c, inv, 4)
			assert.NotContains(t, c.snapshot("test_job").Entries, work.Token.Candidate)
		})
	}
}

func TestCandidateControllerPreparedOutcomeIsolation(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "a"), candidateTestConfig("b", "b")
	candidateInventory(t, c, candidateTestInventory(1, false, a, b), 0)
	work := candidateEnable(t, c, 0).Decision.Work
	before := c.snapshot("test_job")
	abandoned, err := c.prepareOutcome("test_job", work.Token, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 1)
	require.NoError(t, err)
	require.NotNil(t, abandoned.Decision.Work)
	assert.Equal(t, before, c.snapshot("test_job"), "preparation and abandonment cannot mutate committed ledger/header")
	require.LessOrEqual(t, len(abandoned.after.entryChanges), 2)
	assert.False(t, abandoned.after.entriesOwned)
	require.NotEmpty(t, before.Pass)
	assert.True(t, &abandoned.after.Pass[0] == &c.records["test_job"].Pass[0], "a frozen pass is shared immutable membership")
	accepted, err := c.prepareOutcome("test_job", work.Token, candidateOutcome{Kind: candidateAdmitted}, 1)
	applyCandidateDelta(t, accepted, err)
	committed := c.snapshot("test_job")
	assert.False(t, abandoned.apply())
	assert.Equal(t, committed, c.snapshot("test_job"), "stale Apply cannot merge its private entry changes")
}

func TestCandidateControllerConcurrentView(t *testing.T) {
	c := newCandidateController(1)
	configs := []confgroup.Config{candidateTestConfig("a", "a"), candidateTestConfig("b", "b")}
	candidateInventory(t, c, candidateTestInventory(1, false, configs...), 0)
	var readers sync.WaitGroup
	var invalid atomic.Bool
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				view := c.view("test_job")
				if view == nil {
					invalid.Store(true)
					return
				}
				if view.Active != nil && view.Active.Token.Intent != view.Intent {
					invalid.Store(true)
					return
				}
				if view.Binding != nil {
					view.Binding.Health = dyncfg.StatusDisabled
				}
				if view.Active != nil {
					view.Active.Token.Attempt = 0
				}
			}
		}()
	}
	for pass := 0; pass < 20; pass++ {
		work := candidateEnable(t, c, int64(pass)).Decision.Work
		for work != nil {
			work = candidateResult(t, c, work, candidateOutcome{Kind: candidateFailed, Listed: true}, int64(pass)).Decision.Work
		}
	}
	close(stop)
	readers.Wait()
	assert.False(t, invalid.Load())
	assert.Equal(t, dyncfg.StatusFailed, c.view("test_job").Binding.Health)
}

func candidateCostController(t testing.TB, n int) (*candidateController, *candidateRecord) {
	t.Helper()
	inv := DiscoveredJobInventory{FullName: "test_job", Version: 1, AutoEnable: true, Sources: map[string]uint64{"source": 1}}
	for i := 0; i < n; i++ {
		config := candidateTestConfig("source", fmt.Sprint(i))
		inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: config, Source: "source", Incarnation: uint64(i + 1)})
	}
	c := newCandidateController(1)
	delta, err := c.prepareInventory(inv, 0)
	if err != nil || !delta.apply() {
		t.Fatalf("prepare inventory: %v", err)
	}
	return c, c.snapshot("test_job")
}

func candidateCostFailurePass(t testing.TB, c *candidateController, initial *candidateRecord) {
	t.Helper()
	c.records["test_job"] = cloneCandidateRecord(initial)
	work := c.records["test_job"].Proposal
	for work != nil {
		delta, err := c.prepareOutcome("test_job", work.Token, candidateOutcome{Kind: candidateFailed, Listed: true}, 0)
		if err != nil || !delta.apply() {
			t.Fatalf("settle candidate: %v", err)
		}
		work = delta.Decision.Work
	}
}

func TestCandidateControllerFailurePassAllocationGrowth(t *testing.T) {
	// An eightfold candidate increase permits sorting and linear setup, but must
	// not restore the former quadratic cloning of the entire ledger per failure.
	cSmall, small := candidateCostController(t, 32)
	cLarge, large := candidateCostController(t, 256)
	smallAllocs := testing.AllocsPerRun(3, func() { candidateCostFailurePass(t, cSmall, small) })
	largeAllocs := testing.AllocsPerRun(3, func() { candidateCostFailurePass(t, cLarge, large) })
	assert.Less(t, largeAllocs, smallAllocs*12, "all-failure pass allocations must scale near linearly")
}

func BenchmarkCandidateControllerFailurePass(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c, initial := candidateCostController(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				candidateCostFailurePass(b, c, initial)
			}
		})
	}
}

func BenchmarkCandidateControllerView(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c, _ := candidateCostController(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if c.view("test_job") == nil {
					b.Fatal("missing view")
				}
			}
		})
	}
}

func TestCandidateControllerSourceProposalSurvivesPredecessorClosure(t *testing.T) {
	for name, readyBeforeProposal := range map[string]bool{"predecessor ready before proposal": true, "predecessor ready during proposal": false} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			a := candidateTestConfig("source", "a")
			inv := candidateTestInventory(1, true, a)
			initial := candidateInventory(t, c, inv, 0).Decision.Work
			candidateResult(t, c, initial, candidateOutcome{Kind: candidateAdmitted}, 0)
			if readyBeforeProposal {
				ready, err := c.prepareReady("test_job", initial.Token)
				applyCandidateDelta(t, ready, err)
			}
			b, third := candidateTestConfig("source", "b"), candidateTestConfig("third", "c")
			inv.Version = 2
			inv.Candidates = []DiscoveredJobCandidate{{Config: b, Source: "source", Incarnation: 2}, {Config: third, Source: "third", Incarnation: 3}}
			inv.Sources["third"] = 1
			proposal := candidateInventory(t, c, inv, 1).Decision.Work
			require.NotNil(t, proposal)
			assert.Equal(t, b, proposal.Config)
			if !readyBeforeProposal {
				ready, err := c.prepareReady("test_job", initial.Token)
				applyCandidateDelta(t, ready, err)
				assert.True(t, c.valid("test_job", proposal.Token), "readiness cannot revoke an authorized genuine source replacement")
			}
			terminal := candidateResult(t, c, initial, candidateOutcome{Kind: candidateFailed, Listed: true, AfterReady: true}, 2)
			assert.Nil(t, terminal.Decision.Work, "predecessor failure itself cannot open a sibling sweep")
			assert.True(t, c.snapshot("test_job").SearchClosed)
			assert.True(t, c.valid("test_job", proposal.Token), "terminal predecessor outcome cannot revoke the pending source replacement")
			failedReplacement := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateFailed, Listed: true}, 3)
			require.NotNil(t, failedReplacement.Decision.Work, "accepted source replacement opens its own initial candidate pass")
			assert.Equal(t, third, failedReplacement.Decision.Work.Config)
			assert.Equal(t, proposal.Token.Intent, failedReplacement.Decision.Work.Token.Intent)
			assert.False(t, c.snapshot("test_job").SearchClosed)
		})
	}
}

func TestCandidateControllerAdmittedInvalidRequiresManualRecovery(t *testing.T) {
	for name, exact := range map[string]bool{"exact accepted": true, "source accepted": false} {
		t.Run(name, func(t *testing.T) {
			c := newCandidateController(1)
			config := candidateTestConfig("source", "accepted")
			var work *candidateWork
			var inv DiscoveredJobInventory
			if exact {
				config.SetSourceType(confgroup.TypeDyncfg)
				delta, err := c.prepareAdoptExact(config, true, dyncfg.StatusAccepted, true, 0, 0)
				work = applyCandidateDelta(t, delta, err).Decision.Work
			} else {
				sibling := candidateTestConfig("sibling", "sibling")
				lower := candidateTestConfig("stock", "stock").SetSourceType(confgroup.TypeStock)
				inv = candidateTestInventory(1, true, config, sibling, lower)
				work = candidateInventory(t, c, inv, 0).Decision.Work
				candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
				config = work.Config
			}
			require.NotNil(t, work)
			failed := candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmittedInvalid, RetryAfter: 10}, 1)
			require.NotNil(t, failed.Decision.Binding)
			assert.Equal(t, config, failed.Decision.Binding.Config)
			assert.Equal(t, dyncfg.StatusFailed, failed.Decision.Binding.Health)
			assert.True(t, failed.Decision.Binding.Listed)
			assert.Nil(t, failed.Decision.Work)
			assert.Nil(t, failed.Decision.Wake)
			snapshot := c.snapshot("test_job")
			assert.Equal(t, candidateManualPreparationInvalid, snapshot.Manual)
			assert.Equal(t, candidateFailedManual, snapshot.Phase)
			assert.False(t, snapshot.FrameworkBlocked)
			assert.False(t, snapshot.SearchClosed)
			assert.False(t, c.valid("test_job", work.Token))
			if !exact {
				inv.Version++
				extra := candidateTestConfig("extra", "extra")
				inv.Candidates = append(inv.Candidates, DiscoveredJobCandidate{Config: extra, Source: "extra", Incarnation: 4})
				inv.Sources["extra"] = 1
				addition := candidateInventory(t, c, inv, 2)
				assert.Nil(t, addition.Decision.Work, "preparation invalidation is not a classified operational failure")
				assert.Equal(t, config, c.snapshot("test_job").Binding.Config)
			}
			enabled := candidateEnable(t, c, 3)
			require.NotNil(t, enabled.Decision.Work)
			assert.Equal(t, config, enabled.Decision.Work.Config)
			assert.NotEqual(t, work.Token.Intent, enabled.Decision.Work.Token.Intent)
			assert.Equal(t, dyncfg.StatusAccepted, enabled.Decision.Binding.Health)
			assert.Equal(t, candidateManualNone, c.snapshot("test_job").Manual)
		})
	}
}

func TestCandidateControllerAdmittedInvalidPreservesSourceSuccessor(t *testing.T) {
	c := newCandidateController(1)
	inv := candidateTestInventory(1, true, candidateTestConfig("source", "a"))
	original := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, original, candidateOutcome{Kind: candidateAdmitted}, 0)
	inv.Version = 2
	inv.Candidates[0].Config = candidateTestConfig("source", "replacement")
	inv.Candidates[0].Incarnation = 2
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	require.NotNil(t, proposal)
	failed := candidateResult(t, c, original, candidateOutcome{Kind: candidateAdmittedInvalid}, 2)
	assert.False(t, failed.Decision.Revoke)
	assert.Nil(t, failed.Decision.Work)
	assert.True(t, c.valid("test_job", proposal.Token))
	accepted := candidateResult(t, c, proposal, candidateOutcome{Kind: candidateAdmitted}, 3)
	require.NotNil(t, accepted.Decision.Binding)
	assert.Equal(t, proposal.Config, accepted.Decision.Binding.Config)
	assert.Equal(t, candidateManualNone, c.snapshot("test_job").Manual)
}

func TestCandidateControllerReadyPreparationBoundedCost(t *testing.T) {
	for name, terminal := range map[string]bool{"readiness": false, "terminal AfterReady": true} {
		t.Run(name, func(t *testing.T) {
			allocations := make(map[int]float64)
			for _, n := range []int{32, 256, 1024} {
				c, _ := candidateCostController(t, n)
				work := c.view("test_job").Proposal
				candidateResult(t, c, work, candidateOutcome{Kind: candidateAdmitted}, 0)
				allocations[n] = testing.AllocsPerRun(5, func() {
					var delta *candidateDelta
					var err error
					if terminal {
						delta, err = c.prepareOutcome("test_job", work.Token, candidateOutcome{Kind: candidateFailed, Listed: true, AfterReady: true}, 0)
					} else {
						delta, err = c.prepareReady("test_job", work.Token)
					}
					if err != nil {
						t.Fatalf("prepare settlement: %v", err)
					}
					if len(delta.after.entryChanges) > 1 || delta.after.entriesOwned {
						t.Fatalf("settlement prepared population changes for %d candidates", n)
					}
				})
			}
			assert.LessOrEqual(t, allocations[1024], allocations[32]+2, "live settlement must have constant allocation cost")
			assert.LessOrEqual(t, allocations[256], allocations[32]+2)
			t.Logf("allocations at 32/256/1024 candidates: %.0f/%.0f/%.0f", allocations[32], allocations[256], allocations[1024])
		})
	}
}

func TestCandidateControllerReadinessRevokesHistoricalRetainedGrants(t *testing.T) {
	c := newCandidateController(1)
	a, b := candidateTestConfig("a", "old"), candidateTestConfig("b", "b")
	inv := candidateTestInventory(1, true, a, b)
	first := candidateInventory(t, c, inv, 0).Decision.Work
	candidateResult(t, c, first, candidateOutcome{Kind: candidateAdmitted}, 0)
	inv.Version = 2
	for i := range inv.Candidates {
		if inv.Candidates[i].Source == first.Config.Source() {
			inv.Candidates[i].Config = candidateTestConfig(first.Config.Source(), "invalid replacement")
			inv.Candidates[i].Incarnation = 7
		}
	}
	proposal := candidateInventory(t, c, inv, 1).Decision.Work
	candidateResult(t, c, proposal, candidateOutcome{Kind: candidateRejected}, 1)
	sibling := candidateResult(t, c, first, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 1).Decision.Work
	require.NotNil(t, sibling)
	candidateResult(t, c, sibling, candidateOutcome{Kind: candidateAdmitted}, 1)
	assert.True(t, candidateAuthorized(c.snapshot("test_job"), first.Token.Candidate))
	ready, err := c.prepareReady("test_job", sibling.Token)
	applyCandidateDelta(t, ready, err)
	closed := c.snapshot("test_job")
	assert.False(t, candidateAuthorized(closed, first.Token.Candidate), "readiness immediately revokes historical sibling recovery without scanning")
	assert.True(t, candidateAuthorized(closed, sibling.Token.Candidate))
	// A rejected replacement of the selected source still retains that selected
	// admission. The global readiness cut must not revoke its incumbent exception.
	inv.Version = 3
	var invalidKey candidateKey
	for i := range inv.Candidates {
		if inv.Candidates[i].Source == sibling.Config.Source() {
			inv.Candidates[i].Config = candidateTestConfig(sibling.Config.Source(), "invalid selected replacement")
			inv.Candidates[i].Incarnation = 8
			invalidKey = candidateIdentity(inv.Candidates[i])
		}
	}
	replacement, err := c.prepareInventoryValidated(inv, map[candidateKey]bool{invalidKey: true}, 2)
	applyCandidateDelta(t, replacement, err)
	assert.True(t, c.valid("test_job", sibling.Token))
	disable, err := c.prepareDisable("test_job")
	applyCandidateDelta(t, disable, err)
	enabled := candidateEnable(t, c, 3)
	require.NotNil(t, enabled.Decision.Work)
	assert.Equal(t, sibling.Token.Candidate, enabled.Decision.Work.Token.Candidate)
	assert.NotContains(t, c.snapshot("test_job").Entries, first.Token.Candidate, "fresh ENABLE prunes expired sibling grants off-loop")
	failed := candidateResult(t, c, enabled.Decision.Work, candidateOutcome{Kind: candidateFailed, Listed: true}, 4)
	assert.Nil(t, failed.Decision.Work)
	assert.Nil(t, failed.Decision.Wake)
}
