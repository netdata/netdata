// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"errors"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const maximumClaimSettlementQuantum = 4

type authorityClaimKey struct {
	references      int                 // outstanding edges referencing this key
	yielded         int                 // active suffix yielders retaining reacquisition priority
	yieldLanes      map[string]int      // active yield reservations by borrower resource lane
	holder          *authorityClaimEdge // the edge currently holding the key, if any
	waiterHead      *authorityClaimEdge // head of the FIFO waiter list
	waiterTail      *authorityClaimEdge // tail of the FIFO waiter list
	yieldWaiterTail *authorityClaimEdge // tail of the leading yield-reacquisition waiters
	reservationHead *commandOperation   // composites parked before taking any prefix claim
	reservationTail *commandOperation   // tail of the pre-acquisition reservation list
	settlementIndex int                 // index in the settlement heap (-1 when absent)
}

type authorityClaimEdge struct {
	claim     string              // claim key this edge requests
	operation *commandOperation   // operation owning this edge
	key       *authorityClaimKey  // the claim key this edge is registered against
	held      bool                // the edge currently holds the key
	waiting   bool                // the edge is parked in the key's waiter list
	yielded   bool                // the edge has a live suffix-yield reservation
	prev      *authorityClaimEdge // previous edge in the key's waiter list
	next      *authorityClaimEdge // next edge in the key's waiter list
}

type authorityClaimHeap []*authorityClaimKey

func (ach *authorityClaimHeap) Len() int { return len(*ach) }

func (ach *authorityClaimHeap) Less(left, right int) bool {
	leftOperation := claimSettlementOperation((*ach)[left])
	rightOperation := claimSettlementOperation((*ach)[right])
	return leftOperation.claimTicket < rightOperation.claimTicket
}

func (ach *authorityClaimHeap) Swap(left, right int) {
	(*ach)[left], (*ach)[right] = (*ach)[right], (*ach)[left]
	(*ach)[left].settlementIndex = left
	(*ach)[right].settlementIndex = right
}

func (ach *authorityClaimHeap) Push(value any) {
	state := value.(*authorityClaimKey)
	state.settlementIndex = len(*ach)
	*ach = append(*ach, state)
}

func (ach *authorityClaimHeap) Pop() any {
	old := *ach
	last := old[len(old)-1]
	old[len(old)-1] = nil
	last.settlementIndex = -1
	*ach = old[:len(old)-1]
	return last
}

type claimAuthority struct {
	keys             map[string]*authorityClaimKey                    // active claim keys by string
	nextTicket       uint64                                           // next global FIFO ticket to assign
	waiterCount      int                                              // count of operations currently waiting
	settlements      authorityClaimHeap                               // min-heap of keys with a serviceable waiter (by ticket)
	settlementGrants [maximumClaimSettlementQuantum]*commandOperation // reusable grant buffer for one settlement quantum
	observer         lifecycle.RuntimeObserver                        // sink for claim runtime counters
	now              func() time.Time                                 // clock for the oldest-wait metric
	waitHead         *commandOperation                                // head of the global wait list (oldest-wait metric)
	waitTail         *commandOperation                                // tail of the global wait list
}

func newClaimAuthority() *claimAuthority {
	return &claimAuthority{
		keys: make(map[string]*authorityClaimKey),
	}
}

func (ca *claimAuthority) bindRuntimeObserver(observer lifecycle.RuntimeObserver, now func() time.Time) error {
	if ca == nil || observer == nil || now == nil {
		return errors.New("jobmgr claims: invalid runtime observer")
	}
	if ca.observer != nil || len(ca.keys) != 0 || ca.waiterCount != 0 {
		return errors.New("jobmgr claims: runtime observer bound after activation")
	}
	ca.observer = observer
	ca.now = now
	ca.observeRuntime()
	return nil
}

func normalizeAuthorityClaims(input []string) ([]string, error) {
	claims := slices.Clone(input)
	if slices.Contains(claims, "") {
		return nil, errors.New("jobmgr claims: empty key")
	}
	slices.Sort(claims)
	claims = slices.Compact(claims)
	return claims, nil
}

func prepareClaimEdges(operation *commandOperation, claims []string) {
	operation.claims = claims
	operation.authorityClaimEdges = make([]authorityClaimEdge, len(claims))
	for index, claim := range claims {
		operation.authorityClaimEdges[index] = authorityClaimEdge{
			claim:     claim,
			operation: operation,
		}
	}
	operation.claimPrepared = true
}

