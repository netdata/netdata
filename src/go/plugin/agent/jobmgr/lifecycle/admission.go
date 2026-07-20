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
	Ref            AdmissionRef     // reservation this grant satisfies
	InputBodyToken uint64           // input-body token when Kind is an input-body growth grant
	Kind           ReservationKind  // reservation kind (ordinary / cleanup / input-body growth)
	Bytes          int64            // granted byte amount
	Lane           AdmissionLaneRef // admission lane the reservation belongs to
}

type AdmissionCensus struct {
	Phase             string
	ProcessBytes      int64
	ActiveRecords     int
	OrdinaryWaiting   int
	OrdinaryGranted   int
	OrdinarySuspended int
	OrdinaryBytes     int64
	LongLivedRecords  int
	LongLivedBytes    int64
	CleanupWaiting    int
	CleanupGranted    bool
	CleanupRetained   bool
	InputBodyActive   bool
	InputBodyWaiting  bool
	InputBodyCarried  bool
	InputBodyBytes    int64
	AllocatedRadix    int
	FreeRecords       int
	FreeRadixNodes    int
	RunGeneration     uint64
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
	admissionOrdinarySuspended
	admissionCleanupWaiting
	admissionCleanupGranted
	admissionCleanupRetained
	admissionInputBodyWaiting
	admissionInputBodyGranted
	admissionInputBodyGrowing
)

type admissionRecord struct {
	generation     uint32               // ABA guard; paired with the slot to form an AdmissionRef
	state          admissionRecordState // reservation state-machine node this record occupies
	runGeneration  uint64               // owning run; used to reject stale-generation reservations
	lane           AdmissionLaneRef     // owning admission lane
	bytes          int64                // bytes still waiting to be granted
	heldBytes      int64                // bytes currently charged to ordinaryBytes
	longLivedBytes int64                // portion of heldBytes transferred to a long-lived permit
	ordinaryHeld   bool                 // record counts toward ordinaryGranted
	growthBytes    int64                // pending input-body/ordinary growth delta
	ticket         uint64               // monotonic FIFO tiebreaker within a radix leaf
	previous       uint32               // intrusive list link (cleanup queue or radix leaf)
	next           uint32               // intrusive list link (cleanup queue or radix leaf)
	leaf           uint32               // radix node index this record is filed under
}

type admissionRadixNode struct {
	parent    uint32    // parent node index
	children  [2]uint32 // child node indices for the next byte bit (0/1)
	oldest    [2]uint32 // cached oldest-ticket slot reachable through each child
	head      uint32    // record list head at a leaf
	tail      uint32    // record list tail at a leaf
	freeNext  uint32    // freelist link when the node is recycled
	depth     uint8     // bit depth; == admissionRadixBits marks a leaf
	parentBit uint8     // which child slot of the parent this node is
}

type admissionLaneUse struct {
	generation uint32
	count      int
}

type AdmissionLedger struct {
	lanes                   map[uint32]admissionLaneUse // per-lane use tracking (refcounts)
	records                 []admissionRecord           // reservation record storage (freelist-backed)
	nodes                   []admissionRadixNode        // byte-indexed radix tree over ordinary reservations
	ordinaryWaiting         int                         // count of ordinary reservations awaiting a grant
	longLivedBytes          int64                       // total bytes retained by long-lived permits
	inputBodyNextGeneration uint64                      // next input-body reservation generation
	freeRecords             int                         // number of free record slots
	activeRecords           int                         // number of active reservation records
	runGeneration           uint64                      // current run generation (stale reservations rejected)
	cleanupWaiting          int                         // count of cleanup reservations awaiting the single grant
	nextTicket              uint64                      // next FIFO ticket to assign within a radix leaf
	freeNodes               int                         // number of free radix nodes
	longLivedRecords        int                         // count of records carrying long-lived bytes
	ordinaryGranted         int                         // count of currently granted ordinary reservations
	ordinarySuspended       int                         // count of granted ordinary reservations suspended (growing)
	ordinaryBytes           int64                       // ordinary bytes currently charged
	processBytes            int64                       // bytes reserved for process-lifetime storage
	mu                      sync.Mutex                  // guards all fields
	freeNodeHead            uint32                      // head of the radix-node freelist
	cleanupHead             uint32                      // head of the cleanup reservation FIFO
	cleanupTail             uint32                      // tail of the cleanup reservation FIFO
	cleanupGrant            uint32                      // record holding the single active cleanup grant
	freeRecordHead          uint32                      // head of the record freelist
	processReserved         bool                        // process byte reservation has been taken
	phase                   admissionPhase              // admission phase (open / draining / closed)
	cleanupRetained         bool                        // the cleanup grant is retained across a generation
	inputBodyCarried        bool                        // the input-body reservation is carried across a generation
}

func NewAdmissionLedger() *AdmissionLedger {
	ledger := &AdmissionLedger{
		phase:   admissionOrdinaryOpen,
		records: make([]admissionRecord, inputBodyRecordSlot+1),
		lanes:   make(map[uint32]admissionLaneUse),
		nodes:   make([]admissionRadixNode, 2),
	}
	return ledger
}

func (al *AdmissionLedger) ReserveProcessBytes(bytes int64) error {
	if al == nil || bytes < 0 || bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr admission: invalid process byte reservation")
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.processReserved || al.phase != admissionOrdinaryOpen || al.runGeneration != 0 || al.activeRecords != 0 || al.ordinaryWaiting != 0 || al.ordinaryGranted != 0 || al.ordinarySuspended != 0 || al.ordinaryBytes != 0 {
		return errors.New("jobmgr admission: process bytes must be reserved before run admission")
	}
	al.processBytes = bytes
	al.processReserved = true
	return nil
}

