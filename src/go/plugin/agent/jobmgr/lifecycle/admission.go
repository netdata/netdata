// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
	"sync"
)

const (
	admissionRadixBits  = 28
	inputBodyRecordSlot = 1
)

type ReservationKind uint8

const (
	ReservationOrdinary ReservationKind = iota + 1
	ReservationOrdinaryGrowth
	ReservationCleanup
	ReservationInputBodyGrowth
)

type FrameOutcome uint8

const (
	FrameOutcomeCommitted FrameOutcome = iota + 1
	FrameOutcomeSafeAbort
	FrameOutcomePoisoned
)

type AdmissionLaneRef struct {
	Slot       uint32
	Generation uint32
}

func (ref AdmissionLaneRef) Valid() bool {
	return ref.Slot > 0 && ref.Generation > 0
}

type AdmissionRef struct {
	Slot       uint32
	Generation uint32
}

func (ref AdmissionRef) Valid() bool {
	return ref.Slot > inputBodyRecordSlot && ref.Generation > 0
}

type AdmissionRequestResult struct {
	Ref      AdmissionRef
	Rejected error
}

type AdmissionGrant struct {
	Ref            AdmissionRef
	InputBodyToken uint64
	Kind           ReservationKind
	Bytes          int64
	Lane           AdmissionLaneRef
}

type AdmissionCensus struct {
	Phase            string
	ProcessBytes     int64
	ActiveRecords    int
	OrdinaryWaiting  int
	OrdinaryGranted  int
	OrdinaryBytes    int64
	LongLivedRecords int
	LongLivedBytes   int64
	CleanupWaiting   int
	CleanupGranted   bool
	CleanupRetained  bool
	InputBodyActive  bool
	InputBodyWaiting bool
	InputBodyCarried bool
	InputBodyBytes   int64
	AllocatedRadix   int
	FreeRecords      int
	FreeRadixNodes   int
	RunGeneration    uint64
}

type admissionPhase uint8

const (
	admissionOrdinaryOpen admissionPhase = iota + 1
	admissionCleanupOnly
	admissionClosed
)

type admissionRecordState uint8

const (
	admissionRecordFree admissionRecordState = iota
	admissionOrdinaryWaiting
	admissionOrdinaryGranted
	admissionOrdinaryGrowing
	admissionCleanupWaiting
	admissionCleanupGranted
	admissionCleanupRetained
	admissionInputBodyWaiting
	admissionInputBodyGranted
	admissionInputBodyGrowing
)

type admissionRecord struct {
	generation     uint32
	state          admissionRecordState
	runGeneration  uint64
	lane           AdmissionLaneRef
	bytes          int64
	heldBytes      int64
	longLivedBytes int64
	ordinaryHeld   bool
	growthBytes    int64
	ticket         uint64
	previous       uint32
	next           uint32
	leaf           uint32
}

type admissionRadixNode struct {
	parent    uint32
	children  [2]uint32
	oldest    [2]uint32
	head      uint32
	tail      uint32
	freeNext  uint32
	depth     uint8
	parentBit uint8
}

type admissionLaneUse struct {
	generation uint32
	count      int
}

type AdmissionLedger struct {
	mu sync.Mutex

	phase         admissionPhase
	runGeneration uint64
	nextTicket    uint64

	records        []admissionRecord
	freeRecordHead uint32
	freeRecords    int
	activeRecords  int

	lanes map[uint32]admissionLaneUse

	nodes        []admissionRadixNode
	freeNodeHead uint32
	freeNodes    int

	ordinaryWaiting  int
	ordinaryGranted  int
	ordinaryBytes    int64
	processBytes     int64
	processReserved  bool
	longLivedRecords int
	longLivedBytes   int64

	cleanupHead     uint32
	cleanupTail     uint32
	cleanupWaiting  int
	cleanupGrant    uint32
	cleanupRetained bool

	inputBodyCarried        bool
	inputBodyNextGeneration uint64
}

func NewAdmissionLedger() *AdmissionLedger {
	ledger := &AdmissionLedger{
		phase:   admissionOrdinaryOpen,
		records: make([]admissionRecord, inputBodyRecordSlot+1),
		lanes:   make(map[uint32]admissionLaneUse),
		nodes:   make([]admissionRadixNode, 2),
	}
	ledger.nodes[1].depth = 0
	return ledger
}

func (ledger *AdmissionLedger) ReserveProcessBytes(bytes int64) error {
	if ledger == nil || bytes < 0 || bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr admission: invalid process byte reservation")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.processReserved || ledger.phase != admissionOrdinaryOpen || ledger.runGeneration != 0 || ledger.activeRecords != 0 || ledger.ordinaryWaiting != 0 || ledger.ordinaryGranted != 0 || ledger.ordinaryBytes != 0 {
		return errors.New("jobmgr admission: process bytes must be reserved before run admission")
	}
	ledger.processBytes = bytes
	ledger.processReserved = true
	return nil
}