func (ca *claimAuthority) register(operation *commandOperation) error {
	if operation == nil || !operation.claimPrepared || operation.claimRegistered {
		return errors.New("jobmgr claims: invalid operation registration")
	}
	for index := range operation.authorityClaimEdges {
		edge := &operation.authorityClaimEdges[index]
		edge.key = ca.key(edge.claim)
		edge.key.references++
	}
	operation.claimRegistered = true
	ca.observeRuntime()
	return nil
}

func (ca *claimAuthority) acquire(operation *commandOperation) (bool, error) {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld ||
		(operation.claimCursor != 0 && !operation.claimsYielded) {
		return false, errors.New("jobmgr claims: invalid acquire")
	}
	ca.nextTicket++
	if ca.nextTicket == 0 {
		return false, errors.New("jobmgr claims: ticket wrapped")
	}
	operation.claimTicket = ca.nextTicket
	if edge := compositeYieldConflictEdge(operation); edge != nil {
		ca.enqueueReservation(edge.key, operation)
		operation.claimWaiting = true
		ca.beginRuntimeWait(operation)
		ca.observeRuntime()
		return false, nil
	}
	granted, err := ca.acquireFromCursor(operation)
	if err == nil && !granted {
		ca.beginRuntimeWait(operation)
	}
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) cancel(operation *commandOperation) ([]*commandOperation, error) {
	if operation == nil || !operation.claimRegistered || !operation.claimWaiting || operation.claimsHeld ||
		operation.claimsYielded {
		return nil, errors.New("jobmgr claims: operation is not waiting")
	}
	if operation.claimReservationKey != nil {
		if operation.claimCursor != 0 {
			return nil, errors.New("jobmgr claims: reserved operation retained a prefix")
		}
		if err := ca.removeReservation(operation); err != nil {
			return nil, err
		}
		operation.claimWaiting = false
	} else {
		edge := &operation.authorityClaimEdges[operation.claimCursor]
		if err := ca.removeWaiter(edge); err != nil {
			return nil, err
		}
		operation.claimWaiting = false
		for index := operation.claimCursor - 1; index >= 0; index-- {
			if err := ca.releaseEdge(&operation.authorityClaimEdges[index]); err != nil {
				return nil, err
			}
		}
		operation.claimCursor = 0
	}
	if err := ca.forget(operation); err != nil {
		return nil, err
	}
	ca.endRuntimeWait(operation)
	granted, _, err := ca.serviceSettlements(maximumClaimSettlementQuantum)
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) release(operation *commandOperation) ([]*commandOperation, error) {
	if operation == nil || !operation.claimRegistered || !operation.claimsHeld || operation.claimWaiting ||
		operation.claimCursor != len(operation.authorityClaimEdges) {
		return nil, errors.New("jobmgr claims: release without complete held claims")
	}
	for index := len(operation.authorityClaimEdges) - 1; index >= 0; index-- {
		if err := ca.releaseEdge(&operation.authorityClaimEdges[index]); err != nil {
			return nil, err
		}
	}
	operation.claimsHeld = false
	operation.claimCursor = 0
	if err := ca.forget(operation); err != nil {
		return nil, err
	}
	granted, _, err := ca.serviceSettlements(maximumClaimSettlementQuantum)
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) yield(
	operation *commandOperation,
	claim string,
	borrowerLane string,
) ([]*commandOperation, error) {
	if operation == nil || !operation.claimRegistered || !operation.claimsHeld || operation.claimWaiting ||
		operation.claimCursor != len(operation.authorityClaimEdges) {
		return nil, errors.New("jobmgr claims: yield without complete held claims")
	}
	yieldIndex := len(operation.authorityClaimEdges) - 1
	if yieldIndex < 0 || operation.authorityClaimEdges[yieldIndex].claim != claim {
		return nil, errors.New("jobmgr claims: yielded claim is not the acquisition suffix")
	}
	edge := &operation.authorityClaimEdges[yieldIndex]
	edge.yielded = true
	edge.key.yielded++
	if edge.key.yieldLanes == nil {
		edge.key.yieldLanes = make(map[string]int)
	}
	edge.key.yieldLanes[borrowerLane]++
	operation.claimYieldLane = borrowerLane
	if err := ca.demoteYieldConflictedWaiters(edge.key); err != nil {
		_ = removeYieldReservation(edge)
		return nil, err
	}
	if err := ca.releaseEdge(edge); err != nil {
		_ = removeYieldReservation(edge)
		return nil, err
	}
	operation.claimsHeld = false
	operation.claimCursor = yieldIndex
	granted, _, err := ca.serviceSettlements(maximumClaimSettlementQuantum)
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) abandon(operation *commandOperation) error {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld ||
		operation.claimCursor != 0 {
		return errors.New("jobmgr claims: abandon outside idle prepared state")
	}
	err := ca.forget(operation)
	ca.observeRuntime()
	return err
}

