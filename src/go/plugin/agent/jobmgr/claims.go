// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"cmp"
	"slices"
	"time"
)

// The claim table is the loop-owned multi-key reservation layer over the
// executor's namespaced keys. A command that touches keys beyond its own
// lane (a store update restarting dependent jobs, a collector command
// resolving secrets from a store, a command bound to a vnode) reserves ALL
// of them before its blocking work ships, under these rules:
//
//   - Read claims share a key; a write claim is exclusive. Read and write
//     conflict - the later arrival parks.
//   - ALL of a command's keys - including its primary - are acquired in ONE
//     global lexicographic key order, waiting at each. A single total order
//     makes circular waits structurally impossible; grabbing the primary
//     first and then the rest would deadlock two overlapping multi-key
//     commands.
//   - Waiters at a key are strictly FIFO: a new acquisition parks behind
//     existing waiters even when it would be compatible with the current
//     holders, so writers can never starve behind a stream of readers.
//   - The claim set is recomputed at EVERY (re-)stage attempt: when a parked
//     acquisition wakes, its dependency set may have changed while another
//     command's effect was in flight (stage-time sets go stale). If the set
//     changed, everything held is released and the walk restarts under the
//     new set - a parked command never acts on a stale dependency set.
//   - An acquisition that cannot proceed holds only the lexicographic
//     prefix it already acquired and parks at the blocking key; commands
//     that have not begun acquiring hold nothing.
//
// The table is loop-owned state: every method must be called on the run-loop
// goroutine. Grant callbacks run synchronously on the caller (acquire or the
// release that unblocked them). The table is the single source of key
// exclusivity: every command occupying a lane holds a claim on its own key
// here, so no out-of-band occupancy veto exists.

// claimMode distinguishes shared readers from exclusive writers.
type claimMode int8

const (
	claimRead claimMode = iota + 1
	claimWrite
)

// claim is one key of a command's reservation set. Keys use the executor's
// domain-namespaced form (event.stateKey) so keys from different domains can
// never collide.
type claim struct {
	key  string
	mode claimMode
}

// claimRequest is one command's multi-key reservation.
type claimRequest struct {
	// label identifies the command in logs and metrics.
	label string
	// compute returns the current claim set. It is called at the initial
	// attempt and again at every unpark; duplicate keys are normalized to
	// the strongest mode.
	compute func() []claim
	// granted runs synchronously once every claim is held. The command's
	// blocking work may then ship; the grant must be released at commit.
	granted func(*claimGrant)
}

// claimGrant is a completed acquisition: proof that all claims of the last
// computed set are held, and the handle to release them.
type claimGrant struct {
	held  []claim
	owner *claimAcquisition
}

// claimTable tracks holders and FIFO waiters per key.
type claimTable struct {
	keys map[string]*claimKeyState
	// released, when set, runs after a key loses its last holder so the
	// owner can settle work parked behind the claim.
	released func(key string)

	// wakeQueue defers waiter processing to a single flat pump at the
	// outermost entry point, so releases performed inside an attempt (a
	// recomputed set dropping keys) never recurse.
	wakeQueue     []string
	releasedQueue []string
	pumping       bool
	parked        int
}

type claimKeyState struct {
	writer  *claimAcquisition
	readers map[*claimAcquisition]struct{}
	waiters []*claimAcquisition
}

func (ks *claimKeyState) unused() bool {
	return ks.writer == nil && len(ks.readers) == 0 && len(ks.waiters) == 0
}

// claimAcquisition is one in-progress ordered walk over a normalized set.
type claimAcquisition struct {
	req      *claimRequest
	set      []claim // sorted by key, deduped to the strongest mode
	next     int     // index into set of the key being acquired
	parkedAt time.Time
}

func newClaimTable() *claimTable {
	return &claimTable{keys: make(map[string]*claimKeyState)}
}

// normalizeClaims sorts by key and collapses duplicates to the strongest
// mode, giving every acquisition the same global walk order.
func normalizeClaims(set []claim) []claim {
	if len(set) == 0 {
		return nil
	}
	modes := make(map[string]claimMode, len(set))
	for _, c := range set {
		if c.mode == claimWrite || modes[c.key] == 0 {
			modes[c.key] = max(modes[c.key], c.mode)
		}
	}
	out := make([]claim, 0, len(modes))
	for key, mode := range modes {
		out = append(out, claim{key: key, mode: mode})
	}
	slices.SortFunc(out, func(a, b claim) int { return cmp.Compare(a.key, b.key) })
	return out
}