func (al *AdmissionLedger) ReleaseProcessBytes(bytes int64) error {
	if al == nil || bytes < 0 {
		return errors.New("jobmgr admission: invalid process byte release")
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	pristineConstruction := al.phase == admissionOrdinaryOpen && al.runGeneration == 0 && al.activeRecords == 0 &&
		al.ordinaryWaiting == 0 && al.ordinaryGranted == 0 &&
		al.ordinarySuspended == 0 && al.ordinaryBytes == 0
	closedProcess := al.phase == admissionClosed && al.activeRecords == 0 && al.ordinaryBytes == 0
	if !al.processReserved || (!pristineConstruction && !closedProcess) || al.processBytes != bytes {
		return errors.New("jobmgr admission: stale or premature process byte release")
	}
	al.processBytes = 0
	al.processReserved = false
	return nil
}

func (al *AdmissionLedger) ordinaryCapacity() int64 {
	return OrdinaryBudgetBytes - al.processBytes
}

func (al *AdmissionLedger) RequestOrdinary(runGeneration uint64, lane AdmissionLaneRef, bytes int64) AdmissionRequestResult {
	al.mu.Lock()
	defer al.mu.Unlock()
	if bytes <= 0 || bytes > al.ordinaryCapacity() {
		return AdmissionRequestResult{Rejected: fmt.Errorf("jobmgr admission: ordinary bytes %d outside process capacity", bytes)}
	}
	if err := al.validateRequest(runGeneration, lane, ReservationOrdinary); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := al.allocateRecord(runGeneration, lane, bytes, admissionOrdinaryWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	if err := al.insertOrdinary(ref.Slot); err != nil {
		al.freeRecord(ref.Slot)
		return AdmissionRequestResult{Rejected: err}
	}
	al.ordinaryWaiting++
	return AdmissionRequestResult{Ref: ref}
}

// GrantCompositeProgress admits one parent-owned child even when the ordinary
// budget is fully held. The parent/child lifecycle guarantees that this
// temporary overcommit is reclaimed before the parent can complete.
func (al *AdmissionLedger) GrantCompositeProgress(
	runGeneration uint64,
	parent AdmissionRef,
	lane AdmissionLaneRef,
	bytes int64,
) AdmissionRequestResult {
	al.mu.Lock()
	defer al.mu.Unlock()
	if bytes <= 0 || bytes > al.ordinaryCapacity() {
		return AdmissionRequestResult{Rejected: fmt.Errorf(
			"jobmgr admission: composite bytes %d outside process capacity",
			bytes,
		)}
	}
	parentRecord, err := al.record(parent)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	if parentRecord.runGeneration != runGeneration ||
		parentRecord.state != admissionOrdinaryGranted ||
		!parentRecord.ordinaryHeld {
		return AdmissionRequestResult{Rejected: errors.New(
			"jobmgr admission: composite parent is not granted",
		)}
	}
	if err := al.validateRequest(
		runGeneration,
		lane,
		ReservationOrdinary,
	); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := al.allocateRecord(
		runGeneration,
		lane,
		0,
		admissionOrdinaryGranted,
	)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	record := &al.records[ref.Slot]
	record.heldBytes = bytes
	record.ordinaryHeld = true
	al.ordinaryGranted++
	al.ordinaryBytes += bytes
	return AdmissionRequestResult{Ref: ref}
}

func (al *AdmissionLedger) RequestCleanup(runGeneration uint64, lane AdmissionLaneRef, bytes int64) AdmissionRequestResult {
	if bytes <= 0 || bytes > CleanupBudgetBytes {
		return AdmissionRequestResult{Rejected: fmt.Errorf("jobmgr admission: cleanup bytes %d outside 1..%d", bytes, CleanupBudgetBytes)}
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if err := al.validateRequest(runGeneration, lane, ReservationCleanup); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := al.allocateRecord(runGeneration, lane, bytes, admissionCleanupWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	al.appendCleanup(ref.Slot)
	al.cleanupWaiting++
	return AdmissionRequestResult{Ref: ref}
}

func (al *AdmissionLedger) RequestInputBodyGrowth(runGeneration, token uint64, nextCapacity int64) (uint64, error) {
	if nextCapacity <= 0 || nextCapacity > MaximumInputBodyBytes {
		return 0, fmt.Errorf("jobmgr admission: input body capacity %d outside 1..%d", nextCapacity, MaximumInputBodyBytes)
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if nextCapacity > al.ordinaryCapacity() {
		return 0, errors.New("jobmgr admission: input body exceeds process ordinary capacity")
	}
	if al.inputBodyCarried {
		return 0, errors.New("jobmgr admission: input body is suspended between runs")
	}
	if al.phase != admissionOrdinaryOpen || runGeneration == 0 || (al.runGeneration != 0 && al.runGeneration != runGeneration) {
		return 0, errors.New("jobmgr admission: input body rejected by phase")
	}
	record := &al.records[inputBodyRecordSlot]
	if token == 0 {
		if record.state != admissionRecordFree {
			return 0, errors.New("jobmgr admission: input body already active")
		}
		record.generation++
		if record.generation == 0 {
			return 0, errors.New("jobmgr admission: input body generation wrapped")
		}
		al.nextTicket++
		if al.nextTicket == 0 {
			return 0, errors.New("jobmgr admission: ticket sequence wrapped")
		}
		*record = admissionRecord{
			generation: record.generation, state: admissionInputBodyWaiting,
			runGeneration: runGeneration, bytes: nextCapacity, growthBytes: nextCapacity, ticket: al.nextTicket,
		}
		al.bindRunGeneration(runGeneration)
	} else {
		if uint64(record.generation) != token || record.runGeneration != runGeneration || record.state != admissionInputBodyGranted {
			return 0, errors.New("jobmgr admission: stale input body growth")
		}
		if nextCapacity <= record.heldBytes {
			return 0, errors.New("jobmgr admission: input body growth does not increase capacity")
		}
		al.nextTicket++
		if al.nextTicket == 0 {
			return 0, errors.New("jobmgr admission: ticket sequence wrapped")
		}
		record.state = admissionInputBodyGrowing
		record.bytes = nextCapacity
		record.growthBytes = nextCapacity
		record.ticket = al.nextTicket
	}
	if err := al.insertOrdinary(inputBodyRecordSlot); err != nil {
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
	al.ordinaryWaiting++
	return uint64(record.generation), nil
}

func (al *AdmissionLedger) CommitInputBodyGrowth(token uint64, capacity int64) (bool, error) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.inputBodyCarried {
		return false, errors.New("jobmgr admission: input body is suspended between runs")
	}
	record, err := al.inputBodyRecord(token)
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
	al.ordinaryBytes -= oldCapacity
	record.heldBytes = capacity
	record.growthBytes = 0
	return al.hasGrantableWork(), nil
}

func (al *AdmissionLedger) AbortInputBody(token uint64) (bool, error) {
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.inputBodyRecord(token)
	if err != nil {
		return false, err
	}
	switch record.state {
	case admissionInputBodyWaiting, admissionInputBodyGrowing:
		al.removeOrdinary(inputBodyRecordSlot)
		al.ordinaryWaiting--
	case admissionInputBodyGranted:
	default:
		return false, errors.New("jobmgr admission: input body is not abortable")
	}
	al.ordinaryBytes -= record.heldBytes
	al.resetInputBodyRecord()
	return al.hasGrantableWork(), nil
}

func (al *AdmissionLedger) SuspendInputBody(runGeneration, nextGeneration, token uint64) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if (al.phase != admissionOrdinaryOpen && al.phase != admissionCleanupOnly) || runGeneration == 0 || al.runGeneration != runGeneration ||
		(nextGeneration != 0 && nextGeneration <= runGeneration) || al.inputBodyCarried {
		return errors.New("jobmgr admission: invalid input body suspension")
	}
	record, err := al.inputBodyRecord(token)
	if err != nil {
		return err
	}
	if record.state != admissionInputBodyGranted || record.runGeneration != runGeneration || record.heldBytes <= 0 || record.growthBytes != 0 {
		return errors.New("jobmgr admission: input body is not stable for suspension")
	}
	al.inputBodyCarried = true
	al.inputBodyNextGeneration = nextGeneration
	return nil
}

func (al *AdmissionLedger) AdoptInputBody(runGeneration, token uint64) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase != admissionOrdinaryOpen || runGeneration == 0 || al.runGeneration != runGeneration ||
		!al.inputBodyCarried || al.inputBodyNextGeneration != runGeneration {
		return errors.New("jobmgr admission: invalid carried input body adoption")
	}
	record, err := al.inputBodyRecord(token)
	if err != nil {
		return err
	}
	if record.state != admissionInputBodyGranted || record.runGeneration != runGeneration || record.heldBytes <= 0 || record.growthBytes != 0 {
		return errors.New("jobmgr admission: carried input body is not stable for adoption")
	}
	al.inputBodyCarried = false
	al.inputBodyNextGeneration = 0
	return nil
}

func (al *AdmissionLedger) RunDrained(runGeneration uint64) bool {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.runDrained(runGeneration)
}

func (al *AdmissionLedger) RunFinalizerReady(runGeneration uint64, allowedLongLivedRecords int, allowedLongLivedBytes int64) bool {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase != admissionCleanupOnly || runGeneration == 0 || al.runGeneration != runGeneration || allowedLongLivedRecords < 0 || allowedLongLivedBytes < 0 ||
		al.activeRecords != 0 || al.ordinaryWaiting != 0 || al.ordinaryGranted != 0 || al.ordinarySuspended != 0 ||
		al.longLivedRecords != allowedLongLivedRecords || al.longLivedBytes != allowedLongLivedBytes ||
		al.cleanupWaiting != 0 || al.cleanupGrant != 0 || al.cleanupRetained || al.cleanupHead != 0 || al.cleanupTail != 0 ||
		al.oldestInNode(1) != 0 || !al.allOperationRecordsFree() ||
		!al.allRadixNodesFree() {
		return false
	}
	if !al.inputBodyCarried {
		return al.ordinaryBytes == allowedLongLivedBytes && al.records[inputBodyRecordSlot].state == admissionRecordFree && al.inputBodyNextGeneration == 0
	}
	record := al.records[inputBodyRecordSlot]
	return record.state == admissionInputBodyGranted && record.runGeneration == runGeneration && record.heldBytes > 0 && record.growthBytes == 0 &&
		al.ordinaryBytes == allowedLongLivedBytes+record.heldBytes
}

func (al *AdmissionLedger) TransferInputBody(runGeneration uint64, token uint64, lane AdmissionLaneRef, totalBytes, capacity int64) AdmissionRequestResult {
	if totalBytes <= 0 || totalBytes > OrdinaryBudgetBytes || capacity <= 0 || capacity > MaximumInputBodyBytes || totalBytes <= capacity {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: invalid input body transfer")}
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if totalBytes > al.ordinaryCapacity() {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body transfer exceeds process ordinary capacity")}
	}
	if al.inputBodyCarried {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body is suspended between runs")}
	}
	body, err := al.inputBodyRecord(token)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	if body.state != admissionInputBodyGranted || body.runGeneration != runGeneration || body.heldBytes != capacity || body.growthBytes != 0 {
		return AdmissionRequestResult{Rejected: errors.New("jobmgr admission: input body is not transferable")}
	}
	if err := al.validateRequest(runGeneration, lane, ReservationOrdinary); err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	ref, err := al.allocateRecord(runGeneration, lane, totalBytes-capacity, admissionOrdinaryWaiting)
	if err != nil {
		return AdmissionRequestResult{Rejected: err}
	}
	record := &al.records[ref.Slot]
	record.heldBytes = capacity
	if err := al.insertOrdinary(ref.Slot); err != nil {
		al.freeRecord(ref.Slot)
		return AdmissionRequestResult{Rejected: err}
	}
	al.ordinaryWaiting++
	al.resetInputBodyRecord()
	return AdmissionRequestResult{Ref: ref}
}

func (al *AdmissionLedger) CancelWaiting(ref AdmissionRef) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.record(ref)
	if err != nil {
		return err
	}
	switch record.state {
	case admissionOrdinaryWaiting:
		if record.heldBytes < 0 || record.heldBytes > al.ordinaryBytes {
			return errors.New("jobmgr admission: invalid held bytes on waiting request")
		}
		al.removeOrdinary(ref.Slot)
		al.ordinaryWaiting--
		al.ordinaryBytes -= record.heldBytes
		al.freeRecord(ref.Slot)
		return nil
	case admissionOrdinaryGrowing:
		al.removeOrdinary(ref.Slot)
		al.ordinaryWaiting--
		record.state = admissionOrdinaryGranted
		record.bytes = 0
		return nil
	case admissionOrdinarySuspended:
		if record.heldBytes < 0 ||
			record.heldBytes > al.ordinaryBytes {
			return errors.New(
				"jobmgr admission: invalid held bytes on suspended request",
			)
		}
		al.ordinarySuspended--
		al.ordinaryBytes -= record.heldBytes
		al.freeRecord(ref.Slot)
		return nil
	case admissionCleanupWaiting:
		al.removeCleanup(ref.Slot)
		al.cleanupWaiting--
		al.freeRecord(ref.Slot)
		return nil
	default:
		return errors.New("jobmgr admission: request is not waiting")
	}
}

// SuspendOrdinary makes a granted operation admission-ineligible while
// retaining bytes that already exist independently of its execution grant.
func (al *AdmissionLedger) SuspendOrdinary(
	ref AdmissionRef,
	retainedBytes int64,
) (bool, error) {
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.record(ref)
	if err != nil {
		return false, err
	}
	if record.state != admissionOrdinaryGranted ||
		!record.ordinaryHeld ||
		record.longLivedBytes != 0 ||
		retainedBytes < 0 ||
		retainedBytes >= record.heldBytes {
		return false, errors.New(
			"jobmgr admission: ordinary request is not suspendable",
		)
	}
	released := record.heldBytes - retainedBytes
	record.state = admissionOrdinarySuspended
	record.bytes = released
	record.heldBytes = retainedBytes
	record.ordinaryHeld = false
	al.ordinaryGranted--
	al.ordinarySuspended++
	al.ordinaryBytes -= released
	return al.hasGrantableWork(), nil
}

// ResumeOrdinary returns a suspended operation to its original ordered
// admission domain without allocating a replacement ownership record.
func (al *AdmissionLedger) ResumeOrdinary(
	ref AdmissionRef,
) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.record(ref)
	if err != nil {
		return err
	}
	if al.phase != admissionOrdinaryOpen ||
		record.state != admissionOrdinarySuspended ||
		record.ordinaryHeld ||
		record.longLivedBytes != 0 ||
		record.bytes <= 0 ||
		record.heldBytes < 0 {
		return errors.New(
			"jobmgr admission: ordinary request is not resumable",
		)
	}
	al.nextTicket++
	if al.nextTicket == 0 {
		return errors.New(
			"jobmgr admission: ticket sequence wrapped",
		)
	}
	record.ticket = al.nextTicket
	record.state = admissionOrdinaryWaiting
	if err := al.insertOrdinary(ref.Slot); err != nil {
		record.state = admissionOrdinarySuspended
		return err
	}
	al.ordinarySuspended--
	al.ordinaryWaiting++
	return nil
}

func (al *AdmissionLedger) TakeGrants(quantum int, output *[4]AdmissionGrant) (count int, more bool, err error) {
	if output == nil || quantum <= 0 || quantum > len(output) {
		return 0, false, errors.New("jobmgr admission: invalid grant quantum")
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase == admissionClosed {
		return 0, false, errors.New("jobmgr admission: ledger is closed")
	}
	for count < quantum {
		if al.cleanupGrant == 0 && !al.cleanupRetained && al.cleanupHead != 0 {
			slot := al.cleanupHead
			al.removeCleanup(slot)
			record := &al.records[slot]
			record.state = admissionCleanupGranted
			al.cleanupWaiting--
			al.cleanupGrant = slot
			output[count] = al.grant(slot, ReservationCleanup)
			count++
			continue
		}
		if al.phase != admissionOrdinaryOpen || al.ordinaryWaiting == 0 {
			break
		}
		available := al.ordinaryCapacity() - al.ordinaryBytes
		slot := al.oldestFitting(available)
		if slot == 0 {
			break
		}
		al.removeOrdinary(slot)
		record := &al.records[slot]
		kind := ReservationOrdinary
		if record.state == admissionInputBodyWaiting || record.state == admissionInputBodyGrowing {
			kind = ReservationInputBodyGrowth
			record.state = admissionInputBodyGranted
		} else if record.state == admissionOrdinaryGrowing {
			kind = ReservationOrdinaryGrowth
			record.state = admissionOrdinaryGranted
		} else {
			al.ordinaryGranted++
			record.state = admissionOrdinaryGranted
			record.ordinaryHeld = true
		}
		al.ordinaryWaiting--
		al.ordinaryBytes += record.bytes
		record.heldBytes += record.bytes
		record.bytes = 0
		output[count] = al.grant(slot, kind)
		count++
	}
	return count, al.hasGrantableWork(), nil
}

// TakeShutdownInputBodyGrant settles the sole parser-owned ordinary waiter
// without granting unrelated ordinary work after run admission has closed.
func (al *AdmissionLedger) TakeShutdownInputBodyGrant(runGeneration uint64) (AdmissionGrant, bool, error) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase == admissionCleanupOnly && runGeneration != 0 && al.runGeneration == runGeneration {
		return AdmissionGrant{}, false, nil
	}
	if al.phase != admissionOrdinaryOpen || runGeneration == 0 || (al.runGeneration != 0 && al.runGeneration != runGeneration) {
		return AdmissionGrant{}, false, errors.New("jobmgr admission: invalid shutdown input body service")
	}
	record := &al.records[inputBodyRecordSlot]
	if record.state != admissionInputBodyWaiting && record.state != admissionInputBodyGrowing {
		return AdmissionGrant{}, false, nil
	}
	if record.bytes > al.ordinaryCapacity()-al.ordinaryBytes {
		return AdmissionGrant{}, true, nil
	}
	al.removeOrdinary(inputBodyRecordSlot)
	record.state = admissionInputBodyGranted
	al.ordinaryWaiting--
	al.ordinaryBytes += record.bytes
	record.heldBytes += record.bytes
	record.bytes = 0
	return al.grant(inputBodyRecordSlot, ReservationInputBodyGrowth), false, nil
}

func (al *AdmissionLedger) ResizeOrdinary(ref AdmissionRef, totalBytes int64) (ready, wake bool, err error) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if totalBytes <= 0 || totalBytes > al.ordinaryCapacity() {
		return false, false, fmt.Errorf("jobmgr admission: resized ordinary bytes %d outside process capacity", totalBytes)
	}
	record, err := al.record(ref)
	if err != nil {
		return false, false, err
	}
	if record.state != admissionOrdinaryGranted ||
		!record.ordinaryHeld ||
		totalBytes <= record.longLivedBytes {
		return false, false, errors.New("jobmgr admission: ordinary resize requires a granted record")
	}
	if totalBytes <= record.heldBytes {
		al.ordinaryBytes -= record.heldBytes - totalBytes
		record.heldBytes = totalBytes
		return true, al.hasGrantableWork(), nil
	}
	growth := totalBytes - record.heldBytes
	if growth <= al.ordinaryCapacity()-al.ordinaryBytes {
		record.heldBytes = totalBytes
		al.ordinaryBytes += growth
		return true, false, nil
	}
	al.nextTicket++
	if al.nextTicket == 0 {
		return false, false, errors.New("jobmgr admission: ticket sequence wrapped")
	}
	record.ticket = al.nextTicket
	record.bytes = growth
	record.state = admissionOrdinaryGrowing
	if err := al.insertOrdinary(ref.Slot); err != nil {
		record.bytes = 0
		record.state = admissionOrdinaryGranted
		return false, false, err
	}
	al.ordinaryWaiting++
	return false, false, nil
}

func (al *AdmissionLedger) ReleaseOrdinary(ref AdmissionRef) (bool, error) {
	al.mu.Lock()
	record, err := al.record(ref)
	if err != nil {
		al.mu.Unlock()
		return false, err
	}
	if record.state != admissionOrdinaryGranted || !record.ordinaryHeld {
		al.mu.Unlock()
		return false, errors.New("jobmgr admission: ordinary request is not granted")
	}
	if record.longLivedBytes != 0 {
		if err := al.validateRecordLane(record); err != nil {
			al.mu.Unlock()
			return false, err
		}
	}
	ordinaryBytes := record.heldBytes - record.longLivedBytes
	if ordinaryBytes <= 0 {
		al.mu.Unlock()
		return false, errors.New("jobmgr admission: ordinary facet has invalid bytes")
	}
	al.ordinaryBytes -= ordinaryBytes
	al.ordinaryGranted--
	al.freeRecord(ref.Slot)
	wake := al.hasGrantableWork()
	al.mu.Unlock()
	return wake, nil
}

func (al *AdmissionLedger) validateRecordLane(record *admissionRecord) error {
	if record == nil || !record.lane.Valid() {
		return errors.New("jobmgr admission: invalid record lane ownership")
	}
	use := al.lanes[record.lane.Slot]
	if use.count == 0 || use.generation != record.lane.Generation {
		return errors.New("jobmgr admission: invalid record lane ownership")
	}
	return nil
}

func (al *AdmissionLedger) transferLongLived(ref AdmissionRef, bytes int64) error {
	if al == nil || bytes <= 0 || bytes >= OrdinaryBudgetBytes {
		return errors.New("jobmgr admission: invalid long-lived transfer")
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.record(ref)
	if err != nil {
		return err
	}
	if record.state != admissionOrdinaryGranted || !record.ordinaryHeld || record.longLivedBytes != 0 || bytes >= record.heldBytes {
		return errors.New("jobmgr admission: long-lived transfer requires an untransferred granted record with an ordinary remainder")
	}
	record.longLivedBytes = bytes
	al.longLivedRecords++
	al.longLivedBytes += bytes
	return nil
}

func (al *AdmissionLedger) validateChargeFreeLongLived(ref AdmissionRef) error {
	if al == nil {
		return errors.New(
			"jobmgr admission: invalid charge-free long-lived authority",
		)
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	record, err := al.record(ref)
	if err != nil {
		return err
	}
	if record.state != admissionOrdinaryGranted ||
		!record.ordinaryHeld ||
		record.heldBytes <= 0 ||
		record.longLivedBytes != 0 {
		return errors.New(
			"jobmgr admission: charge-free long-lived authority requires an untransferred granted record",
		)
	}
	return nil
}

func (al *AdmissionLedger) releaseLongLived(ref AdmissionRef, bytes int64) (bool, error) {
	if al == nil || !ref.Valid() || bytes <= 0 {
		return false, errors.New("jobmgr admission: invalid long-lived release")
	}
	al.mu.Lock()
	record, recordErr := al.record(ref)
	if recordErr == nil {
		if record.state != admissionOrdinaryGranted ||
			record.longLivedBytes != bytes ||
			record.heldBytes < bytes {
			al.mu.Unlock()
			return false, errors.New("jobmgr admission: stale or mismatched long-lived release")
		}
		record.heldBytes -= bytes
		record.longLivedBytes = 0
	} else if al.longLivedRecords <= 0 ||
		al.longLivedBytes < bytes ||
		al.ordinaryBytes < bytes {
		al.mu.Unlock()
		return false, errors.New("jobmgr admission: stale or mismatched detached long-lived release")
	}
	al.ordinaryBytes -= bytes
	al.longLivedRecords--
	al.longLivedBytes -= bytes
	wake := al.hasGrantableWork()
	al.mu.Unlock()
	return wake, nil
}

func (al *AdmissionLedger) ReleaseCleanup(ref AdmissionRef, outcome FrameOutcome) (bool, error) {
	al.mu.Lock()
	record, err := al.record(ref)
	if err != nil {
		al.mu.Unlock()
		return false, err
	}
	if record.state != admissionCleanupGranted || al.cleanupGrant != ref.Slot {
		al.mu.Unlock()
		return false, errors.New("jobmgr admission: cleanup request is not the active grant")
	}
	switch outcome {
	case FrameOutcomeCommitted, FrameOutcomeSafeAbort:
		al.cleanupGrant = 0
		al.freeRecord(ref.Slot)
	case FrameOutcomePoisoned:
		record.state = admissionCleanupRetained
		al.cleanupGrant = 0
		al.cleanupRetained = true
	default:
		al.mu.Unlock()
		return false, errors.New("jobmgr admission: invalid cleanup frame outcome")
	}
	wake := al.hasGrantableWork()
	al.mu.Unlock()
	return wake, nil
}

func (al *AdmissionLedger) BeginCleanupOnly(runGeneration uint64) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase == admissionCleanupOnly && runGeneration != 0 && al.runGeneration == runGeneration {
		return nil
	}
	if al.phase != admissionOrdinaryOpen {
		return errors.New("jobmgr admission: cleanup-only transition is not available")
	}
	if runGeneration == 0 || (al.runGeneration != 0 && al.runGeneration != runGeneration) {
		return errors.New("jobmgr admission: stale run generation")
	}
	if al.ordinaryWaiting != 0 {
		return errors.New("jobmgr admission: cleanup-only transition with ordinary waiters")
	}
	al.bindRunGeneration(runGeneration)
	al.phase = admissionCleanupOnly
	return nil
}

func (al *AdmissionLedger) CloseDrained(runGeneration uint64) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase != admissionCleanupOnly || runGeneration == 0 || al.runGeneration != runGeneration {
		return errors.New("jobmgr admission: invalid drained close")
	}
	if !al.drained() {
		return errors.New("jobmgr admission: close with retained state")
	}
	al.phase = admissionClosed
	return nil
}

func (al *AdmissionLedger) ReopenDrained(completedGeneration, nextGeneration uint64) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.phase != admissionCleanupOnly || completedGeneration == 0 || al.runGeneration != completedGeneration || nextGeneration <= completedGeneration {
		return errors.New("jobmgr admission: invalid drained reopen")
	}
	if !al.runDrained(completedGeneration) {
		return errors.New("jobmgr admission: reopen with retained state")
	}
	if al.inputBodyCarried {
		if al.inputBodyNextGeneration == 0 || al.inputBodyNextGeneration != nextGeneration {
			return errors.New("jobmgr admission: carried input body targets another generation")
		}
		al.records[inputBodyRecordSlot].runGeneration = nextGeneration
	}
	al.phase = admissionOrdinaryOpen
	al.runGeneration = nextGeneration
	return nil
}