func (ca *claimAuthority) waiting(operation *commandOperation) bool {
	return operation != nil && operation.claimWaiting
}

func (ca *claimAuthority) waitingCount() int { return ca.waiterCount }

func (ca *claimAuthority) pendingSettlements() bool {
	return ca != nil && len(ca.settlements) != 0
}

func (ca *claimAuthority) acquireFromCursor(operation *commandOperation) (bool, error) {
	for operation.claimCursor < len(operation.authorityClaimEdges) {
		edge := &operation.authorityClaimEdges[operation.claimCursor]
		if edge.held || edge.waiting || edge.key == nil {
			return false, errors.New("jobmgr claims: invalid acquisition edge")
		}
		if edge.key.waiterHead != nil || edge.key.holder != nil ||
			compositeClaimBorrowerBlocked(edge) {
			ca.enqueueWaiter(edge)
			operation.claimWaiting = true
			return false, nil
		}
		if err := holdEdge(edge); err != nil {
			return false, err
		}
		ca.refreshSettlement(edge.key)
		operation.claimCursor++
	}
	operation.claimsHeld = true
	return true, nil
}

func holdEdge(edge *authorityClaimEdge) error {
	if edge == nil || edge.key == nil || edge.held || edge.waiting || edge.key.holder != nil {
		return errors.New("jobmgr claims: invalid edge hold")
	}
	if edge.yielded {
		if err := removeYieldReservation(edge); err != nil {
			return err
		}
	}
	edge.held = true
	edge.key.holder = edge
	return nil
}

func removeYieldReservation(edge *authorityClaimEdge) error {
	if edge == nil || edge.key == nil || !edge.yielded || edge.operation == nil ||
		edge.key.yielded <= 0 || edge.key.yieldLanes == nil {
		return errors.New("jobmgr claims: missing yield reservation")
	}
	lane := edge.operation.claimYieldLane
	count := edge.key.yieldLanes[lane]
	if count <= 0 {
		return errors.New("jobmgr claims: missing yield lane reservation")
	}
	if count == 1 {
		delete(edge.key.yieldLanes, lane)
	} else {
		edge.key.yieldLanes[lane] = count - 1
	}
	if len(edge.key.yieldLanes) == 0 {
		edge.key.yieldLanes = nil
	}
	edge.key.yielded--
	edge.yielded = false
	edge.operation.claimYieldLane = ""
	return nil
}

func (ca *claimAuthority) releaseEdge(edge *authorityClaimEdge) error {
	if edge == nil || !edge.held || edge.waiting || edge.key == nil {
		return errors.New("jobmgr claims: release of unheld edge")
	}
	if edge.key.holder != edge {
		return errors.New("jobmgr claims: holder mismatch")
	}
	edge.key.holder = nil
	edge.held = false
	edge.prev = nil
	edge.next = nil
	ca.refreshSettlement(edge.key)
	return nil
}

func (ca *claimAuthority) enqueueWaiter(edge *authorityClaimEdge) {
	edge.waiting = true
	if edge.yielded {
		previous := edge.key.yieldWaiterTail
		if previous == nil {
			edge.next = edge.key.waiterHead
			if edge.next != nil {
				edge.next.prev = edge
			} else {
				edge.key.waiterTail = edge
			}
			edge.key.waiterHead = edge
		} else {
			edge.prev = previous
			edge.next = previous.next
			previous.next = edge
			if edge.next != nil {
				edge.next.prev = edge
			} else {
				edge.key.waiterTail = edge
			}
		}
		edge.key.yieldWaiterTail = edge
	} else {
		edge.prev = edge.key.waiterTail
		if edge.key.waiterTail != nil {
			edge.key.waiterTail.next = edge
		} else {
			edge.key.waiterHead = edge
		}
		edge.key.waiterTail = edge
	}
	ca.waiterCount++
	ca.refreshSettlement(edge.key)
}