func (ledger *AdmissionLedger) ReleaseProcessBytes(bytes int64) error {
	if ledger == nil || bytes < 0 {
		return errors.New("jobmgr admission: invalid process byte release")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	pristineConstruction := ledger.phase == admissionOrdinaryOpen && ledger.runGeneration == 0 && ledger.activeRecords == 0 &&
		ledger.ordinaryWaiting == 0 && ledger.ordinaryGranted == 0 && ledger.ordinaryBytes == 0
	closedProcess := ledger.phase == admissionClosed && ledger.activeRecords == 0 && ledger.ordinaryBytes == 0
	if !ledger.processReserved || (!pristineConstruction && !closedProcess) || ledger.processBytes != bytes {
		return errors.New("jobmgr admission: stale or premature process byte release")
	}
	ledger.processBytes = 0
	ledger.processReserved = false
	return nil
}

func (ledger *AdmissionLedger) ordinaryCapacity() int64 {
	return OrdinaryBudgetBytes - ledger.processBytes
}

func (ledger *AdmissionLedger) RequestOrdinary(runGeneration uint64, lane AdmissionLaneRef, bytes int64) AdmissionRequestResult {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if bytes <= 0 || bytes > ledger.ordinaryCapacity() {
		return AdmissionRequestResult{Rejected: fmt.Errorf("jobmgr admission: ordinary bytes %d outside process capacity", bytes)}
	}
	if err := ledger.validateRequest(runGeneration, lane, ReservationOrdinary); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := ledger.allocateRecord(runGeneration, lane, bytes, admissionOrdinaryWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	if err := ledger.insertOrdinary(ref.Slot); err != nil {
		ledger.freeRecord(ref.Slot)
		return AdmissionRequestResult{Rejected: err}
	}
	ledger.ordinaryWaiting++
	return AdmissionRequestResult{Ref: ref}
}

func (ledger *AdmissionLedger) RequestCleanup(runGeneration uint64, lane AdmissionLaneRef, bytes int64) AdmissionRequestResult {
	if bytes <= 0 || bytes > CleanupBudgetBytes {
		return AdmissionRequestResult{Rejected: fmt.Errorf("jobmgr admission: cleanup bytes %d outside 1..%d", bytes, CleanupBudgetBytes)}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if err := ledger.validateRequest(runGeneration, lane, ReservationCleanup); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := ledger.allocateRecord(runGeneration, lane, bytes, admissionCleanupWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ledger.appendCleanup(ref.Slot)
	ledger.cleanupWaiting++
	return AdmissionRequestResult{Ref: ref}
}

func (ledger *AdmissionLedger) RequestInputBodyGrowth(runGeneration, token uint64, nextCapacity int64) (uint64, error) {
	if nextCapacity <= 0 || nextCapacity > MaximumInputBodyBytes {
		return 0, fmt.Errorf("jobmgr admission: input body capacity %d outside 1..%d", nextCapacity, MaximumInputBodyBytes)
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if nextCapacity > ledger.ordinaryCapacity() {
		return 0, errors.New("jobmgr admission: input body exceeds process ordinary capacity")
	}
	if ledger.inputBodyCarried {
		return 0, errors.New("jobmgr admission: input body is suspended between runs")
	}
	if ledger.phase != admissionOrdinaryOpen || runGeneration == 0 || (ledger.runGeneration != 0 && ledger.runGeneration != runGeneration) {
		return 0, errors.New("jobmgr admission: input body rejected by phase")
	}
	record := &ledger.records[inputBodyRecordSlot]
	if token == 0 {
		if record.state != admissionRecordFree {
			return 0, errors.New("jobmgr admission: input body already active")
		}
		record.generation++
		if record.generation == 0 {
			return 0, errors.New("jobmgr admission: input body generation wrapped")
		}
		ledger.nextTicket++
		if ledger.nextTicket == 0 {
			return 0, errors.New("jobmgr admission: ticket sequence wrapped")
		}
		*record = admissionRecord{
			generation: record.generation, state: admissionInputBodyWaiting,
			runGeneration: runGeneration, bytes: nextCapacity, growthBytes: nextCapacity, ticket: ledger.nextTicket,
		}
		ledger.bindRunGeneration(runGeneration)
	} else {
		if uint64(record.generation) != token || record.runGeneration != runGeneration || record.state != admissionInputBodyGranted {
			return 0, errors.New("jobmgr admission: stale input body growth")
		}
		if nextCapacity <= record.heldBytes {
			return 0, errors.New("jobmgr admission: input body growth does not increase capacity")
		}
		ledger.nextTicket++
		if ledger.nextTicket == 0 {
			return 0, errors.New("jobmgr admission: ticket sequence wrapped")
		}
		record.state = admissionInputBodyGrowing
		record.bytes = nextCapacity
		record.growthBytes = nextCapacity
		record.ticket = ledger.nextTicket
	}
	if err := ledger.insertOrdinary(inputBodyRecordSlot); err != nil {
		if token == 0 {
			generation := record.generation
			*record = admissionRecord{generation: generation}
		} else {
			record.state = admissionInputBodyGranted
			record.bytes = 0
			record.growthBytes = 0
		}
		return 0, err
	}
	ledger.ordinaryWaiting++
	return uint64(record.generation), nil
}

func (ledger *AdmissionLedger) CommitInputBodyGrowth(token uint64, capacity int64) (bool, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.inputBodyCarried {
		return false, errors.New("jobmgr admission: input body is suspended between runs")
	}
	record, err := ledger.inputBodyRecord(token)
	if err != nil {
		return false, err
	}
	if record.state != admissionInputBodyGranted || capacity <= 0 || capacity != record.growthBytes || capacity > record.heldBytes {
		return false, errors.New("jobmgr admission: input body growth is not granted")
	}
	oldCapacity := record.heldBytes - capacity
	if oldCapacity < 0 || (oldCapacity == 0 && record.heldBytes != capacity) {
		return false, errors.New("jobmgr admission: invalid input body growth accounting")
	}
	ledger.ordinaryBytes -= oldCapacity
	record.heldBytes = capacity
	record.growthBytes = 0
	return ledger.hasGrantableWork(), nil
}

func (ledger *AdmissionLedger) AbortInputBody(token uint64) (bool, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	record, err := ledger.inputBodyRecord(token)
	if err != nil {
		return false, err
	}
	switch record.state {
	case admissionInputBodyWaiting, admissionInputBodyGrowing:
		ledger.removeOrdinary(inputBodyRecordSlot)
		ledger.ordinaryWaiting--
	case admissionInputBodyGranted:
	default:
		return false, errors.New("jobmgr admission: input body is not abortable")
	}
	ledger.ordinaryBytes -= record.heldBytes
	ledger.resetInputBodyRecord()
	return ledger.hasGrantableWork(), nil
}

func (ledger *AdmissionLedger) SuspendInputBody(runGeneration, nextGeneration, token uint64) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if (ledger.phase != admissionOrdinaryOpen && ledger.phase != admissionCleanupOnly) || runGeneration == 0 || ledger.runGeneration != runGeneration ||
		(nextGeneration != 0 && nextGeneration <= runGeneration) || ledger.inputBodyCarried {
		return errors.New("jobmgr admission: invalid input body suspension")
	}
	record, err := ledger.inputBodyRecord(token)
	if err != nil {
		return err
	}
	if record.state != admissionInputBodyGranted || record.runGeneration != runGeneration || record.heldBytes <= 0 || record.growthBytes != 0 {
		return errors.New("jobmgr admission: input body is not stable for suspension")
	}
	ledger.inputBodyCarried = true
	ledger.inputBodyNextGeneration = nextGeneration
	return nil
}

func (ledger *AdmissionLedger) AdoptInputBody(runGeneration, token uint64) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase != admissionOrdinaryOpen || runGeneration == 0 || ledger.runGeneration != runGeneration ||
		!ledger.inputBodyCarried || ledger.inputBodyNextGeneration != runGeneration {
		return errors.New("jobmgr admission: invalid carried input body adoption")
	}
	record, err := ledger.inputBodyRecord(token)
	if err != nil {
		return err
	}
	if record.state != admissionInputBodyGranted || record.runGeneration != runGeneration || record.heldBytes <= 0 || record.growthBytes != 0 {
		return errors.New("jobmgr admission: carried input body is not stable for adoption")
	}
	ledger.inputBodyCarried = false
	ledger.inputBodyNextGeneration = 0
	return nil
}

func (ledger *AdmissionLedger) RunDrained(runGeneration uint64) bool {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.runDrained(runGeneration)
}

func (ledger *AdmissionLedger) RunFinalizerReady(runGeneration uint64, allowedLongLivedRecords int, allowedLongLivedBytes int64) bool {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase != admissionCleanupOnly || runGeneration == 0 || ledger.runGeneration != runGeneration || allowedLongLivedRecords < 0 || allowedLongLivedBytes < 0 ||
		ledger.activeRecords != 0 || ledger.ordinaryWaiting != 0 || ledger.ordinaryGranted != 0 ||
		ledger.longLivedRecords != allowedLongLivedRecords || ledger.longLivedBytes != allowedLongLivedBytes ||
		ledger.cleanupWaiting != 0 || ledger.cleanupGrant != 0 || ledger.cleanupRetained || ledger.cleanupHead != 0 || ledger.cleanupTail != 0 ||
		ledger.oldestInNode(1) != 0 || !ledger.allOperationRecordsFree() ||
		!ledger.allRadixNodesFree() {
		return false
	}
	if !ledger.inputBodyCarried {
		return ledger.ordinaryBytes == allowedLongLivedBytes && ledger.records[inputBodyRecordSlot].state == admissionRecordFree && ledger.inputBodyNextGeneration == 0
	}
	record := ledger.records[inputBodyRecordSlot]
	return record.state == admissionInputBodyGranted && record.runGeneration == runGeneration && record.heldBytes > 0 && record.growthBytes == 0 &&
		ledger.ordinaryBytes == allowedLongLivedBytes+record.heldBytes
}

func (ledger *AdmissionLedger) TransferInputBody(runGeneration uint64, token uint64, lane AdmissionLaneRef, totalBytes, capacity int64) AdmissionRequestResult {
	if totalBytes <= 0 || totalBytes > OrdinaryBudgetBytes || capacity <= 0 || capacity > MaximumInputBodyBytes || totalBytes <= capacity {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: invalid input body transfer")}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if totalBytes > ledger.ordinaryCapacity() {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body transfer exceeds process ordinary capacity")}
	}
	if ledger.inputBodyCarried {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body is suspended between runs")}
	}
	body, err := ledger.inputBodyRecord(token)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	if body.state != admissionInputBodyGranted || body.runGeneration != runGeneration || body.heldBytes != capacity || body.growthBytes != 0 {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body is not transferable")}
	}
	if err := ledger.validateRequest(runGeneration, lane, ReservationOrdinary); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := ledger.allocateRecord(runGeneration, lane, totalBytes-capacity, admissionOrdinaryWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	record := &ledger.records[ref.Slot]
	record.heldBytes = capacity
	if err := ledger.insertOrdinary(ref.Slot); err != nil {
		ledger.freeRecord(ref.Slot)
		return AdmissionRequestResult{Rejected: err}
	}
	ledger.ordinaryWaiting++
	ledger.resetInputBodyRecord()
	return AdmissionRequestResult{Ref: ref}
}

func (ledger *AdmissionLedger) CancelWaiting(ref AdmissionRef) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	record, err := ledger.record(ref)
	if err != nil {
		return err
	}
	switch record.state {
	case admissionOrdinaryWaiting:
		if record.heldBytes < 0 || record.heldBytes > ledger.ordinaryBytes {
			return errors.New("jobmgr admission: invalid held bytes on waiting request")
		}
		ledger.removeOrdinary(ref.Slot)
		ledger.ordinaryWaiting--
		ledger.ordinaryBytes -= record.heldBytes
		ledger.freeRecord(ref.Slot)
		return nil
	case admissionOrdinaryGrowing:
		ledger.removeOrdinary(ref.Slot)
		ledger.ordinaryWaiting--
		record.state = admissionOrdinaryGranted
		record.bytes = 0
		return nil
	case admissionCleanupWaiting:
		ledger.removeCleanup(ref.Slot)
		ledger.cleanupWaiting--
		ledger.freeRecord(ref.Slot)
		return nil
	default:
		return errors.New("jobmgr admission: request is not waiting")
	}
}

func (ledger *AdmissionLedger) TakeGrants(quantum int, output *[4]AdmissionGrant) (count int, more bool, err error) {
	if output == nil || quantum <= 0 || quantum > len(output) {
		return 0, false, errors.New("jobmgr admission: invalid grant quantum")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase == admissionClosed {
		return 0, false, errors.New("jobmgr admission: ledger is closed")
	}
	for count < quantum {
		if ledger.cleanupGrant == 0 && !ledger.cleanupRetained && ledger.cleanupHead != 0 {
			slot := ledger.cleanupHead
			ledger.removeCleanup(slot)
			record := &ledger.records[slot]
			record.state = admissionCleanupGranted
			ledger.cleanupWaiting--
			ledger.cleanupGrant = slot
			output[count] = ledger.grant(slot, ReservationCleanup)
			count++
			continue
		}
		if ledger.phase != admissionOrdinaryOpen || ledger.ordinaryWaiting == 0 {
			break
		}
		available := ledger.ordinaryCapacity() - ledger.ordinaryBytes
		slot, _ := ledger.oldestFitting(available)
		if slot == 0 {
			break
		}
		ledger.removeOrdinary(slot)
		record := &ledger.records[slot]
		kind := ReservationOrdinary
		if record.state == admissionInputBodyWaiting || record.state == admissionInputBodyGrowing {
			kind = ReservationInputBodyGrowth
			record.state = admissionInputBodyGranted
		} else if record.state == admissionOrdinaryGrowing {
			kind = ReservationOrdinaryGrowth
			record.state = admissionOrdinaryGranted
		} else {
			ledger.ordinaryGranted++
			record.state = admissionOrdinaryGranted
			record.ordinaryHeld = true
		}
		ledger.ordinaryWaiting--
		ledger.ordinaryBytes += record.bytes
		record.heldBytes += record.bytes
		record.bytes = 0
		output[count] = ledger.grant(slot, kind)
		count++
	}
	return count, ledger.hasGrantableWork(), nil
}

// TakeShutdownInputBodyGrant settles the sole parser-owned ordinary waiter
// without granting unrelated ordinary work after run admission has closed.
func (ledger *AdmissionLedger) TakeShutdownInputBodyGrant(runGeneration uint64) (AdmissionGrant, bool, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase == admissionCleanupOnly && runGeneration != 0 && ledger.runGeneration == runGeneration {
		return AdmissionGrant{}, false, nil
	}
	if ledger.phase != admissionOrdinaryOpen || runGeneration == 0 || (ledger.runGeneration != 0 && ledger.runGeneration != runGeneration) {
		return AdmissionGrant{}, false, errors.New("jobmgr admission: invalid shutdown input body service")
	}
	record := &ledger.records[inputBodyRecordSlot]
	if record.state != admissionInputBodyWaiting && record.state != admissionInputBodyGrowing {
		return AdmissionGrant{}, false, nil
	}
	if record.bytes > ledger.ordinaryCapacity()-ledger.ordinaryBytes {
		return AdmissionGrant{}, true, nil
	}
	ledger.removeOrdinary(inputBodyRecordSlot)
	record.state = admissionInputBodyGranted
	ledger.ordinaryWaiting--
	ledger.ordinaryBytes += record.bytes
	record.heldBytes += record.bytes
	record.bytes = 0
	return ledger.grant(inputBodyRecordSlot, ReservationInputBodyGrowth), false, nil
}

func (ledger *AdmissionLedger) ResizeOrdinary(ref AdmissionRef, totalBytes int64) (ready, wake bool, err error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if totalBytes <= 0 || totalBytes > ledger.ordinaryCapacity() {
		return false, false, fmt.Errorf("jobmgr admission: resized ordinary bytes %d outside process capacity", totalBytes)
	}
	record, err := ledger.record(ref)
	if err != nil {
		return false, false, err
	}
	if record.state != admissionOrdinaryGranted ||
		!record.ordinaryHeld ||
		totalBytes <= record.longLivedBytes {
		return false, false, errors.New("jobmgr admission: ordinary resize requires a granted record")
	}
	if totalBytes <= record.heldBytes {
		ledger.ordinaryBytes -= record.heldBytes - totalBytes
		record.heldBytes = totalBytes
		return true, ledger.hasGrantableWork(), nil
	}
	growth := totalBytes - record.heldBytes
	if growth <= ledger.ordinaryCapacity()-ledger.ordinaryBytes {
		record.heldBytes = totalBytes
		ledger.ordinaryBytes += growth
		return true, false, nil
	}
	ledger.nextTicket++
	if ledger.nextTicket == 0 {
		return false, false, errors.New("jobmgr admission: ticket sequence wrapped")
	}
	record.ticket = ledger.nextTicket
	record.bytes = growth
	record.state = admissionOrdinaryGrowing
	if err := ledger.insertOrdinary(ref.Slot); err != nil {
		record.bytes = 0
		record.state = admissionOrdinaryGranted
		return false, false, err
	}
	ledger.ordinaryWaiting++
	return false, false, nil
}

func (ledger *AdmissionLedger) ReleaseOrdinary(ref AdmissionRef) (bool, error) {
	ledger.mu.Lock()
	record, err := ledger.record(ref)
	if err != nil {
		ledger.mu.Unlock()
		return false, err
	}
	if record.state != admissionOrdinaryGranted || !record.ordinaryHeld {
		ledger.mu.Unlock()
		return false, errors.New("jobmgr admission: ordinary request is not granted")
	}
	if record.longLivedBytes != 0 {
		if err := ledger.validateRecordLane(record); err != nil {
			ledger.mu.Unlock()
			return false, err
		}
	}
	ordinaryBytes := record.heldBytes - record.longLivedBytes
	if ordinaryBytes <= 0 {
		ledger.mu.Unlock()
		return false, errors.New("jobmgr admission: ordinary facet has invalid bytes")
	}
	ledger.ordinaryBytes -= ordinaryBytes
	ledger.ordinaryGranted--
	ledger.freeRecord(ref.Slot)
	wake := ledger.hasGrantableWork()
	ledger.mu.Unlock()
	return wake, nil
}

func (ledger *AdmissionLedger) validateRecordLane(record *admissionRecord) error {
	if record == nil || !record.lane.Valid() {
		return errors.New("jobmgr admission: invalid record lane ownership")
	}
	use := ledger.lanes[record.lane.Slot]
	if use.count == 0 || use.generation != record.lane.Generation {
		return errors.New("jobmgr admission: invalid record lane ownership")
	}
	return nil
}

func (ledger *AdmissionLedger) transferLongLived(ref AdmissionRef, bytes int64) error {
	if ledger == nil || bytes <= 0 || bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr admission: invalid long-lived transfer")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	record, err := ledger.record(ref)
	if err != nil {
		return err
	}
	if record.state != admissionOrdinaryGranted || !record.ordinaryHeld || record.longLivedBytes != 0 || bytes >= record.heldBytes {
		return errors.New("jobmgr admission: long-lived transfer requires an untransferred granted record with an ordinary remainder")
	}
	record.longLivedBytes = bytes
	ledger.longLivedRecords++
	ledger.longLivedBytes += bytes
	return nil
}

func (ledger *AdmissionLedger) releaseLongLived(ref AdmissionRef, bytes int64) (bool, error) {
	if ledger == nil || !ref.Valid() || bytes <= 0 {
		return false, errors.New("jobmgr admission: invalid long-lived release")
	}
	ledger.mu.Lock()
	record, recordErr := ledger.record(ref)
	if recordErr == nil {
		if record.state != admissionOrdinaryGranted ||
			record.longLivedBytes != bytes ||
			record.heldBytes < bytes {
			ledger.mu.Unlock()
			return false, errors.New("jobmgr admission: stale or mismatched long-lived release")
		}
		record.heldBytes -= bytes
		record.longLivedBytes = 0
	} else if ledger.longLivedRecords <= 0 ||
		ledger.longLivedBytes < bytes ||
		ledger.ordinaryBytes < bytes {
		ledger.mu.Unlock()
		return false, errors.New("jobmgr admission: stale or mismatched detached long-lived release")
	}
	ledger.ordinaryBytes -= bytes
	ledger.longLivedRecords--
	ledger.longLivedBytes -= bytes
	wake := ledger.hasGrantableWork()
	ledger.mu.Unlock()
	return wake, nil
}

func (ledger *AdmissionLedger) ReleaseCleanup(ref AdmissionRef, outcome FrameOutcome) (bool, error) {
	ledger.mu.Lock()
	record, err := ledger.record(ref)
	if err != nil {
		ledger.mu.Unlock()
		return false, err
	}
	if record.state != admissionCleanupGranted || ledger.cleanupGrant != ref.Slot {
		ledger.mu.Unlock()
		return false, errors.New("jobmgr admission: cleanup request is not the active grant")
	}
	switch outcome {
	case FrameOutcomeCommitted, FrameOutcomeSafeAbort:
		ledger.cleanupGrant = 0
		ledger.freeRecord(ref.Slot)
	case FrameOutcomePoisoned:
		record.state = admissionCleanupRetained
		ledger.cleanupGrant = 0
		ledger.cleanupRetained = true
	default:
		ledger.mu.Unlock()
		return false, errors.New("jobmgr admission: invalid cleanup frame outcome")
	}
	wake := ledger.hasGrantableWork()
	ledger.mu.Unlock()
	return wake, nil
}

func (ledger *AdmissionLedger) BeginCleanupOnly(runGeneration uint64) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase == admissionCleanupOnly && runGeneration != 0 && ledger.runGeneration == runGeneration {
		return nil
	}
	if ledger.phase != admissionOrdinaryOpen {
		return errors.New("jobmgr admission: cleanup-only transition is not available")
	}
	if runGeneration == 0 || (ledger.runGeneration != 0 && ledger.runGeneration != runGeneration) {
		return errors.New("jobmgr admission: stale run generation")
	}
	if ledger.ordinaryWaiting != 0 {
		return errors.New("jobmgr admission: cleanup-only transition with ordinary waiters")
	}
	ledger.bindRunGeneration(runGeneration)
	ledger.phase = admissionCleanupOnly
	return nil
}

func (ledger *AdmissionLedger) CloseDrained(runGeneration uint64) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase != admissionCleanupOnly || runGeneration == 0 || ledger.runGeneration != runGeneration {
		return errors.New("jobmgr admission: invalid drained close")
	}
	if !ledger.drained() {
		return errors.New("jobmgr admission: close with retained state")
	}
	ledger.phase = admissionClosed
	return nil
}

