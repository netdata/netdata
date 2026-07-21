// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"time"
)

type functionCleanupTask struct {
	ref FunctionCleanupRef
	err error
}

type functionCleanupQueue struct {
	plans []FunctionCleanupPlan
	head  int
	count int
}

func (fcq *functionCleanupQueue) push(plan FunctionCleanupPlan) error {
	if err := plan.validate(); err != nil {
		return err
	}
	if !plan.Ref.Valid() {
		return nil
	}
	fcq.compact()
	fcq.plans = append(fcq.plans, plan)
	fcq.count++
	return nil
}

func (fcq *functionCleanupQueue) compact() {
	if fcq.head == 0 || fcq.head < len(fcq.plans)/2 {
		return
	}
	if cap(fcq.plans) > 2*fcq.count {
		plans := make([]FunctionCleanupPlan, fcq.count)
		copy(plans, fcq.plans[fcq.head:])
		fcq.plans = plans
		fcq.head = 0
		return
	}
	copy(fcq.plans, fcq.plans[fcq.head:])
	clear(fcq.plans[fcq.count:])
	fcq.plans = fcq.plans[:fcq.count]
	fcq.head = 0
}

func (fcq *functionCleanupQueue) front() FunctionCleanupPlan {
	if fcq.count == 0 {
		return FunctionCleanupPlan{}
	}
	return fcq.plans[fcq.head]
}

func (fcq *functionCleanupQueue) pop() {
	if fcq.count == 0 {
		return
	}
	fcq.plans[fcq.head] = FunctionCleanupPlan{}
	fcq.head++
	fcq.count--
	if fcq.count == 0 {
		fcq.plans = nil
		fcq.head = 0
	}
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
