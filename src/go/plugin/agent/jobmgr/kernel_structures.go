// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"time"
)

type functionCleanupTask struct {
	ref FunctionCleanupRef
	err error
}

// Fixed chunks keep every kernel-loop queue operation worst-case O(1). The
// chunk size amortizes allocation; it does not cap the backlog.
const kernelQueueChunkCapacity = 64

type fixedChunkQueueChunk[T any] struct {
	values [kernelQueueChunkCapacity]T
	head   int
	tail   int
	next   *fixedChunkQueueChunk[T]
}

type fixedChunkQueue[T any] struct {
	head  *fixedChunkQueueChunk[T]
	tail  *fixedChunkQueueChunk[T]
	count int
}

func (fcq *fixedChunkQueue[T]) push(value T) {
	if fcq.tail == nil || fcq.tail.tail == kernelQueueChunkCapacity {
		chunk := &fixedChunkQueueChunk[T]{}
		if fcq.tail == nil {
			fcq.head = chunk
		} else {
			fcq.tail.next = chunk
		}
		fcq.tail = chunk
	}
	fcq.tail.values[fcq.tail.tail] = value
	fcq.tail.tail++
	fcq.count++
}

func (fcq *fixedChunkQueue[T]) front() (zero T) {
	if fcq.count == 0 {
		return zero
	}
	return fcq.head.values[fcq.head.head]
}

func (fcq *fixedChunkQueue[T]) pop() {
	if fcq.count == 0 {
		return
	}
	chunk := fcq.head
	var zero T
	chunk.values[chunk.head] = zero
	chunk.head++
	fcq.count--
	if chunk.head != chunk.tail {
		return
	}
	fcq.head = chunk.next
	chunk.next = nil
	if fcq.head == nil {
		fcq.tail = nil
	}
}

type functionCleanupQueue struct {
	fixedChunkQueue[FunctionCleanupPlan]
}

func (fcq *functionCleanupQueue) push(plan FunctionCleanupPlan) {
	if !plan.Valid() {
		return
	}
	fcq.fixedChunkQueue.push(plan)
}

type readyQueue struct {
	head *commandLane
	tail *commandLane
	len  int
}

func (rq *readyQueue) push(lane *commandLane) {
	if lane.ready {
		return
	}
	lane.ready = true
	lane.readyPrev = rq.tail
	if rq.tail != nil {
		rq.tail.readyNext = lane
	} else {
		rq.head = lane
	}
	rq.tail = lane
	rq.len++
}

func (rq *readyQueue) pop() *commandLane {
	lane := rq.head
	if lane == nil {
		return nil
	}
	rq.head = lane.readyNext
	if rq.head != nil {
		rq.head.readyPrev = nil
	} else {
		rq.tail = nil
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	rq.len--
	return lane
}

func (rq *readyQueue) remove(lane *commandLane) {
	if !lane.ready {
		return
	}
	if lane.readyPrev != nil {
		lane.readyPrev.readyNext = lane.readyNext
	} else {
		rq.head = lane.readyNext
	}
	if lane.readyNext != nil {
		lane.readyNext.readyPrev = lane.readyPrev
	} else {
		rq.tail = lane.readyPrev
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	rq.len--
}

type deadlineEntry struct {
	when      time.Time
	operation *commandOperation
	index     int
}

type deadlineHeap []*deadlineEntry

func (dh *deadlineHeap) Len() int { return len(*dh) }

func (dh *deadlineHeap) Less(i, j int) bool { return (*dh)[i].when.Before((*dh)[j].when) }

func (dh *deadlineHeap) Swap(i, j int) {
	(*dh)[i], (*dh)[j] = (*dh)[j], (*dh)[i]
	(*dh)[i].index = i
	(*dh)[j].index = j
}

func (dh *deadlineHeap) Push(value any) {
	entry := value.(*deadlineEntry)
	entry.index = len(*dh)
	*dh = append(*dh, entry)
}

func (dh *deadlineHeap) Pop() any {
	old := *dh
	last := old[len(old)-1]
	old[len(old)-1] = nil
	last.index = -1
	*dh = old[:len(old)-1]
	return last
}
