// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type authorityClaimMode uint8

const (
	authorityClaimRead authorityClaimMode = iota + 1
	authorityClaimWrite
)

const maximumClaimSettlementQuantum = 4

type authorityClaim struct {
	key  string
	mode authorityClaimMode
}

type authorityClaimKey struct {
	references      int
	writer          *authorityClaimEdge
	readerHead      *authorityClaimEdge
	readerTail      *authorityClaimEdge
	waiterHead      *authorityClaimEdge
	waiterTail      *authorityClaimEdge
	settlementIndex int
}

type authorityClaimEdge struct {
	claim     authorityClaim
	operation *commandOperation
	key       *authorityClaimKey
	held      bool
	waiting   bool
	prev      *authorityClaimEdge
	next      *authorityClaimEdge
}

type authorityClaimHeap []*authorityClaimKey

func (ach authorityClaimHeap) Len() int { return len(ach) }

func (ach authorityClaimHeap) Less(left, right int) bool {
	return ach[left].waiterHead.operation.claimTicket <
		ach[right].waiterHead.operation.claimTicket
}

func (ach authorityClaimHeap) Swap(left, right int) {
	ach[left], ach[right] = ach[right], ach[left]
	ach[left].settlementIndex = left
	ach[right].settlementIndex = right
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
	keys             map[string]*authorityClaimKey
	nextTicket       uint64
	waiterCount      int
	settlements      authorityClaimHeap
	settlementGrants [maximumClaimSettlementQuantum]*commandOperation
	observer         lifecycle.RuntimeObserver
	now              func() time.Time
	waitHead         *commandOperation
	waitTail         *commandOperation
}

func newClaimAuthority() *claimAuthority {
	return &claimAuthority{
		keys:        make(map[string]*authorityClaimKey),
		settlements: make(authorityClaimHeap, 0, maximumPlanClaims),
	}
}

func (ca *claimAuthority) BindRuntimeObserver(
	observer lifecycle.RuntimeObserver,
	now func() time.Time,
) error {
	if ca == nil || observer == nil || now == nil {
		return errors.New("jobmgr claims: invalid runtime observer")
	}
	if ca.observer != nil || len(ca.keys) != 0 ||
		ca.waiterCount != 0 {
		return errors.New("jobmgr claims: runtime observer bound after activation")
	}
	ca.observer = observer
	ca.now = now
	ca.observeRuntime()
	return nil
}

func normalizeAuthorityClaims(writeClaims []string) ([]authorityClaim, error) {
	return normalizeAuthorityClaimModes(writeClaims, nil)
}

func normalizeAuthorityClaimModes(writeClaims, readClaims []string) ([]authorityClaim, error) {
	claims := make([]authorityClaim, 0, len(writeClaims)+len(readClaims))
	for _, key := range readClaims {
		claims = append(claims, authorityClaim{key: key, mode: authorityClaimRead})
	}
	for _, key := range writeClaims {
		claims = append(claims, authorityClaim{key: key, mode: authorityClaimWrite})
	}
	for _, claim := range claims {
		if claim.key == "" {
			return nil, errors.New("jobmgr claims: empty key")
		}
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].key == claims[j].key {
			return claims[i].mode > claims[j].mode
		}
		return claims[i].key < claims[j].key
	})
	claims = slices.CompactFunc(claims, func(left, right authorityClaim) bool { return left.key == right.key })
	return claims, nil
}

func prepareClaimEdges(operation *commandOperation, claims []authorityClaim) {
	operation.claims = claims
	operation.authorityClaimEdges = make([]authorityClaimEdge, len(claims))
	for index, claim := range claims {
		operation.authorityClaimEdges[index] = authorityClaimEdge{claim: claim, operation: operation}
	}
	operation.claimPrepared = true
}

func (ca *claimAuthority) Register(operation *commandOperation) error {
	if operation == nil || !operation.claimPrepared || operation.claimRegistered {
		return errors.New("jobmgr claims: invalid operation registration")
	}
	for index := range operation.authorityClaimEdges {
		edge := &operation.authorityClaimEdges[index]
		edge.key = ca.key(edge.claim.key)
		edge.key.references++
	}
	operation.claimRegistered = true
	ca.observeRuntime()
	return nil
}