func (al *AdmissionLedger) drained() bool {
	return al.activeRecords == 0 && al.ordinaryWaiting == 0 && al.ordinaryGranted == 0 && al.ordinarySuspended == 0 && al.ordinaryBytes == 0 &&
		al.longLivedRecords == 0 && al.longLivedBytes == 0 &&
		al.cleanupWaiting == 0 && al.cleanupGrant == 0 && !al.cleanupRetained && al.cleanupHead == 0 && al.cleanupTail == 0 &&
		al.oldestInNode(1) == 0 && al.allOperationRecordsFree() &&
		al.allRadixNodesFree() &&
		al.records[inputBodyRecordSlot].state == admissionRecordFree && !al.inputBodyCarried && al.inputBodyNextGeneration == 0
}

func (al *AdmissionLedger) runDrained(runGeneration uint64) bool {
	if al.phase != admissionCleanupOnly || runGeneration == 0 || al.runGeneration != runGeneration || al.activeRecords != 0 || al.ordinaryWaiting != 0 || al.ordinaryGranted != 0 || al.ordinarySuspended != 0 ||
		al.longLivedRecords != 0 || al.longLivedBytes != 0 || al.cleanupWaiting != 0 || al.cleanupGrant != 0 || al.cleanupRetained ||
		al.cleanupHead != 0 || al.cleanupTail != 0 || al.oldestInNode(1) != 0 ||
		!al.allOperationRecordsFree() || !al.allRadixNodesFree() {
		return false
	}
	if !al.inputBodyCarried {
		return al.ordinaryBytes == 0 && al.records[inputBodyRecordSlot].state == admissionRecordFree && al.inputBodyNextGeneration == 0
	}
	record := al.records[inputBodyRecordSlot]
	return record.state == admissionInputBodyGranted && record.runGeneration == runGeneration && record.heldBytes > 0 && record.growthBytes == 0 &&
		al.ordinaryBytes == record.heldBytes
}