func (ledger *AdmissionLedger) ReopenDrained(completedGeneration, nextGeneration uint64) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.phase != admissionCleanupOnly || completedGeneration == 0 || ledger.runGeneration != completedGeneration || nextGeneration <= completedGeneration {
		return errors.New("jobmgr admission: invalid drained reopen")
	}
	if !ledger.runDrained(completedGeneration) {
		return errors.New("jobmgr admission: reopen with retained state")
	}
	if ledger.inputBodyCarried {
		if ledger.inputBodyNextGeneration == 0 || ledger.inputBodyNextGeneration != nextGeneration {
			return errors.New("jobmgr admission: carried input body targets another generation")
		}
		ledger.records[inputBodyRecordSlot].runGeneration = nextGeneration
	}
	ledger.phase = admissionOrdinaryOpen
	ledger.runGeneration = nextGeneration
	return nil
}

func (ledger *AdmissionLedger) drained() bool {
	return ledger.activeRecords == 0 && ledger.ordinaryWaiting == 0 && ledger.ordinaryGranted == 0 && ledger.ordinaryBytes == 0 &&
		ledger.longLivedRecords == 0 && ledger.longLivedBytes == 0 &&
		ledger.cleanupWaiting == 0 && ledger.cleanupGrant == 0 && !ledger.cleanupRetained && ledger.cleanupHead == 0 && ledger.cleanupTail == 0 &&
		ledger.oldestInNode(1) == 0 && ledger.allOperationRecordsFree() &&
		ledger.allRadixNodesFree() &&
		ledger.records[inputBodyRecordSlot].state == admissionRecordFree && !ledger.inputBodyCarried && ledger.inputBodyNextGeneration == 0
}

