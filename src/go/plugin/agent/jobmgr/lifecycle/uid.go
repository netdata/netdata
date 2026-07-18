// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	UIDReturnBatch       = 64
	UIDTombstoneLifetime = 60 * time.Second
)

type uidState uint8

const (
	uidStateFree uidState = iota
	uidStateActive
	uidStateTombstone
)

type uidRecord struct {
	state uidState
	key   string

	expires time.Time
	prev    *uidRecord
	next    *uidRecord

	freeNext *uidRecord
}

type UIDLedger struct {
	mu     sync.Mutex
	closed bool

	free  *uidRecord
	index map[string]*uidRecord

	tombstoneHead *uidRecord
	tombstoneTail *uidRecord
	active        int
	tombstones    int
}

func NewUIDLedger() *UIDLedger {
	return &UIDLedger{index: make(map[string]*uidRecord)}
}

func (ledger *UIDLedger) Admit(uid string, now time.Time) error {
	if uid == "" {
		return errors.New("jobmgr UID ledger: empty UID")
	}
	if now.IsZero() {
		return errors.New("jobmgr UID ledger: zero admission time")
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.closed {
		return errors.New("jobmgr UID ledger: closed")
	}

	ledger.expireForAdmission(uid, now)
	if record := ledger.index[uid]; record != nil {
		if record.state == uidStateActive {
			return errors.New("jobmgr UID ledger: duplicate active UID")
		}
		return errors.New("jobmgr UID ledger: tombstoned UID")
	}
	record := ledger.free
	if record == nil {
		record = &uidRecord{}
	} else {
		ledger.free = record.freeNext
	}
	*record = uidRecord{
		state: uidStateActive,
		key:   strings.Clone(uid),
	}
	ledger.index[record.key] = record
	ledger.active++
	return nil
}

func (ledger *UIDLedger) Complete(uid string, tombstone bool, now time.Time) error {
	if now.IsZero() {
		return errors.New("jobmgr UID ledger: zero completion time")
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	record := ledger.index[uid]
	if record == nil || record.state != uidStateActive {
		return errors.New("jobmgr UID ledger: completion for inactive UID")
	}

	ledger.active--
	if !tombstone {
		ledger.removeRecord(record)
		return nil
	}

	record.state = uidStateTombstone
	record.expires = now.Add(UIDTombstoneLifetime)
	record.prev = ledger.tombstoneTail
	record.next = nil
	if ledger.tombstoneTail == nil {
		ledger.tombstoneHead = record
	} else {
		ledger.tombstoneTail.next = record
	}
	ledger.tombstoneTail = record
	ledger.tombstones++
	return nil
}

func (ledger *UIDLedger) CloseBatch(max int) (more bool, err error) {
	if max <= 0 || max > UIDReturnBatch {
		return false, errors.New("jobmgr UID ledger: invalid close batch")
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.closed = true
	if ledger.active != 0 {
		return true, errors.New("jobmgr UID ledger: close with active UIDs")
	}
	for count := 0; count < max && ledger.tombstoneHead != nil; count++ {
		ledger.removeRecord(ledger.tombstoneHead)
	}
	return ledger.tombstoneHead != nil, nil
}

func (ledger *UIDLedger) Census() (active, tombstones int, closed bool) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.active, ledger.tombstones, ledger.closed
}

func (ledger *UIDLedger) expireForAdmission(uid string, now time.Time) {
	remaining := UIDReturnBatch
	if record := ledger.index[uid]; record != nil {
		if record.state == uidStateTombstone && !record.expires.After(now) {
			ledger.removeRecord(record)
			remaining--
		}
	}
	for remaining > 0 && ledger.tombstoneHead != nil {
		record := ledger.tombstoneHead
		if record.expires.After(now) {
			break
		}
		ledger.removeRecord(ledger.tombstoneHead)
		remaining--
	}
}

func (ledger *UIDLedger) removeRecord(record *uidRecord) {
	if record == nil {
		return
	}
	if record.state == uidStateTombstone {
		if record.prev == nil {
			ledger.tombstoneHead = record.next
		} else {
			record.prev.next = record.next
		}
		if record.next == nil {
			ledger.tombstoneTail = record.prev
		} else {
			record.next.prev = record.prev
		}
		ledger.tombstones--
	}
	delete(ledger.index, record.key)
	*record = uidRecord{state: uidStateFree, freeNext: ledger.free}
	ledger.free = record
}
