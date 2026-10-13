// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"container/heap"
	"time"
)

// Keep support evidence reasonably fresh without rewriting every ten-second poll.
const normalPublicationEvery = 5 * time.Minute

// Each current writer has at most one entry. Polls replace its cut without
// postponing publication or allocating another scheduling entry.
type normalQueue []*NormalWriter

func (q normalQueue) Len() int           { return len(q) }
func (q normalQueue) Less(i, j int) bool { return q[i].due.Before(q[j].due) }
func (q normalQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index, q[j].index = i, j
}
func (q *normalQueue) Push(value any) {
	w := value.(*NormalWriter)
	w.index = len(*q)
	*q = append(*q, w)
}
func (q *normalQueue) Pop() any {
	i := len(*q) - 1
	w := (*q)[i]
	(*q)[i] = nil
	*q = (*q)[:i]
	w.index = -1
	return w
}

func (p *Publisher) queueNormalLocked(w *NormalWriter) {
	if w.index < 0 && !w.writing && w.pending != nil {
		if w.due.IsZero() {
			// First evidence is eligible now, behind already overdue work.
			w.due = time.Now()
		}
		heap.Push(&p.normal.queue, w)
	}
}

func (p *Publisher) normalDeadline() (time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.normal.queue) == 0 {
		return time.Time{}, false
	}
	return p.normal.queue[0].due, true
}
