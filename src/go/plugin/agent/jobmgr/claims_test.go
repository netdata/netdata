// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimProbe records a request's grant lifecycle for assertions.
type claimProbe struct {
	label  string
	grants int
	grant  *claimGrant
}

func (p *claimProbe) granted() bool { return p.grants > 0 }

func (p *claimProbe) request(compute func() []claim) *claimRequest {
	return &claimRequest{
		label:   p.label,
		compute: compute,
		granted: func(g *claimGrant) {
			p.grants++
			p.grant = g
		},
	}
}

func staticClaims(claims ...claim) func() []claim {
	return func() []claim { return claims }
}

func heldKeys(g *claimGrant) []string {
	keys := make([]string, 0, len(g.held))
	for _, c := range g.held {
		keys = append(keys, c.key)
	}
	slices.Sort(keys)
	return keys
}

// TestClaimTable_BothOrdersNoDeadlock is the binding overlap scenario: a
// collector command (write on its job key, read on a store) against a store
// update (write on the store, write on that collector's job key), staggered
// through a pre-existing store holder so both end up mid-acquisition, in
// BOTH arrival orders and BOTH lexicographic key directions. Single global
// acquisition order must resolve every variant; a primary-first
// implementation deadlocks the second-arrival variants (one command holding
// its primary while waiting for the other's).
func TestClaimTable_BothOrdersNoDeadlock(t *testing.T) {
	tests := map[string]struct {
		jobKey, storeKey string
		collectorFirst   bool
	}{
		"job key sorts first, collector arrives first":   {jobKey: "a|job", storeKey: "s|store", collectorFirst: true},
		"job key sorts first, store arrives first":       {jobKey: "a|job", storeKey: "s|store", collectorFirst: false},
		"store key sorts first, collector arrives first": {jobKey: "t|job", storeKey: "s|store", collectorFirst: true},
		"store key sorts first, store arrives first":     {jobKey: "t|job", storeKey: "s|store", collectorFirst: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ct := newClaimTable()

			blocker := &claimProbe{label: "blocker"}
			ct.acquire(blocker.request(staticClaims(claim{tc.storeKey, claimWrite})))
			require.True(t, blocker.granted(), "the blocker must acquire the free store immediately")

			collector := &claimProbe{label: "collector"}
			store := &claimProbe{label: "store-update"}
			collectorReq := collector.request(staticClaims(
				claim{tc.jobKey, claimWrite},  // primary
				claim{tc.storeKey, claimRead}, // referenced store
			))
			storeReq := store.request(staticClaims(
				claim{tc.storeKey, claimWrite}, // primary
				claim{tc.jobKey, claimWrite},   // dependent job restart
			))

			first, second := collector, store
			if tc.collectorFirst {
				ct.acquire(collectorReq)
				ct.acquire(storeReq)
			} else {
				ct.acquire(storeReq)
				ct.acquire(collectorReq)
				first, second = store, collector
			}

			assert.False(t, collector.granted(), "the collector command must park behind the blocked store")
			assert.False(t, store.granted(), "the store update must park behind the blocked store")

			ct.release(blocker.grant)
			require.True(t, first.granted(),
				"releasing the blocker must grant the first arrival (%s)", first.label)
			require.False(t, second.granted(),
				"the second arrival (%s) must wait for the first's conflicting holds", second.label)

			ct.release(first.grant)
			require.True(t, second.granted(),
				"releasing the first grant must grant the second arrival (%s) - a deadlock here means acquisition is not a single global order", second.label)

			ct.release(second.grant)
			assert.Equal(t, 0, ct.parkedCount(), "nothing may stay parked after all grants release")
		})
	}
}