func (ca *claimAuthority) removeWaiter(edge *authorityClaimEdge) error {
	if edge == nil || !edge.waiting || edge.held || edge.key == nil {
		return errors.New("jobmgr claims: remove of non-waiter edge")
	}
	if edge.prev != nil {
		edge.prev.next = edge.next
	} else if edge.key.waiterHead == edge {
		edge.key.waiterHead = edge.next
	} else {
		return errors.New("jobmgr claims: waiter head mismatch")
	}
	if edge.next != nil {
		edge.next.prev = edge.prev
	} else if edge.key.waiterTail == edge {
		edge.key.waiterTail = edge.prev
	} else {
		return errors.New("jobmgr claims: waiter tail mismatch")
	}
	if edge.key.yieldWaiterTail == edge {
		if edge.prev != nil && edge.prev.yielded {
			edge.key.yieldWaiterTail = edge.prev
		} else {
			edge.key.yieldWaiterTail = nil
		}
	}
	edge.waiting = false
	edge.prev = nil
	edge.next = nil
	ca.waiterCount--
	ca.refreshSettlement(edge.key)
	return nil
}

func (ca *claimAuthority) serviceSettlements(quantum int) ([]*commandOperation, bool, error) {
	if ca == nil || quantum <= 0 || quantum > len(ca.settlementGrants) {
		return nil, false, errors.New("jobmgr claims: invalid settlement quantum")
	}
	clear(ca.settlementGrants[:])
	grantCount := 0
	for visited := 0; visited < quantum && len(ca.settlements) != 0; visited++ {
		key := heap.Pop(&ca.settlements).(*authorityClaimKey)
		if !claimSettlementEligible(key) {
			return nil, false, errors.New("jobmgr claims: ineligible settlement key")
		}
		operation, reserved := claimSettlementCandidate(key)
		if operation == nil || !operation.claimWaiting {
			return nil, false, errors.New("jobmgr claims: invalid wake operation")
		}
		if reserved {
			if operation.claimCursor != 0 || operation.claimReservationKey != key {
				return nil, false, errors.New("jobmgr claims: invalid reservation wake")
			}
			if err := ca.removeReservation(operation); err != nil {
				return nil, false, err
			}
			operation.claimWaiting = false
			if conflict := compositeYieldConflictEdge(operation); conflict != nil {
				ca.enqueueReservation(conflict.key, operation)
				operation.claimWaiting = true
				continue
			}
		} else {
			if operation.claimCursor >= len(operation.authorityClaimEdges) {
				return nil, false, errors.New("jobmgr claims: invalid wake cursor")
			}
			edge := &operation.authorityClaimEdges[operation.claimCursor]
			if edge.key != key || key.holder != nil {
				return nil, false, errors.New("jobmgr claims: settlement waiter differs")
			}
			if err := ca.removeWaiter(edge); err != nil {
				return nil, false, err
			}
			operation.claimWaiting = false
			if err := holdEdge(edge); err != nil {
				return nil, false, err
			}
			ca.refreshSettlement(key)
			operation.claimCursor++
		}
		granted, err := ca.acquireFromCursor(operation)
		if err != nil {
			return nil, false, err
		}
		if granted {
			ca.endRuntimeWait(operation)
			ca.settlementGrants[grantCount] = operation
			grantCount++
		}
	}
	ca.observeRuntime()
	return ca.settlementGrants[:grantCount:grantCount], len(ca.settlements) != 0, nil
}

func (ca *claimAuthority) beginRuntimeWait(operation *commandOperation) {
	if ca.observer == nil || operation == nil || operation.claimWaitListed {
		return
	}
	operation.claimWaitStarted = ca.now()
	operation.claimWaitPrevious = ca.waitTail
	if ca.waitTail != nil {
		ca.waitTail.claimWaitNext = operation
	} else {
		ca.waitHead = operation
	}
	ca.waitTail = operation
	operation.claimWaitListed = true
}