func (al *AdmissionLedger) Census() AdmissionCensus {
	al.mu.Lock()
	defer al.mu.Unlock()
	return AdmissionCensus{
		Phase: al.phase.String(), ProcessBytes: al.processBytes, ActiveRecords: al.activeRecords,
		OrdinaryWaiting: al.ordinaryWaiting, OrdinaryGranted: al.ordinaryGranted, OrdinarySuspended: al.ordinarySuspended, OrdinaryBytes: al.ordinaryBytes,
		LongLivedRecords: al.longLivedRecords, LongLivedBytes: al.longLivedBytes,
		CleanupWaiting: al.cleanupWaiting, CleanupGranted: al.cleanupGrant != 0, CleanupRetained: al.cleanupRetained,
		InputBodyActive:  al.records[inputBodyRecordSlot].state != admissionRecordFree,
		InputBodyWaiting: al.records[inputBodyRecordSlot].state == admissionInputBodyWaiting || al.records[inputBodyRecordSlot].state == admissionInputBodyGrowing,
		InputBodyCarried: al.inputBodyCarried,
		InputBodyBytes:   al.records[inputBodyRecordSlot].heldBytes,
		AllocatedRadix:   len(al.nodes) - 2 - al.freeNodes,
		FreeRecords:      al.freeRecords,
		FreeRadixNodes:   al.freeNodes,
		RunGeneration:    al.runGeneration,
	}
}

