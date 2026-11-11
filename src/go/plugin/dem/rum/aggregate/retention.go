// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"container/heap"
	"time"
)

// HTTP body reads can finish out of receipt order. Indexed heaps keep eviction
// chronological without scanning retained state on inserts or revision updates.
// An entry holds its value and receipt time in one allocation.
type receiptEntry[T any] struct {
	Value    T
	received time.Time
	index    int
	sequence uint64
}
type receiptHeap[T any] struct {
	entries  []*receiptEntry[T]
	sequence uint64
}

func (h *receiptHeap[T]) Len() int { return len(h.entries) }
func (h *receiptHeap[T]) Less(i, j int) bool {
	a, b := h.entries[i], h.entries[j]
	if a.received.Equal(b.received) {
		return a.sequence < b.sequence
	}
	return a.received.Before(b.received)
}
func (h *receiptHeap[T]) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.entries[i].index = i
	h.entries[j].index = j
}
func (h *receiptHeap[T]) Push(v any) {
	e := v.(*receiptEntry[T])
	e.index = len(h.entries)
	h.entries = append(h.entries, e)
}
func (h *receiptHeap[T]) Pop() any {
	n := len(h.entries) - 1
	e := h.entries[n]
	h.entries[n] = nil
	h.entries = h.entries[:n]
	e.index = -1
	return e
}
func (h *receiptHeap[T]) oldest() *receiptEntry[T] {
	if len(h.entries) == 0 {
		return nil
	}
	return h.entries[0]
}
func (h *receiptHeap[T]) add(received time.Time, v T) *receiptEntry[T] {
	h.sequence++
	e := &receiptEntry[T]{
		Value:    v,
		received: received,
		sequence: h.sequence,
	}
	heap.Push(h, e)
	return e
}

// renew moves an entry to a later receipt time; earlier times leave it in place.
func (h *receiptHeap[T]) renew(e *receiptEntry[T], received time.Time) {
	if received.After(e.received) {
		e.received = received
		heap.Fix(h, e.index)
	}
}
func (h *receiptHeap[T]) remove(e *receiptEntry[T]) { heap.Remove(h, e.index) }
func (h *receiptHeap[T]) pop() *receiptEntry[T]     { return heap.Pop(h).(*receiptEntry[T]) }