// TestClaimTable_OrderedAcquisitionProperties is a randomized sweep:
// many requests with random claim sets acquired and released in random
// interleavings. Invariants: reader/writer exclusion holds at every grant,
// every request is granted exactly once, and nothing stays parked once all
// grants are released.
func TestClaimTable_OrderedAcquisitionProperties(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			ct := newClaimTable()

			keys := make([]string, 10)
			for i := range keys {
				keys[i] = fmt.Sprintf("k%02d", i)
			}

			type holdState struct{ readers, writers int }
			holds := map[string]*holdState{}
			holdOf := func(key string) *holdState {
				if holds[key] == nil {
					holds[key] = &holdState{}
				}
				return holds[key]
			}

			const requests = 40
			var outstanding []*claimGrant
			grantsSeen := 0

			newRequest := func(i int) *claimRequest {
				n := 1 + rng.Intn(4)
				perm := rng.Perm(len(keys))[:n]
				set := make([]claim, 0, n)
				for _, ki := range perm {
					mode := claimRead
					if rng.Intn(2) == 0 {
						mode = claimWrite
					}
					set = append(set, claim{keys[ki], mode})
				}
				req := &claimRequest{
					label:   fmt.Sprintf("req-%d", i),
					compute: staticClaims(set...),
				}
				req.granted = func(g *claimGrant) {
					grantsSeen++
					for _, c := range g.held {
						hs := holdOf(c.key)
						switch c.mode {
						case claimWrite:
							require.Zero(t, hs.readers, "write grant on %s with readers held", c.key)
							require.Zero(t, hs.writers, "write grant on %s with a writer held", c.key)
							hs.writers++
						case claimRead:
							require.Zero(t, hs.writers, "read grant on %s with a writer held", c.key)
							hs.readers++
						}
					}
					outstanding = append(outstanding, g)
				}
				return req
			}

			releaseRandom := func() {
				i := rng.Intn(len(outstanding))
				g := outstanding[i]
				outstanding = slices.Delete(outstanding, i, i+1)
				for _, c := range g.held {
					hs := holdOf(c.key)
					if c.mode == claimWrite {
						hs.writers--
					} else {
						hs.readers--
					}
				}
				ct.release(g)
			}

			for i := range requests {
				ct.acquire(newRequest(i))
				// Release with 50% probability per outstanding grant to keep
				// a healthy mix of holds and parks.
				for len(outstanding) > 0 && rng.Intn(2) == 0 {
					releaseRandom()
				}
			}
			for len(outstanding) > 0 {
				releaseRandom()
			}

			assert.Equal(t, requests, grantsSeen, "every request must be granted exactly once")
			assert.Equal(t, 0, ct.parkedCount(), "nothing may stay parked after all releases")
		})
	}
}

// TestClaimTable_WaiterFIFOPreventsWriterStarvation pins fair per-key
// queuing: a later reader parks BEHIND an already-parked writer instead of
// joining the current readers, so a stream of readers can never starve a
// writer.
func TestClaimTable_WaiterFIFOPreventsWriterStarvation(t *testing.T) {
	ct := newClaimTable()
	const k = "k"

	reader1 := &claimProbe{label: "reader-1"}
	ct.acquire(reader1.request(staticClaims(claim{k, claimRead})))
	require.True(t, reader1.granted())

	writer := &claimProbe{label: "writer"}
	ct.acquire(writer.request(staticClaims(claim{k, claimWrite})))
	require.False(t, writer.granted(), "the writer must park behind the held read claim")

	reader2 := &claimProbe{label: "reader-2"}
	ct.acquire(reader2.request(staticClaims(claim{k, claimRead})))
	require.False(t, reader2.granted(),
		"a later reader must park behind the waiting writer, not join the current readers")

	ct.release(reader1.grant)
	require.True(t, writer.granted(), "the writer is first in the FIFO")
	require.False(t, reader2.granted(), "the reader waits out the writer's exclusive hold")

	ct.release(writer.grant)
	require.True(t, reader2.granted())
	ct.release(reader2.grant)
	assert.Equal(t, 0, ct.parkedCount())
}

// TestClaimTable_ReleasePumpsWaitersBeforeReleasedHook pins the lane/claim
// ordering seam: executor lane FIFO work parked behind a foreign write hold
// must not settle until same-key claim waiters had the first chance to acquire
// the key. Otherwise a later status no-op could run before an older store
// update waiter gets its dependent write claim.
func TestClaimTable_ReleasePumpsWaitersBeforeReleasedHook(t *testing.T) {
	ct := newClaimTable()
	var released []string
	ct.released = func(key string) {
		released = append(released, key)
	}

	holder := &claimProbe{label: "holder"}
	ct.acquire(holder.request(staticClaims(claim{"job", claimWrite})))
	require.True(t, holder.granted())

	waiter := &claimProbe{label: "store-update"}
	ct.acquire(waiter.request(staticClaims(claim{"job", claimWrite})))
	require.False(t, waiter.granted(), "the store update must park behind the held job key")

	ct.release(holder.grant)
	require.True(t, waiter.granted(), "the claim waiter must acquire before lane FIFO settlement")
	assert.Empty(t, released, "the executor release hook must not run while the waiter now holds the key")

	ct.release(waiter.grant)
	assert.Equal(t, []string{"job"}, released)
}