func (ledger *AdmissionLedger) runDrained(runGeneration uint64) bool {
	if ledger.phase != admissionCleanupOnly || runGeneration == 0 || ledger.runGeneration != runGeneration || ledger.activeRecords != 0 || ledger.ordinaryWaiting != 0 || ledger.ordinaryGranted != 0 ||
		ledger.longLivedRecords != 0 || ledger.longLivedBytes != 0 || ledger.cleanupWaiting != 0 || ledger.cleanupGrant != 0 || ledger.cleanupRetained ||
		ledger.cleanupHead != 0 || ledger.cleanupTail != 0 || ledger.oldestInNode(1) != 0 ||
		!ledger.allOperationRecordsFree() || !ledger.allRadixNodesFree() {
		return false
	}
	if !ledger.inputBodyCarried {
		return ledger.ordinaryBytes == 0 && ledger.records[inputBodyRecordSlot].state == admissionRecordFree && ledger.inputBodyNextGeneration == 0
	}
	record := ledger.records[inputBodyRecordSlot]
	return record.state == admissionInputBodyGranted && record.runGeneration == runGeneration && record.heldBytes > 0 && record.growthBytes == 0 &&
		ledger.ordinaryBytes == record.heldBytes
}

func (ledger *AdmissionLedger) Census() AdmissionCensus {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return AdmissionCensus{
		Phase: ledger.phase.String(), ProcessBytes: ledger.processBytes, ActiveRecords: ledger.activeRecords,
		OrdinaryWaiting: ledger.ordinaryWaiting, OrdinaryGranted: ledger.ordinaryGranted, OrdinaryBytes: ledger.ordinaryBytes,
		LongLivedRecords: ledger.longLivedRecords, LongLivedBytes: ledger.longLivedBytes,
		CleanupWaiting: ledger.cleanupWaiting, CleanupGranted: ledger.cleanupGrant != 0, CleanupRetained: ledger.cleanupRetained,
		InputBodyActive:  ledger.records[inputBodyRecordSlot].state != admissionRecordFree,
		InputBodyWaiting: ledger.records[inputBodyRecordSlot].state == admissionInputBodyWaiting || ledger.records[inputBodyRecordSlot].state == admissionInputBodyGrowing,
		InputBodyCarried: ledger.inputBodyCarried,
		InputBodyBytes:   ledger.records[inputBodyRecordSlot].heldBytes,
		AllocatedRadix:   len(ledger.nodes) - 2 - ledger.freeNodes,
		FreeRecords:      ledger.freeRecords,
		FreeRadixNodes:   ledger.freeNodes,
		RunGeneration:    ledger.runGeneration,
	}
}