func (ca *claimAuthority) endRuntimeWait(operation *commandOperation) {
	if ca.observer == nil || operation == nil || !operation.claimWaitListed {
		return
	}
	if operation.claimWaitPrevious != nil {
		operation.claimWaitPrevious.claimWaitNext = operation.claimWaitNext
	} else {
		ca.waitHead = operation.claimWaitNext
	}
	if operation.claimWaitNext != nil {
		operation.claimWaitNext.claimWaitPrevious = operation.claimWaitPrevious
	} else {
		ca.waitTail = operation.claimWaitPrevious
	}
	operation.claimWaitStarted = time.Time{}
	operation.claimWaitPrevious = nil
	operation.claimWaitNext = nil
	operation.claimWaitListed = false
}

func (ca *claimAuthority) observeRuntime() {
	if ca == nil || ca.observer == nil {
		return
	}
	ca.observer.SetRuntimeGauge(lifecycle.RuntimeGaugeClaimKeysTracked, len(ca.keys))
	ca.observer.SetRuntimeGauge(lifecycle.RuntimeGaugeClaimWaiters, ca.waiterCount)
	var oldest time.Time
	if ca.waitHead != nil {
		oldest = ca.waitHead.claimWaitStarted
	}
	ca.observer.SetRuntimeTimestamp(lifecycle.RuntimeTimestampOldestClaimWait, oldest)
}

func (ca *claimAuthority) refreshSettlement(key *authorityClaimKey) {
	if key == nil {
		return
	}
	eligible := claimSettlementEligible(key)
	switch {
	case eligible && key.settlementIndex < 0:
		heap.Push(&ca.settlements, key)
	case eligible:
		heap.Fix(&ca.settlements, key.settlementIndex)
	case key.settlementIndex >= 0:
		heap.Remove(&ca.settlements, key.settlementIndex)
	}
}

func claimSettlementEligible(key *authorityClaimKey) bool {
	return claimSettlementOperation(key) != nil
}

func claimSettlementOperation(key *authorityClaimKey) *commandOperation {
	operation, _ := claimSettlementCandidate(key)
	return operation
}

func claimSettlementCandidate(key *authorityClaimKey) (*commandOperation, bool) {
	if key == nil || key.holder != nil {
		return nil, false
	}
	var waiter *commandOperation
	for edge := key.waiterHead; edge != nil; edge = edge.next {
		if !compositeClaimBorrowerBlocked(edge) {
			waiter = edge.operation
			break
		}
	}
	var reserved *commandOperation
	for operation := key.reservationHead; operation != nil; operation = operation.claimReservationNext {
		if !compositeConflictsWithYieldedLanes(operation, key) {
			reserved = operation
			break
		}
	}
	switch {
	case waiter == nil:
		return reserved, reserved != nil
	case reserved == nil || waiter.claimTicket < reserved.claimTicket:
		return waiter, false
	default:
		return reserved, true
	}
}

func compositeClaimBorrowerBlocked(edge *authorityClaimEdge) bool {
	return edge != nil && edge.key != nil && !edge.yielded &&
		compositeConflictsWithYieldedLanes(edge.operation, edge.key)
}

func compositeYieldConflictEdge(operation *commandOperation) *authorityClaimEdge {
	if operation == nil || operation.claimsYielded ||
		operation.plan.Transaction == nil ||
		operation.plan.Transaction.PrepareComposite == nil {
		return nil
	}
	for index := range operation.authorityClaimEdges {
		edge := &operation.authorityClaimEdges[index]
		if compositeConflictsWithYieldedLanes(operation, edge.key) {
			return edge
		}
	}
	return nil
}

func compositeConflictsWithYieldedLanes(operation *commandOperation, key *authorityClaimKey) bool {
	if operation == nil || key == nil || key.yielded == 0 ||
		operation.plan.Transaction == nil ||
		operation.plan.Transaction.PrepareComposite == nil {
		return false
	}
	conflicts := operation.plan.Transaction.CompositeChildLaneConflict
	if conflicts == nil {
		return true
	}
	for lane := range key.yieldLanes {
		if callCompositeChildLaneConflict(conflicts, lane) {
			return true
		}
	}
	return false
}

func callCompositeChildLaneConflict(conflicts func(string) bool, lane string) (result bool) {
	result = true
	defer func() {
		_ = recover()
	}()
	return conflicts(lane)
}