// acquire starts an ordered acquisition for req. When every key is free it
// completes synchronously (req.granted runs before acquire returns);
// otherwise the acquisition holds its lexicographic prefix and parks at the
// blocking key until releases resume it.
func (ct *claimTable) acquire(req *claimRequest) {
	ct.attempt(&claimAcquisition{req: req}, "")
	ct.pump()
}

// attempt (re)runs an acquisition: recompute the set, release everything on
// a set change, then walk the remaining keys in order. When the walk blocks
// at reparkFront, the acquisition re-enters that queue at the FRONT (it was
// just popped as its head and must not lose its position).
func (ct *claimTable) attempt(a *claimAcquisition, reparkFront string) {
	set := normalizeClaims(a.req.compute())
	if !slices.Equal(set, a.set) {
		ct.releaseHeld(a)
		a.set = set
		a.next = 0
	}
	ct.walk(a, reparkFront)
}

// walk acquires a.set[a.next:] in order, parking at the first unavailable
// key; on full acquisition the request is granted.
func (ct *claimTable) walk(a *claimAcquisition, reparkFront string) {
	for a.next < len(a.set) {
		c := a.set[a.next]
		ks := ct.keys[c.key]
		if ks == nil {
			ks = &claimKeyState{}
			ct.keys[c.key] = ks
		}
		// FIFO fairness: park behind existing waiters even when compatible
		// with the current holders - except at the key this acquisition was
		// just woken from (any waiters there arrived behind it).
		barged := len(ks.waiters) > 0 && c.key != reparkFront
		if barged || !ct.canHold(ks, c) {
			a.parkedAt = time.Now()
			ct.parked++
			if c.key == reparkFront {
				ks.waiters = append([]*claimAcquisition{a}, ks.waiters...)
			} else {
				ks.waiters = append(ks.waiters, a)
			}
			return
		}
		if c.mode == claimWrite {
			ks.writer = a
		} else {
			if ks.readers == nil {
				ks.readers = make(map[*claimAcquisition]struct{})
			}
			ks.readers[a] = struct{}{}
		}
		a.next++
	}
	a.parkedAt = time.Time{}
	a.req.granted(&claimGrant{held: a.set, owner: a})
}

func (ct *claimTable) canHold(ks *claimKeyState, c claim) bool {
	if ks.writer != nil {
		return false
	}
	if c.mode == claimWrite && len(ks.readers) > 0 {
		return false
	}
	return true
}

// release frees every key of a completed grant and resumes parked
// acquisitions that can now proceed (their granted callbacks may run
// synchronously inside this call).
func (ct *claimTable) release(g *claimGrant) {
	if g == nil || g.owner == nil {
		return
	}
	a := g.owner
	g.owner = nil
	a.next = len(a.set)
	ct.releaseHeld(a)
	ct.pump()
}

// releaseReadClaims frees the grant's READ claims and keeps its writes.
// Used at a deadline-abandon commit: the command has committed its outcome,
// so its read reservations (referenced stores, vnodes - keys its leaked
// work only LOOKED at) must not outlive it. Its write claims - the
// command's own key and any keys the leaked work may still MUTATE, such as
// a store command's dependent job keys - stay held until the late return.
func (ct *claimTable) releaseReadClaims(g *claimGrant) {
	if g == nil || g.owner == nil {
		return
	}
	a := g.owner
	var kept []claim
	for _, c := range a.set {
		if c.mode == claimWrite {
			kept = append(kept, c)
			continue
		}
		ks := ct.keys[c.key]
		if ks == nil {
			continue
		}
		delete(ks.readers, a)
		ct.wakeQueue = append(ct.wakeQueue, c.key)
		if ks.writer == nil && len(ks.readers) == 0 {
			ct.releasedQueue = append(ct.releasedQueue, c.key)
		}
	}
	a.set = kept
	a.next = len(kept)
	g.held = kept
	ct.pump()
}

// releaseHeld drops the acquisition's held prefix (set[:next]) and queues
// wakes for the freed keys.
func (ct *claimTable) releaseHeld(a *claimAcquisition) {
	for i := range a.next {
		c := a.set[i]
		ks := ct.keys[c.key]
		if ks == nil {
			continue
		}
		if c.mode == claimWrite {
			if ks.writer == a {
				ks.writer = nil
			}
		} else {
			delete(ks.readers, a)
		}
		ct.wakeQueue = append(ct.wakeQueue, c.key)
		if ks.writer == nil && len(ks.readers) == 0 {
			ct.releasedQueue = append(ct.releasedQueue, c.key)
		}
	}
	a.next = 0
}