func (ledger *AdmissionLedger) allOperationRecordsFree() bool {
	return ledger.activeRecords == 0 &&
		ledger.freeRecords == len(ledger.records)-inputBodyRecordSlot-1
}

func (ledger *AdmissionLedger) allRadixNodesFree() bool {
	return ledger.freeNodes == len(ledger.nodes)-2
}

func (phase admissionPhase) String() string {
	switch phase {
	case admissionOrdinaryOpen:
		return "ordinary-open"
	case admissionCleanupOnly:
		return "cleanup-only"
	case admissionClosed:
		return "closed"
	default:
		return "invalid"
	}
}

func (ledger *AdmissionLedger) validateRequest(runGeneration uint64, lane AdmissionLaneRef, kind ReservationKind) error {
	if runGeneration == 0 || !lane.Valid() {
		return errors.New("jobmgr admission: invalid request identity")
	}
	if ledger.phase == admissionClosed || (kind == ReservationOrdinary && ledger.phase != admissionOrdinaryOpen) {
		return errors.New("jobmgr admission: request rejected by phase")
	}
	if ledger.runGeneration != 0 && ledger.runGeneration != runGeneration {
		return errors.New("jobmgr admission: stale run generation")
	}
	if use := ledger.lanes[lane.Slot]; use.count != 0 &&
		use.generation != lane.Generation {
		return errors.New("jobmgr admission: stale lane generation")
	}
	ledger.bindRunGeneration(runGeneration)
	return nil
}

