// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"container/heap"
	"time"
)

// HTTP body reads can finish out of receipt order. Indexed heaps keep eviction
// chronological without scanning retained state on inserts or revision updates.
type receiptObservation interface{ receivedAt() time.Time }

func (v *identity) receivedAt() time.Time            { return v.received }
func (v *vitalObservation) receivedAt() time.Time    { return v.received }
func (v *sessionObservation) receivedAt() time.Time  { return v.received }
func (v *activityObservation) receivedAt() time.Time { return v.received }

type receiptEntry struct {
	Value    receiptObservation
	index    int
	sequence uint64
}
type receiptHeap struct {
	entries  []*receiptEntry
	sequence uint64
}

func (h *receiptHeap) Len() int { return len(h.entries) }
func (h *receiptHeap) Less(i, j int) bool {
	a, b := h.entries[i], h.entries[j]
	if a.Value.receivedAt().Equal(b.Value.receivedAt()) {
		return a.sequence < b.sequence
	}
	return a.Value.receivedAt().Before(b.Value.receivedAt())
}
func (h *receiptHeap) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.entries[i].index = i
	h.entries[j].index = j
}
func (h *receiptHeap) Push(v any) {
	e := v.(*receiptEntry)
	e.index = len(h.entries)
	h.entries = append(h.entries, e)
}
func (h *receiptHeap) Pop() any {
	n := len(h.entries) - 1
	e := h.entries[n]
	h.entries[n] = nil
	h.entries = h.entries[:n]
	e.index = -1
	return e
}
func (h *receiptHeap) oldest() *receiptEntry {
	if len(h.entries) == 0 {
		return nil
	}
	return h.entries[0]
}
func (h *receiptHeap) add(v receiptObservation) *receiptEntry {
	h.sequence++
	e := &receiptEntry{
		Value:    v,
		sequence: h.sequence,
	}
	heap.Push(h, e)
	return e
}
func (h *receiptHeap) remove(e *receiptEntry) { heap.Remove(h, e.index) }
func (h *receiptHeap) fix(e *receiptEntry)    { heap.Fix(h, e.index) }
func (h *receiptHeap) pop() *receiptEntry     { return heap.Pop(h).(*receiptEntry) }

// Capacity is per canonical population. An older incoming observation cannot
// displace a newer retained one; callers record loss for the actual victim.
func (h *receiptHeap) capacityVictim(incoming receiptObservation) receiptObservation {
	if h.Len() < maxWindowObservations {
		return nil
	}
	if incoming.receivedAt().Before(h.oldest().Value.receivedAt()) {
		return incoming
	}
	return h.pop().Value
}