func TestClaimTable_ReleasedHookWaitsForGrantedReaders(t *testing.T) {
	ct := newClaimTable()
	var released []string
	ct.released = func(key string) {
		released = append(released, key)
	}

	holder := &claimProbe{label: "holder"}
	ct.acquire(holder.request(staticClaims(claim{"store", claimWrite})))
	require.True(t, holder.granted())

	reader := &claimProbe{label: "collector"}
	ct.acquire(reader.request(staticClaims(claim{"store", claimRead})))
	require.False(t, reader.granted(), "the reader must park behind the writer")

	ct.release(holder.grant)
	require.True(t, reader.granted(), "the reader must acquire before release hooks run")
	assert.Empty(t, released, "the release hook must not run while a reader holds the key")

	ct.release(reader.grant)
	assert.Equal(t, []string{"store"}, released)
}

// TestClaimTable_WakeReattemptsWaitersWithoutRelease pins the wedge wake
// seam: waiters parked at a wedged key must recompute without a holder change.
// A waiter whose recomputed set drops the wedged key proceeds; one that still
// needs it re-parks at the FIFO front.
func TestClaimTable_WakeReattemptsWaitersWithoutRelease(t *testing.T) {
	ct := newClaimTable()

	holder := &claimProbe{label: "wedged-holder"}
	ct.acquire(holder.request(staticClaims(claim{"wedged", claimWrite})))
	require.True(t, holder.granted())

	waiterSet := []claim{{"wedged", claimWrite}}
	waiter := &claimProbe{label: "store-update"}
	ct.acquire(waiter.request(func() []claim { return waiterSet }))
	require.False(t, waiter.granted(), "the waiter starts parked at the wedged key")

	stillNeedsWedged := &claimProbe{label: "still-needs-wedged"}
	ct.acquire(stillNeedsWedged.request(staticClaims(claim{"wedged", claimWrite})))
	require.False(t, stillNeedsWedged.granted(), "a later waiter parks behind the first")
	require.Equal(t, 2, ct.parkedCount())

	waiterSet = []claim{{"other", claimWrite}}
	ct.wake("wedged")

	require.True(t, waiter.granted(), "wake must reattempt the head and grant after recompute drops the wedged key")
	assert.Equal(t, []string{"other"}, heldKeys(waiter.grant))
	require.False(t, stillNeedsWedged.granted(), "the next waiter still needs the wedged key and must re-park")
	assert.Equal(t, 1, ct.parkedCount())

	ct.release(holder.grant)
	require.True(t, stillNeedsWedged.granted(), "the re-parked waiter must keep FIFO position when the key releases")

	ct.release(waiter.grant)
	ct.release(stillNeedsWedged.grant)
	assert.Equal(t, 0, ct.parkedCount())
}