func (ledger *AdmissionLedger) bindRunGeneration(runGeneration uint64) {
	if ledger.runGeneration == 0 {
		ledger.runGeneration = runGeneration
	}
}

func (ledger *AdmissionLedger) allocateRecord(runGeneration uint64, lane AdmissionLaneRef, bytes int64, state admissionRecordState) (AdmissionRef, error) {
	slot := ledger.freeRecordHead
	if slot == 0 {
		if uint64(len(ledger.records)) > uint64(^uint32(0)) {
			return AdmissionRef{}, errors.New(
				"jobmgr admission: record reference space exhausted",
			)
		}
		slot = uint32(len(ledger.records))
		ledger.records = append(ledger.records, admissionRecord{})
	} else {
		ledger.freeRecordHead = ledger.records[slot].next
		ledger.freeRecords--
	}
	record := &ledger.records[slot]
	generation := record.generation + 1
	if generation == 0 {
		*record = admissionRecord{
			generation: record.generation,
			next:       ledger.freeRecordHead,
		}
		ledger.freeRecordHead = slot
		ledger.freeRecords++
		return AdmissionRef{}, errors.New("jobmgr admission: record generation wrapped")
	}
	ledger.nextTicket++
	if ledger.nextTicket == 0 {
		*record = admissionRecord{
			generation: record.generation,
			next:       ledger.freeRecordHead,
		}
		ledger.freeRecordHead = slot
		ledger.freeRecords++
		return AdmissionRef{}, errors.New("jobmgr admission: ticket sequence wrapped")
	}
	*record = admissionRecord{
		generation: generation, state: state, runGeneration: runGeneration,
		lane: lane, bytes: bytes, ticket: ledger.nextTicket,
	}
	use := ledger.lanes[lane.Slot]
	if use.count == 0 {
		use.generation = lane.Generation
	}
	use.count++
	ledger.lanes[lane.Slot] = use
	ledger.activeRecords++
	return AdmissionRef{Slot: slot, Generation: generation}, nil
}

