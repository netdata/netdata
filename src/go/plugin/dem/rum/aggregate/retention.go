// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"container/heap"
	"time"
)

// HTTP body reads can finish out of receipt order. Indexed heaps keep eviction
// chronological without scanning retained state on inserts or revision updates.
// An entry holds its key, value and receipt time in one allocation; unkeyed
// populations use an empty key.
type receiptEntry[K comparable, V any] struct {
	Value    V
	key      K
	received time.Time
	index    int
	sequence uint64
}
type receiptHeap[K comparable, V any] struct {
	entries  []*receiptEntry[K, V]
	sequence uint64
}

func (h *receiptHeap[K, V]) Len() int { return len(h.entries) }
func (h *receiptHeap[K, V]) Less(i, j int) bool {
	a, b := h.entries[i], h.entries[j]
	if a.received.Equal(b.received) {
		return a.sequence < b.sequence
	}
	return a.received.Before(b.received)
}
func (h *receiptHeap[K, V]) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.entries[i].index = i
	h.entries[j].index = j
}
func (h *receiptHeap[K, V]) Push(v any) {
	e := v.(*receiptEntry[K, V])
	e.index = len(h.entries)
	h.entries = append(h.entries, e)
}
func (h *receiptHeap[K, V]) Pop() any {
	n := len(h.entries) - 1
	e := h.entries[n]
	h.entries[n] = nil
	h.entries = h.entries[:n]
	e.index = -1
	return e
}
func (h *receiptHeap[K, V]) add(received time.Time, key K, v V) *receiptEntry[K, V] {
	h.sequence++
	e := &receiptEntry[K, V]{
		Value:    v,
		key:      key,
		received: received,
		sequence: h.sequence,
	}
	heap.Push(h, e)
	return e
}

// renew moves an entry to a later receipt time; earlier times leave it in place.
func (h *receiptHeap[K, V]) renew(e *receiptEntry[K, V], received time.Time) {
	if received.After(e.received) {
		e.received = received
		heap.Fix(h, e.index)
	}
}
func (h *receiptHeap[K, V]) remove(e *receiptEntry[K, V]) { heap.Remove(h, e.index) }

// popExpired removes and returns the oldest entry received before cutoff, or nil.
// Expiry touches only expired roots, each in O(log retained).
func (h *receiptHeap[K, V]) popExpired(cutoff time.Time) *receiptEntry[K, V] {
	if len(h.entries) == 0 || !h.entries[0].received.Before(cutoff) {
		return nil
	}
	return heap.Pop(h).(*receiptEntry[K, V])
}
func (h *receiptHeap[K, V]) expire(cutoff time.Time) {
	for h.popExpired(cutoff) != nil {
	}
}

// expireMeasurements drops canonical populations and replay evidence received
// before their horizons, removing only expired heap roots.
func (st *siteState) expireMeasurements(now time.Time) {
	cutoff := now.Add(-st.window)
	st.observations.expire(cutoff)
	st.activity.expire(cutoff)
	st.resources.expire(cutoff)
	st.sessions.expire(cutoff)
	st.pageSessions.expire(cutoff)
	identityCutoff := now.Add(-identityRetention)
	st.identities.expire(identityCutoff)
	st.resourceIdentities.expire(identityCutoff)
	st.vitalIdentities.expire(identityCutoff)
}

// expire is read-side cleanup: measurements, receiver activity and idle detail sessions.
func (st *siteState) expire(now time.Time) {
	st.expireMeasurements(now)
	st.evictActivity(now)
	st.evictSessions(now)
}

// receiptIndex retains at most one entry per key, in receipt order.
type receiptIndex[K comparable, V any] struct {
	byKey map[K]*receiptEntry[K, V]
	order receiptHeap[K, V]
}

func newReceiptIndex[K comparable, V any]() receiptIndex[K, V] {
	return receiptIndex[K, V]{byKey: map[K]*receiptEntry[K, V]{}}
}
func (x *receiptIndex[K, V]) len() int                      { return len(x.byKey) }
func (x *receiptIndex[K, V]) get(key K) *receiptEntry[K, V] { return x.byKey[key] }

// add retains an absent key; put replaces a retained one.
func (x *receiptIndex[K, V]) add(received time.Time, key K, v V) {
	x.byKey[key] = x.order.add(received, key, v)
}
func (x *receiptIndex[K, V]) renew(e *receiptEntry[K, V], received time.Time) {
	x.order.renew(e, received)
}
func (x *receiptIndex[K, V]) remove(e *receiptEntry[K, V]) {
	x.order.remove(e)
	delete(x.byKey, e.key)
}

// put replaces a retained value; its receipt time never moves backwards.
func (x *receiptIndex[K, V]) put(received time.Time, key K, v V) {
	if e := x.byKey[key]; e != nil {
		e.Value = v
		x.order.renew(e, received)
		return
	}
	x.add(received, key, v)
}

// current returns key's entry unless it was received more than horizon before
// now. A stale entry no longer counts as evidence and is removed.
func (x *receiptIndex[K, V]) current(key K, now time.Time, horizon time.Duration) *receiptEntry[K, V] {
	e := x.byKey[key]
	if e != nil && now.Sub(e.received) > horizon {
		x.remove(e)
		return nil
	}
	return e
}
func (x *receiptIndex[K, V]) expire(cutoff time.Time) {
	for e := x.order.popExpired(cutoff); e != nil; e = x.order.popExpired(cutoff) {
		delete(x.byKey, e.key)
	}
}