func (al *AdmissionLedger) allOperationRecordsFree() bool {
	return al.activeRecords == 0 &&
		al.freeRecords == len(al.records)-inputBodyRecordSlot-1
}

func (al *AdmissionLedger) allRadixNodesFree() bool {
	return al.freeNodes == len(al.nodes)-2
}

func (ap admissionPhase) String() string {
	switch ap {
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

func (al *AdmissionLedger) validateRequest(runGeneration uint64, lane AdmissionLaneRef, kind ReservationKind) error {
	if runGeneration == 0 || !lane.Valid() {
		return errors.New("jobmgr admission: invalid request identity")
	}
	if al.phase == admissionClosed || (kind == ReservationOrdinary && al.phase != admissionOrdinaryOpen) {
		return errors.New("jobmgr admission: request rejected by phase")
	}
	if al.runGeneration != 0 && al.runGeneration != runGeneration {
		return errors.New("jobmgr admission: stale run generation")
	}
	if use := al.lanes[lane.Slot]; use.count != 0 &&
		use.generation != lane.Generation {
		return errors.New("jobmgr admission: stale lane generation")
	}
	al.bindRunGeneration(runGeneration)
	return nil
}

func (al *AdmissionLedger) bindRunGeneration(runGeneration uint64) {
	if al.runGeneration == 0 {
		al.runGeneration = runGeneration
	}
}

func (al *AdmissionLedger) allocateRecord(runGeneration uint64, lane AdmissionLaneRef, bytes int64, state admissionRecordState) (AdmissionRef, error) {
	slot := al.freeRecordHead
	if slot == 0 {
		if uint64(len(al.records)) > uint64(^uint32(0)) {
			return AdmissionRef{}, errors.New(
				"jobmgr admission: record reference space exhausted",
			)
		}
		slot = uint32(len(al.records))
		al.records = append(al.records, admissionRecord{})
	} else {
		al.freeRecordHead = al.records[slot].next
		al.freeRecords--
	}
	record := &al.records[slot]
	generation := record.generation + 1
	if generation == 0 {
		*record = admissionRecord{
			generation: record.generation,
			next:       al.freeRecordHead,
		}
		al.freeRecordHead = slot
		al.freeRecords++
		return AdmissionRef{}, errors.New("jobmgr admission: record generation wrapped")
	}
	al.nextTicket++
	if al.nextTicket == 0 {
		*record = admissionRecord{
			generation: record.generation,
			next:       al.freeRecordHead,
		}
		al.freeRecordHead = slot
		al.freeRecords++
		return AdmissionRef{}, errors.New("jobmgr admission: ticket sequence wrapped")
	}
	*record = admissionRecord{
		generation: generation, state: state, runGeneration: runGeneration,
		lane: lane, bytes: bytes, ticket: al.nextTicket,
	}
	use := al.lanes[lane.Slot]
	if use.count == 0 {
		use.generation = lane.Generation
	}
	use.count++
	al.lanes[lane.Slot] = use
	al.activeRecords++
	return AdmissionRef{Slot: slot, Generation: generation}, nil
}

func (al *AdmissionLedger) freeRecord(slot uint32) {
	record := &al.records[slot]
	generation := record.generation
	lane := record.lane
	if lane.Valid() {
		use := al.lanes[lane.Slot]
		if use.count <= 0 || use.generation != lane.Generation {
			panic("jobmgr admission: invalid lane record release")
		}
		use.count--
		if use.count == 0 {
			delete(al.lanes, lane.Slot)
		} else {
			al.lanes[lane.Slot] = use
		}
	}
	*record = admissionRecord{generation: generation, next: al.freeRecordHead}
	al.freeRecordHead = slot
	al.freeRecords++
	al.activeRecords--
}

func (al *AdmissionLedger) record(ref AdmissionRef) (*admissionRecord, error) {
	if !ref.Valid() || uint64(ref.Slot) >= uint64(len(al.records)) {
		return nil, errors.New("jobmgr admission: invalid request reference")
	}
	record := &al.records[ref.Slot]
	if record.state == admissionRecordFree || record.generation != ref.Generation {
		return nil, errors.New("jobmgr admission: stale request reference")
	}
	return record, nil
}

func (al *AdmissionLedger) inputBodyRecord(token uint64) (*admissionRecord, error) {
	record := &al.records[inputBodyRecordSlot]
	if token == 0 || record.state == admissionRecordFree || uint64(record.generation) != token {
		return nil, errors.New("jobmgr admission: stale input body reference")
	}
	return record, nil
}

func (al *AdmissionLedger) resetInputBodyRecord() {
	record := &al.records[inputBodyRecordSlot]
	generation := record.generation
	*record = admissionRecord{generation: generation}
	al.inputBodyCarried = false
	al.inputBodyNextGeneration = 0
}

func (al *AdmissionLedger) appendCleanup(slot uint32) {
	record := &al.records[slot]
	record.previous = al.cleanupTail
	if al.cleanupTail == 0 {
		al.cleanupHead = slot
	} else {
		al.records[al.cleanupTail].next = slot
	}
	al.cleanupTail = slot
}

func (al *AdmissionLedger) removeCleanup(slot uint32) {
	record := &al.records[slot]
	if record.previous == 0 {
		al.cleanupHead = record.next
	} else {
		al.records[record.previous].next = record.next
	}
	if record.next == 0 {
		al.cleanupTail = record.previous
	} else {
		al.records[record.next].previous = record.previous
	}
	record.previous = 0
	record.next = 0
}

func (al *AdmissionLedger) insertOrdinary(slot uint32) error {
	record := &al.records[slot]
	nodeIndex := uint32(1)
	value := uint32(record.bytes)
	for depth := range admissionRadixBits {
		bit := uint8((value >> uint(admissionRadixBits-1-depth)) & 1)
		node := &al.nodes[nodeIndex]
		child := node.children[bit]
		if child == 0 {
			var err error
			child, err = al.allocateNode(nodeIndex, bit, uint8(depth+1))
			if err != nil {
				return err
			}
			al.nodes[nodeIndex].children[bit] = child
		}
		nodeIndex = child
	}
	leaf := &al.nodes[nodeIndex]
	record.leaf = nodeIndex
	record.previous = leaf.tail
	if leaf.tail == 0 {
		leaf.head = slot
	} else {
		al.records[leaf.tail].next = slot
	}
	leaf.tail = slot
	al.refreshAncestors(nodeIndex)
	return nil
}

func (al *AdmissionLedger) removeOrdinary(slot uint32) {
	record := &al.records[slot]
	leafIndex := record.leaf
	leaf := &al.nodes[leafIndex]
	headChanged := leaf.head == slot
	if record.previous == 0 {
		leaf.head = record.next
	} else {
		al.records[record.previous].next = record.next
	}
	if record.next == 0 {
		leaf.tail = record.previous
	} else {
		al.records[record.next].previous = record.previous
	}
	record.previous = 0
	record.next = 0
	record.leaf = 0
	if !headChanged {
		return
	}
	current := leafIndex
	for current != 1 && al.nodeEmpty(current) {
		node := al.nodes[current]
		parent := &al.nodes[node.parent]
		parent.children[node.parentBit] = 0
		parent.oldest[node.parentBit] = 0
		al.freeNode(current)
		current = node.parent
	}
	al.refreshAncestors(current)
}

func (al *AdmissionLedger) allocateNode(parent uint32, parentBit, depth uint8) (uint32, error) {
	index := al.freeNodeHead
	if index == 0 {
		if uint64(len(al.nodes)) > uint64(^uint32(0)) {
			return 0, errors.New(
				"jobmgr admission: radix reference space exhausted",
			)
		}
		index = uint32(len(al.nodes))
		al.nodes = append(al.nodes, admissionRadixNode{})
	} else {
		al.freeNodeHead = al.nodes[index].freeNext
		al.freeNodes--
	}
	al.nodes[index] = admissionRadixNode{parent: parent, parentBit: parentBit, depth: depth}
	return index, nil
}

func (al *AdmissionLedger) freeNode(index uint32) {
	al.nodes[index] = admissionRadixNode{freeNext: al.freeNodeHead}
	al.freeNodeHead = index
	al.freeNodes++
}

func (al *AdmissionLedger) nodeEmpty(index uint32) bool {
	node := al.nodes[index]
	return node.head == 0 && node.children[0] == 0 && node.children[1] == 0
}

func (al *AdmissionLedger) refreshAncestors(childIndex uint32) {
	for childIndex != 1 {
		child := &al.nodes[childIndex]
		parent := &al.nodes[child.parent]
		parent.oldest[child.parentBit] = al.oldestInNode(childIndex)
		childIndex = child.parent
	}
}

func (al *AdmissionLedger) oldestInNode(index uint32) uint32 {
	if index == 0 {
		return 0
	}
	node := &al.nodes[index]
	if node.depth == admissionRadixBits {
		return node.head
	}
	return al.older(node.oldest[0], node.oldest[1])
}

func (al *AdmissionLedger) older(first, second uint32) uint32 {
	if first == 0 {
		return second
	}
	if second == 0 || al.records[first].ticket < al.records[second].ticket {
		return first
	}
	return second
}

func (al *AdmissionLedger) oldestFitting(available int64) (slot uint32) {
	if available <= 0 || al.ordinaryWaiting == 0 {
		return 0
	}
	if available > OrdinaryBudgetBytes {
		available = OrdinaryBudgetBytes
	}
	value := uint32(available)
	nodeIndex := uint32(1)
	for depth := range admissionRadixBits {
		node := &al.nodes[nodeIndex]
		bit := uint8((value >> uint(admissionRadixBits-1-depth)) & 1)
		if bit == 1 {
			slot = al.older(slot, node.oldest[0])
		}
		child := node.children[bit]
		if child == 0 {
			return slot
		}
		nodeIndex = child
	}
	slot = al.older(slot, al.nodes[nodeIndex].head)
	return slot
}

func (al *AdmissionLedger) grant(slot uint32, kind ReservationKind) AdmissionGrant {
	record := al.records[slot]
	if kind == ReservationInputBodyGrowth {
		return AdmissionGrant{InputBodyToken: uint64(record.generation), Kind: kind, Bytes: record.heldBytes}
	}
	return AdmissionGrant{
		Ref:  AdmissionRef{Slot: slot, Generation: record.generation},
		Kind: kind, Bytes: record.heldBytes, Lane: record.lane,
	}
}

func (al *AdmissionLedger) hasGrantableWork() bool {
	if al.phase == admissionClosed {
		return false
	}
	if al.cleanupGrant == 0 && !al.cleanupRetained && al.cleanupHead != 0 {
		return true
	}
	if al.phase != admissionOrdinaryOpen || al.ordinaryWaiting == 0 {
		return false
	}
	slot := al.oldestFitting(al.ordinaryCapacity() - al.ordinaryBytes)
	return slot != 0
}