// TestClaimTable_RecomputeAtRestage pins the stale-set rule: a parked
// acquisition recomputes its claim set when it wakes; a changed set releases
// EVERYTHING held and reacquires under the new set, so keys dropped from the
// set free immediately and newly-added dependencies are respected.
func TestClaimTable_RecomputeAtRestage(t *testing.T) {
	t.Run("changed set releases the held prefix and reacquires", func(t *testing.T) {
		ct := newClaimTable()

		blocker := &claimProbe{label: "blocker"}
		ct.acquire(blocker.request(staticClaims(claim{"k3", claimWrite})))
		require.True(t, blocker.granted())

		set := []claim{{"k1", claimWrite}, {"k3", claimWrite}}
		parked := &claimProbe{label: "parked"}
		ct.acquire(parked.request(func() []claim { return set }))
		require.False(t, parked.granted(), "the acquisition must park at the blocked k3 holding k1")

		probe := &claimProbe{label: "k1-probe"}
		ct.acquire(probe.request(staticClaims(claim{"k1", claimWrite})))
		require.False(t, probe.granted(), "k1 must be held by the parked acquisition's prefix")

		// The dependency set changes while the acquisition is parked (the
		// stale-credential class: another command's commit altered it).
		set = []claim{{"k2", claimWrite}, {"k3", claimWrite}}

		ct.release(blocker.grant)
		require.True(t, parked.granted(), "the woken acquisition must complete under the new set")
		assert.Equal(t, []string{"k2", "k3"}, heldKeys(parked.grant),
			"the grant must reflect the RECOMPUTED set, not the stale one")
		require.True(t, probe.granted(),
			"k1 was dropped from the set - the stale prefix hold must have been released")

		ct.release(parked.grant)
		ct.release(probe.grant)
		assert.Equal(t, 0, ct.parkedCount())
	})

	t.Run("unchanged set keeps the held prefix", func(t *testing.T) {
		ct := newClaimTable()

		blocker := &claimProbe{label: "blocker"}
		ct.acquire(blocker.request(staticClaims(claim{"k3", claimWrite})))
		require.True(t, blocker.granted())

		parked := &claimProbe{label: "parked"}
		ct.acquire(parked.request(staticClaims(claim{"k1", claimWrite}, claim{"k3", claimWrite})))
		require.False(t, parked.granted())

		probe := &claimProbe{label: "k1-probe"}
		ct.acquire(probe.request(staticClaims(claim{"k1", claimWrite})))
		require.False(t, probe.granted())

		ct.release(blocker.grant)
		require.True(t, parked.granted())
		assert.Equal(t, []string{"k1", "k3"}, heldKeys(parked.grant))
		require.False(t, probe.granted(), "k1 stays held until the grant releases")

		ct.release(parked.grant)
		require.True(t, probe.granted())
		ct.release(probe.grant)
	})
}

// TestClaimTable_ClaimSetNormalization pins that duplicate keys collapse to
// the strongest mode (union claim sets legitimately produce the same key as
// both read and write).
func TestClaimTable_ClaimSetNormalization(t *testing.T) {
	ct := newClaimTable()

	p := &claimProbe{label: "dup"}
	ct.acquire(p.request(staticClaims(
		claim{"k", claimRead},
		claim{"k", claimWrite},
		claim{"k", claimRead},
	)))
	require.True(t, p.granted())
	require.Len(t, p.grant.held, 1, "duplicate keys must collapse to one claim")
	assert.Equal(t, claimWrite, p.grant.held[0].mode, "write is the strongest mode")

	reader := &claimProbe{label: "reader"}
	ct.acquire(reader.request(staticClaims(claim{"k", claimRead})))
	assert.False(t, reader.granted(), "the collapsed claim must be exclusive")

	ct.release(p.grant)
	assert.True(t, reader.granted())
	ct.release(reader.grant)
}

// TestClaimTable_EmptySetGrantsImmediately pins the degenerate case: a
// command with no claims runs immediately and its release is a no-op.
func TestClaimTable_EmptySetGrantsImmediately(t *testing.T) {
	ct := newClaimTable()
	p := &claimProbe{label: "empty"}
	ct.acquire(p.request(staticClaims()))
	require.True(t, p.granted())
	ct.release(p.grant)
	assert.Equal(t, 0, ct.parkedCount())
}