func (ca *claimAuthority) Acquire(operation *commandOperation) (bool, error) {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld || operation.claimCursor != 0 {
		return false, errors.New("jobmgr claims: invalid acquire")
	}
	ca.nextTicket++
	if ca.nextTicket == 0 {
		return false, errors.New("jobmgr claims: ticket wrapped")
	}
	operation.claimTicket = ca.nextTicket
	granted, err := ca.acquireFromCursor(operation)
	if err == nil && !granted {
		ca.beginRuntimeWait(operation)
	}
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) Cancel(operation *commandOperation) ([]*commandOperation, error) {
	if operation == nil || !operation.claimRegistered || !operation.claimWaiting || operation.claimsHeld {
		return nil, errors.New("jobmgr claims: operation is not waiting")
	}
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
	if err := ca.forget(operation); err != nil {
		return nil, err
	}
	ca.endRuntimeWait(operation)
	granted, _, err := ca.ServiceSettlements(
		maximumClaimSettlementQuantum,
	)
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) Release(operation *commandOperation) ([]*commandOperation, error) {
	if operation == nil || !operation.claimRegistered || !operation.claimsHeld || operation.claimWaiting || operation.claimCursor != len(operation.authorityClaimEdges) {
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
	granted, _, err := ca.ServiceSettlements(
		maximumClaimSettlementQuantum,
	)
	ca.observeRuntime()
	return granted, err
}

func (ca *claimAuthority) Abandon(operation *commandOperation) error {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld || operation.claimCursor != 0 {
		return errors.New("jobmgr claims: abandon outside idle prepared state")
	}
	err := ca.forget(operation)
	ca.observeRuntime()
	return err
}

func (ca *claimAuthority) Waiting(operation *commandOperation) bool {
	return operation != nil && operation.claimWaiting
}

func (ca *claimAuthority) WaitingCount() int { return ca.waiterCount }

func (ca *claimAuthority) PendingSettlements() bool {
	return ca != nil && len(ca.settlements) != 0
}

func (ca *claimAuthority) acquireFromCursor(operation *commandOperation) (bool, error) {
	for operation.claimCursor < len(operation.authorityClaimEdges) {
		edge := &operation.authorityClaimEdges[operation.claimCursor]
		if edge.held || edge.waiting || edge.key == nil {
			return false, errors.New("jobmgr claims: invalid acquisition edge")
		}
		if edge.key.waiterHead != nil || !canHold(edge.key, edge.claim.mode) {
			ca.enqueueWaiter(edge)
			operation.claimWaiting = true
			return false, nil
		}
		holdEdge(edge)
		ca.refreshSettlement(edge.key)
		operation.claimCursor++
	}
	operation.claimsHeld = true
	return true, nil
}

func canHold(key *authorityClaimKey, mode authorityClaimMode) bool {
	if key.writer != nil {
		return false
	}
	return mode == authorityClaimRead || key.readerHead == nil
}

func holdEdge(edge *authorityClaimEdge) {
	edge.held = true
	if edge.claim.mode == authorityClaimWrite {
		edge.key.writer = edge
		return
	}
	edge.prev = edge.key.readerTail
	if edge.key.readerTail != nil {
		edge.key.readerTail.next = edge
	} else {
		edge.key.readerHead = edge
	}
	edge.key.readerTail = edge
}

func (ca *claimAuthority) releaseEdge(edge *authorityClaimEdge) error {
	if edge == nil || !edge.held || edge.waiting || edge.key == nil {
		return errors.New("jobmgr claims: release of unheld edge")
	}
	if edge.claim.mode == authorityClaimWrite {
		if edge.key.writer != edge {
			return errors.New("jobmgr claims: writer holder mismatch")
		}
		edge.key.writer = nil
	} else {
		if edge.prev != nil {
			edge.prev.next = edge.next
		} else if edge.key.readerHead == edge {
			edge.key.readerHead = edge.next
		} else {
			return errors.New("jobmgr claims: reader holder mismatch")
		}
		if edge.next != nil {
			edge.next.prev = edge.prev
		} else if edge.key.readerTail == edge {
			edge.key.readerTail = edge.prev
		} else {
			return errors.New("jobmgr claims: reader tail mismatch")
		}
	}
	edge.held = false
	edge.prev = nil
	edge.next = nil
	ca.refreshSettlement(edge.key)
	return nil
}

func (ca *claimAuthority) enqueueWaiter(edge *authorityClaimEdge) {
	edge.waiting = true
	edge.prev = edge.key.waiterTail
	if edge.key.waiterTail != nil {
		edge.key.waiterTail.next = edge
	} else {
		edge.key.waiterHead = edge
	}
	edge.key.waiterTail = edge
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
	edge.waiting = false
	edge.prev = nil
	edge.next = nil
	ca.waiterCount--
	ca.refreshSettlement(edge.key)
	return nil
}

func (ca *claimAuthority) ServiceSettlements(
	quantum int,
) ([]*commandOperation, bool, error) {
	if ca == nil || quantum <= 0 ||
		quantum > len(ca.settlementGrants) {
		return nil, false, errors.New("jobmgr claims: invalid settlement quantum")
	}
	clear(ca.settlementGrants[:])
	grantCount := 0
	for visited := 0; visited < quantum && len(ca.settlements) != 0; visited++ {
		key := heap.Pop(&ca.settlements).(*authorityClaimKey)
		if !claimSettlementEligible(key) {
			return nil, false, errors.New("jobmgr claims: ineligible settlement key")
		}
		operation := key.waiterHead.operation
		if operation == nil || !operation.claimWaiting || operation.claimCursor >= len(operation.authorityClaimEdges) {
			return nil, false, errors.New("jobmgr claims: invalid wake operation")
		}
		edge := &operation.authorityClaimEdges[operation.claimCursor]
		if edge.key != key || key.waiterHead != edge ||
			!canHold(key, edge.claim.mode) {
			return nil, false, errors.New("jobmgr claims: settlement head differs")
		}
		if err := ca.removeWaiter(edge); err != nil {
			return nil, false, err
		}
		operation.claimWaiting = false
		holdEdge(edge)
		ca.refreshSettlement(key)
		operation.claimCursor++
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
	return ca.settlementGrants[:grantCount:grantCount],
		len(ca.settlements) != 0,
		nil
}

func (ca *claimAuthority) beginRuntimeWait(
	operation *commandOperation,
) {
	if ca.observer == nil || operation == nil ||
		operation.claimWaitListed {
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

func (ca *claimAuthority) endRuntimeWait(
	operation *commandOperation,
) {
	if ca.observer == nil || operation == nil ||
		!operation.claimWaitListed {
		return
	}
	if operation.claimWaitPrevious != nil {
		operation.claimWaitPrevious.claimWaitNext = operation.claimWaitNext
	} else {
		ca.waitHead = operation.claimWaitNext
	}
	if operation.claimWaitNext != nil {
		operation.claimWaitNext.claimWaitPrevious =
			operation.claimWaitPrevious
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
	ca.observer.SetRuntimeGauge(
		lifecycle.RuntimeGaugeClaimKeysTracked,
		len(ca.keys),
	)
	ca.observer.SetRuntimeGauge(
		lifecycle.RuntimeGaugeClaimWaiters,
		ca.waiterCount,
	)
	var oldest time.Time
	if ca.waitHead != nil {
		oldest = ca.waitHead.claimWaitStarted
	}
	ca.observer.SetRuntimeTimestamp(
		lifecycle.RuntimeTimestampOldestClaimWait,
		oldest,
	)
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
	if key == nil || key.writer != nil || key.waiterHead == nil {
		return false
	}
	if key.waiterHead.claim.mode == authorityClaimWrite {
		return key.readerHead == nil
	}
	return true
}

func (ca *claimAuthority) forget(operation *commandOperation) error {
	if operation == nil || !operation.claimRegistered || operation.claimWaiting || operation.claimsHeld || operation.claimCursor != 0 {
		return errors.New("jobmgr claims: forget outside terminal claim state")
	}
	for index := range operation.authorityClaimEdges {
		edge := &operation.authorityClaimEdges[index]
		if edge.held || edge.waiting || edge.key == nil || edge.key.references <= 0 {
			return errors.New("jobmgr claims: invalid terminal edge")
		}
		edge.key.references--
		if edge.key.references == 0 && edge.key.writer == nil && edge.key.readerHead == nil && edge.key.waiterHead == nil {
			if edge.key.settlementIndex >= 0 {
				return errors.New("jobmgr claims: terminal key remains settlement-eligible")
			}
			delete(ca.keys, edge.claim.key)
		}
		edge.key = nil
	}
	operation.claimRegistered = false
	return nil
}

func (ca *claimAuthority) key(name string) *authorityClaimKey {
	state := ca.keys[name]
	if state == nil {
		state = &authorityClaimKey{settlementIndex: -1}
		ca.keys[name] = state
	}
	return state
}