// heldForWrite reports whether an acquisition holds the key EXCLUSIVELY.
// Lane owners park events for a write-held key until the release hook
// settles them; read holds never park lane events - read-shaped inline
// commands share safely, and claim-acquiring events arbitrate their modes in
// the table itself (read/read shares, write parks fairly in the FIFO).
func (ct *claimTable) heldForWrite(key string) bool {
	ks := ct.keys[key]
	return ks != nil && ks.writer != nil
}

// wake re-attempts the FIFO waiters parked at key even though its holders
// did not change. Used when a key WEDGES: its retained write claim now has
// no bounded release, and waiters whose recomputed sets exclude wedged keys
// (store mutations skip-and-report wedged dependents) must proceed instead
// of waiting out the leaked call. A waiter whose recomputed set still needs
// the key simply re-parks at the front.
func (ct *claimTable) wake(key string) {
	ct.wakeQueue = append(ct.wakeQueue, key)
	ct.pump()
}

// pump processes deferred wakes until quiescence. Only the outermost caller
// pumps; attempts running inside it queue further wakes instead of
// recursing.
func (ct *claimTable) pump() {
	if ct.pumping {
		return
	}
	ct.pumping = true
	defer func() { ct.pumping = false }()
	for {
		for len(ct.wakeQueue) > 0 {
			key := ct.wakeQueue[0]
			ct.wakeQueue = ct.wakeQueue[1:]
			ct.processWaiters(key)
		}
		if len(ct.releasedQueue) == 0 {
			return
		}
		keys := ct.releasedQueue
		ct.releasedQueue = nil
		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			ct.notifyReleased(key)
		}
	}
}

func (ct *claimTable) notifyReleased(key string) {
	ks := ct.keys[key]
	if ks != nil && (ks.writer != nil || len(ks.readers) > 0 || len(ks.waiters) > 0) {
		return
	}
	if ct.released != nil {
		ct.released(key)
	}
	if ks != nil && ks.unused() {
		delete(ct.keys, key)
	}
}

// processWaiters resumes the FIFO head of a key's waiter queue until the
// head cannot proceed (it re-parked at this key) or the queue empties.
func (ct *claimTable) processWaiters(key string) {
	for {
		ks := ct.keys[key]
		if ks == nil || len(ks.waiters) == 0 {
			return
		}
		head := ks.waiters[0]
		ks.waiters = ks.waiters[1:]
		ct.parked--
		head.parkedAt = time.Time{}
		ct.attempt(head, key)
		ksNow := ct.keys[key]
		if ksNow != nil && len(ksNow.waiters) > 0 && ksNow.waiters[0] == head {
			// The head is still blocked here: everyone behind it stays
			// queued (strict FIFO, no barging past a blocked head).
			return
		}
	}
}

// drainParked fails every parked acquisition: each is reported through fail,
// its held prefix is released, and it will never be granted. Used by the
// shutdown drain so parked commands reach a terminal outcome.
func (ct *claimTable) drainParked(fail func(*claimRequest)) {
	var drained []*claimAcquisition
	for _, ks := range ct.keys {
		drained = append(drained, ks.waiters...)
		ct.parked -= len(ks.waiters)
		ks.waiters = nil
	}
	for _, a := range drained {
		ct.releaseHeld(a)
		fail(a.req)
	}
	for key, ks := range ct.keys {
		if ks.unused() {
			delete(ct.keys, key)
		}
	}
	// No waiters remain, so queued wakes cannot grant anything; drop them.
	ct.wakeQueue = nil
	ct.releasedQueue = nil
}

// parkedCount reports how many acquisitions are currently parked.
func (ct *claimTable) parkedCount() int {
	return ct.parked
}

// oldestParkedSince reports the park time of the longest-parked acquisition
// and false when nothing is parked.
func (ct *claimTable) oldestParkedSince() (time.Time, bool) {
	var oldest time.Time
	for _, ks := range ct.keys {
		for _, a := range ks.waiters {
			if !a.parkedAt.IsZero() && (oldest.IsZero() || a.parkedAt.Before(oldest)) {
				oldest = a.parkedAt
			}
		}
	}
	return oldest, !oldest.IsZero()
}