func (ca *claimAuthority) demoteYieldConflictedWaiters(key *authorityClaimKey) error {
	if key == nil || key.yielded == 0 {
		return errors.New("jobmgr claims: invalid yield-conflict demotion")
	}
	for edge := key.waiterHead; edge != nil; {
		next := edge.next
		if compositeClaimBorrowerBlocked(edge) {
			operation := edge.operation
			if operation == nil || operation.claimCursor >= len(operation.authorityClaimEdges) ||
				&operation.authorityClaimEdges[operation.claimCursor] != edge {
				return errors.New("jobmgr claims: invalid conflicted waiter")
			}
			if err := ca.removeWaiter(edge); err != nil {
				return err
			}
			operation.claimWaiting = false
			for index := operation.claimCursor - 1; index >= 0; index-- {
				if err := ca.releaseEdge(&operation.authorityClaimEdges[index]); err != nil {
					return err
				}
			}
			operation.claimCursor = 0
			ca.enqueueReservation(key, operation)
			operation.claimWaiting = true
		}
		edge = next
	}
	return nil
}

func (ca *claimAuthority) enqueueReservation(key *authorityClaimKey, operation *commandOperation) {
	operation.claimReservationKey = key
	cursor := key.reservationTail
	for cursor != nil && cursor.claimTicket > operation.claimTicket {
		cursor = cursor.claimReservationPrevious
	}
	if cursor == nil {
		operation.claimReservationNext = key.reservationHead
		if key.reservationHead != nil {
			key.reservationHead.claimReservationPrevious = operation
		} else {
			key.reservationTail = operation
		}
		key.reservationHead = operation
	} else {
		operation.claimReservationPrevious = cursor
		operation.claimReservationNext = cursor.claimReservationNext
		cursor.claimReservationNext = operation
		if operation.claimReservationNext != nil {
			operation.claimReservationNext.claimReservationPrevious = operation
		} else {
			key.reservationTail = operation
		}
	}
	ca.waiterCount++
	ca.refreshSettlement(key)
}

func (ca *claimAuthority) removeReservation(operation *commandOperation) error {
	if operation == nil || operation.claimReservationKey == nil {
		return errors.New("jobmgr claims: remove of non-reservation")
	}
	key := operation.claimReservationKey
	if operation.claimReservationPrevious != nil {
		operation.claimReservationPrevious.claimReservationNext = operation.claimReservationNext
	} else if key.reservationHead == operation {
		key.reservationHead = operation.claimReservationNext
	} else {
		return errors.New("jobmgr claims: reservation head mismatch")
	}
	if operation.claimReservationNext != nil {
		operation.claimReservationNext.claimReservationPrevious = operation.claimReservationPrevious
	} else if key.reservationTail == operation {
		key.reservationTail = operation.claimReservationPrevious
	} else {
		return errors.New("jobmgr claims: reservation tail mismatch")
	}
	operation.claimReservationKey = nil
	operation.claimReservationPrevious = nil
	operation.claimReservationNext = nil
	ca.waiterCount--
	ca.refreshSettlement(key)
	return nil
}

func (ca *claimAuthority) forget(operation *commandOperation) error {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld ||
		operation.claimCursor != 0 {
		return errors.New("jobmgr claims: forget outside terminal claim state")
	}
	for index := range operation.authorityClaimEdges {
		edge := &operation.authorityClaimEdges[index]
		if edge.held || edge.waiting || edge.yielded || edge.key == nil || edge.key.references <= 0 ||
			operation.claimReservationKey != nil || operation.claimYieldLane != "" {
			return errors.New("jobmgr claims: invalid terminal edge")
		}
		edge.key.references--
		if edge.key.references == 0 && edge.key.holder == nil && edge.key.waiterHead == nil &&
			edge.key.yielded == 0 && edge.key.yieldWaiterTail == nil &&
			edge.key.reservationHead == nil && edge.key.reservationTail == nil &&
			len(edge.key.yieldLanes) == 0 {
			if edge.key.settlementIndex >= 0 {
				return errors.New("jobmgr claims: terminal key remains settlement-eligible")
			}
			delete(ca.keys, edge.claim)
		}
		edge.key = nil
	}
	operation.claimRegistered = false
	return nil
}

func (ca *claimAuthority) key(name string) *authorityClaimKey {
	state := ca.keys[name]
	if state == nil {
		state = &authorityClaimKey{
			settlementIndex: -1,
		}
		ca.keys[name] = state
	}
	return state
}