// TestClaimTable_FairnessSweep is the N-parked-keys starvation property:
// many contenders with random modes and multi-key sets, all overlapping on
// one shared key, released in random order - grants on the shared key must
// follow arrival order exactly (strict per-key FIFO admits no barging in
// either direction), and nobody starves.
func TestClaimTable_FairnessSweep(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			ct := newClaimTable()

			const shared = "kshared"
			const contenders = 25

			// A pre-existing holder guarantees every contender parks, so the
			// recorded grant order is decided by the waiter FIFO alone.
			blocker := &claimProbe{label: "blocker"}
			ct.acquire(blocker.request(staticClaims(claim{shared, claimWrite})))
			require.True(t, blocker.granted())

			var grantOrder []int
			var outstanding []*claimGrant
			for i := range contenders {
				mode := claimRead
				if rng.Intn(2) == 0 {
					mode = claimWrite
				}
				set := []claim{{shared, mode}}
				if rng.Intn(2) == 0 {
					set = append(set, claim{fmt.Sprintf("own-%02d", i), claimWrite})
				}
				req := &claimRequest{
					label:   fmt.Sprintf("contender-%d", i),
					compute: staticClaims(set...),
				}
				req.granted = func(g *claimGrant) {
					grantOrder = append(grantOrder, i)
					outstanding = append(outstanding, g)
				}
				ct.acquire(req)
			}
			require.Empty(t, grantOrder, "every contender must park behind the blocker")

			ct.release(blocker.grant)
			for len(outstanding) > 0 {
				i := rng.Intn(len(outstanding))
				g := outstanding[i]
				outstanding = slices.Delete(outstanding, i, i+1)
				ct.release(g)
			}

			require.Len(t, grantOrder, contenders, "nobody may starve")
			assert.True(t, slices.IsSorted(grantOrder),
				"grants on the shared key must follow arrival order, got %v", grantOrder)
			assert.Equal(t, 0, ct.parkedCount())
		})
	}
}

// TestClaimTable_DrainParked pins the shutdown path: draining fails every
// parked acquisition through the supplied callback, releases their held
// prefixes, and leaves the table empty; granted never fires for them.
func TestClaimTable_DrainParked(t *testing.T) {
	ct := newClaimTable()

	holder := &claimProbe{label: "holder"}
	ct.acquire(holder.request(staticClaims(claim{"k2", claimWrite})))
	require.True(t, holder.granted())

	parked := &claimProbe{label: "parked"}
	ct.acquire(parked.request(staticClaims(claim{"k1", claimWrite}, claim{"k2", claimWrite})))
	require.False(t, parked.granted())

	probe := &claimProbe{label: "k1-probe"}
	ct.acquire(probe.request(staticClaims(claim{"k1", claimWrite})))
	require.False(t, probe.granted(), "k1 must be held by the parked acquisition's prefix")

	var drained []string
	ct.drainParked(func(req *claimRequest) { drained = append(drained, req.label) })

	assert.ElementsMatch(t, []string{"parked", "k1-probe"}, drained,
		"every parked acquisition must be reported to the drain callback")
	assert.False(t, parked.granted(), "a drained acquisition is never granted")
	assert.Equal(t, 0, ct.parkedCount())

	free := &claimProbe{label: "k1-after-drain"}
	ct.acquire(free.request(staticClaims(claim{"k1", claimWrite})))
	assert.True(t, free.granted(), "the drained acquisition's prefix holds must be released")

	ct.release(holder.grant)
	ct.release(free.grant)
}

// TestClaimTable_ParkedMetrics pins the aging signal: parkedCount and
// oldestParkedSince track park/grant transitions.
func TestClaimTable_ParkedMetrics(t *testing.T) {
	ct := newClaimTable()

	_, ok := ct.oldestParkedSince()
	assert.False(t, ok, "no parked acquisitions yet")

	holder := &claimProbe{label: "holder"}
	ct.acquire(holder.request(staticClaims(claim{"k", claimWrite})))
	require.True(t, holder.granted())
	assert.Equal(t, 0, ct.parkedCount())

	waiter1 := &claimProbe{label: "waiter-1"}
	ct.acquire(waiter1.request(staticClaims(claim{"k", claimWrite})))
	waiter2 := &claimProbe{label: "waiter-2"}
	ct.acquire(waiter2.request(staticClaims(claim{"k", claimWrite})))
	assert.Equal(t, 2, ct.parkedCount())
	since, ok := ct.oldestParkedSince()
	require.True(t, ok)
	assert.False(t, since.IsZero())

	ct.release(holder.grant)
	require.True(t, waiter1.granted())
	assert.Equal(t, 1, ct.parkedCount(), "waiter-2 stays parked behind waiter-1's hold")

	ct.release(waiter1.grant)
	require.True(t, waiter2.granted())
	assert.Equal(t, 0, ct.parkedCount())
	_, ok = ct.oldestParkedSince()
	assert.False(t, ok)
	ct.release(waiter2.grant)
}
