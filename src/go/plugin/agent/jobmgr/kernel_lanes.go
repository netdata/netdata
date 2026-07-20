// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"
)

func (ck *CommandKernel) allocateLane(mapKey commandLaneKey, request Request) (*commandLane, error) {
	slot := ck.freeLane
	if slot == 0 {
		if uint64(len(ck.laneSlots)) > uint64(^uint32(0)) {
			return nil, errors.New("jobmgr kernel: lane reference space exhausted")
		}
		slot = uint32(len(ck.laneSlots))
		ck.laneSlots = append(ck.laneSlots, &commandLane{slot: slot})
	}
	lane := ck.laneSlots[slot]
	if ck.freeLane != 0 {
		ck.freeLane = lane.freeNext
	}
	generation := lane.generation + 1
	if generation == 0 {
		lane.freeNext = ck.freeLane
		ck.freeLane = slot
		return nil, errors.New("jobmgr kernel: lane generation wrapped")
	}
	*lane = commandLane{
		slot: slot, generation: generation, mapKey: mapKey,
		key: request.LaneKey, source: request.Source,
	}
	ck.lanes[mapKey] = lane
	ck.appendLane(lane)
	return lane, nil
}

func resourceCommandLaneKey(id string) commandLaneKey {
	return commandLaneKey{key: id, resource: true}
}

func (ck *CommandKernel) releaseUnusedLane(lane *commandLane) {
	if lane == nil || lane.owners != 0 || lane.head != nil ||
		lane.tail != nil || lane.active != nil ||
		lane.continuationTail != nil || lane.ready ||
		lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() ||
		lane.installPlanned || lane.stopPlanned ||
		lane.shutdownRequest.Valid() || lane.shutdownTask.Valid() || lane.shutdownAction != 0 {
		return
	}
	if ck.shutdownPhase != commandShutdownRunning &&
		!lane.shutdownVisited {
		return
	}
	delete(ck.lanes, lane.mapKey)
	ck.unlinkLane(lane)
	slot := lane.slot
	generation := lane.generation
	*lane = commandLane{slot: slot, generation: generation, freeNext: ck.freeLane}
	ck.freeLane = slot
}

func (ck *CommandKernel) appendLane(lane *commandLane) {
	if lane == nil || lane.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid lane-list append"))
		return
	}
	lane.allPrevious = ck.laneTail
	if ck.laneTail != nil {
		ck.laneTail.allNext = lane
	} else {
		ck.laneHead = lane
	}
	ck.laneTail = lane
	lane.allListed = true
}

func (ck *CommandKernel) unlinkLane(lane *commandLane) {
	if lane == nil || !lane.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid lane-list removal"))
		return
	}
	if lane.allPrevious != nil {
		lane.allPrevious.allNext = lane.allNext
	} else {
		ck.laneHead = lane.allNext
	}
	if lane.allNext != nil {
		lane.allNext.allPrevious = lane.allPrevious
	} else {
		ck.laneTail = lane.allPrevious
	}
	lane.allPrevious = nil
	lane.allNext = nil
	lane.allListed = false
}