func (ledger *AdmissionLedger) freeRecord(slot uint32) {
	record := &ledger.records[slot]
	generation := record.generation
	lane := record.lane
	if lane.Valid() {
		use := ledger.lanes[lane.Slot]
		if use.count <= 0 || use.generation != lane.Generation {
			panic("jobmgr admission: invalid lane record release")
		}
		use.count--
		if use.count == 0 {
			delete(ledger.lanes, lane.Slot)
		} else {
			ledger.lanes[lane.Slot] = use
		}
	}
	*record = admissionRecord{generation: generation, next: ledger.freeRecordHead}
	ledger.freeRecordHead = slot
	ledger.freeRecords++
	ledger.activeRecords--
}

func (ledger *AdmissionLedger) record(ref AdmissionRef) (*admissionRecord, error) {
	if !ref.Valid() || uint64(ref.Slot) >= uint64(len(ledger.records)) {
		return nil, errors.New("jobmgr admission: invalid request reference")
	}
	record := &ledger.records[ref.Slot]
	if record.state == admissionRecordFree || record.generation != ref.Generation {
		return nil, errors.New("jobmgr admission: stale request reference")
	}
	return record, nil
}

func (ledger *AdmissionLedger) inputBodyRecord(token uint64) (*admissionRecord, error) {
	record := &ledger.records[inputBodyRecordSlot]
	if token == 0 || record.state == admissionRecordFree || uint64(record.generation) != token {
		return nil, errors.New("jobmgr admission: stale input body reference")
	}
	return record, nil
}

func (ledger *AdmissionLedger) resetInputBodyRecord() {
	record := &ledger.records[inputBodyRecordSlot]
	generation := record.generation
	*record = admissionRecord{generation: generation}
	ledger.inputBodyCarried = false
	ledger.inputBodyNextGeneration = 0
}

func (ledger *AdmissionLedger) appendCleanup(slot uint32) {
	record := &ledger.records[slot]
	record.previous = ledger.cleanupTail
	if ledger.cleanupTail == 0 {
		ledger.cleanupHead = slot
	} else {
		ledger.records[ledger.cleanupTail].next = slot
	}
	ledger.cleanupTail = slot
}

func (ledger *AdmissionLedger) removeCleanup(slot uint32) {
	record := &ledger.records[slot]
	if record.previous == 0 {
		ledger.cleanupHead = record.next
	} else {
		ledger.records[record.previous].next = record.next
	}
	if record.next == 0 {
		ledger.cleanupTail = record.previous
	} else {
		ledger.records[record.next].previous = record.previous
	}
	record.previous = 0
	record.next = 0
}

func (ledger *AdmissionLedger) insertOrdinary(slot uint32) error {
	record := &ledger.records[slot]
	nodeIndex := uint32(1)
	value := uint32(record.bytes)
	for depth := 0; depth < admissionRadixBits; depth++ {
		bit := uint8((value >> uint(admissionRadixBits-1-depth)) & 1)
		node := &ledger.nodes[nodeIndex]
		child := node.children[bit]
		if child == 0 {
			var err error
			child, err = ledger.allocateNode(nodeIndex, bit, uint8(depth+1))
			if err != nil {
				return err
			}
			ledger.nodes[nodeIndex].children[bit] = child
		}
		nodeIndex = child
	}
	leaf := &ledger.nodes[nodeIndex]
	record.leaf = nodeIndex
	record.previous = leaf.tail
	if leaf.tail == 0 {
		leaf.head = slot
	} else {
		ledger.records[leaf.tail].next = slot
	}
	leaf.tail = slot
	ledger.refreshAncestors(nodeIndex)
	return nil
}

func (ledger *AdmissionLedger) removeOrdinary(slot uint32) {
	record := &ledger.records[slot]
	leafIndex := record.leaf
	leaf := &ledger.nodes[leafIndex]
	headChanged := leaf.head == slot
	if record.previous == 0 {
		leaf.head = record.next
	} else {
		ledger.records[record.previous].next = record.next
	}
	if record.next == 0 {
		leaf.tail = record.previous
	} else {
		ledger.records[record.next].previous = record.previous
	}
	record.previous = 0
	record.next = 0
	record.leaf = 0
	if !headChanged {
		return
	}
	current := leafIndex
	for current != 1 && ledger.nodeEmpty(current) {
		node := ledger.nodes[current]
		parent := &ledger.nodes[node.parent]
		parent.children[node.parentBit] = 0
		parent.oldest[node.parentBit] = 0
		ledger.freeNode(current)
		current = node.parent
	}
	ledger.refreshAncestors(current)
}

func (ledger *AdmissionLedger) allocateNode(parent uint32, parentBit, depth uint8) (uint32, error) {
	index := ledger.freeNodeHead
	if index == 0 {
		if uint64(len(ledger.nodes)) > uint64(^uint32(0)) {
			return 0, errors.New(
				"jobmgr admission: radix reference space exhausted",
			)
		}
		index = uint32(len(ledger.nodes))
		ledger.nodes = append(ledger.nodes, admissionRadixNode{})
	} else {
		ledger.freeNodeHead = ledger.nodes[index].freeNext
		ledger.freeNodes--
	}
	ledger.nodes[index] = admissionRadixNode{parent: parent, parentBit: parentBit, depth: depth}
	return index, nil
}

func (ledger *AdmissionLedger) freeNode(index uint32) {
	ledger.nodes[index] = admissionRadixNode{freeNext: ledger.freeNodeHead}
	ledger.freeNodeHead = index
	ledger.freeNodes++
}

func (ledger *AdmissionLedger) nodeEmpty(index uint32) bool {
	node := ledger.nodes[index]
	return node.head == 0 && node.children[0] == 0 && node.children[1] == 0
}

func (ledger *AdmissionLedger) refreshAncestors(childIndex uint32) {
	for childIndex != 1 {
		child := &ledger.nodes[childIndex]
		parent := &ledger.nodes[child.parent]
		parent.oldest[child.parentBit] = ledger.oldestInNode(childIndex)
		childIndex = child.parent
	}
}

func (ledger *AdmissionLedger) oldestInNode(index uint32) uint32 {
	if index == 0 {
		return 0
	}
	node := &ledger.nodes[index]
	if node.depth == admissionRadixBits {
		return node.head
	}
	return ledger.older(node.oldest[0], node.oldest[1])
}

func (ledger *AdmissionLedger) older(first, second uint32) uint32 {
	if first == 0 {
		return second
	}
	if second == 0 || ledger.records[first].ticket < ledger.records[second].ticket {
		return first
	}
	return second
}

func (ledger *AdmissionLedger) oldestFitting(available int64) (slot uint32, visited int) {
	if available <= 0 || ledger.ordinaryWaiting == 0 {
		return 0, 0
	}
	if available > OrdinaryBudgetBytes {
		available = OrdinaryBudgetBytes
	}
	value := uint32(available)
	nodeIndex := uint32(1)
	visited = 1
	for depth := 0; depth < admissionRadixBits; depth++ {
		node := &ledger.nodes[nodeIndex]
		bit := uint8((value >> uint(admissionRadixBits-1-depth)) & 1)
		if bit == 1 {
			slot = ledger.older(slot, node.oldest[0])
		}
		child := node.children[bit]
		if child == 0 {
			return slot, visited
		}
		nodeIndex = child
		visited++
	}
	slot = ledger.older(slot, ledger.nodes[nodeIndex].head)
	return slot, visited
}

func (ledger *AdmissionLedger) grant(slot uint32, kind ReservationKind) AdmissionGrant {
	record := ledger.records[slot]
	if kind == ReservationInputBodyGrowth {
		return AdmissionGrant{InputBodyToken: uint64(record.generation), Kind: kind, Bytes: record.heldBytes}
	}
	return AdmissionGrant{
		Ref:  AdmissionRef{Slot: slot, Generation: record.generation},
		Kind: kind, Bytes: record.heldBytes, Lane: record.lane,
	}
}

func (ledger *AdmissionLedger) hasGrantableWork() bool {
	if ledger.phase == admissionClosed {
		return false
	}
	if ledger.cleanupGrant == 0 && !ledger.cleanupRetained && ledger.cleanupHead != 0 {
		return true
	}
	if ledger.phase != admissionOrdinaryOpen || ledger.ordinaryWaiting == 0 {
		return false
	}
	slot, _ := ledger.oldestFitting(ledger.ordinaryCapacity() - ledger.ordinaryBytes)
	return slot != 0
}
