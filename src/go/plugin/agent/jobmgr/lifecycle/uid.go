// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	MaximumUIDRecords    = 16_384
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
	prev    uint16
	next    uint16

	freeNext uint16
}

type UIDLedger struct {
	mu     sync.Mutex
	closed bool

	records [MaximumUIDRecords + 1]uidRecord
	free    uint16
	index   map[string]uint16

	tombstoneHead uint16
	tombstoneTail uint16
	active        int
	tombstones    int
}

func NewUIDLedger() *UIDLedger {
	ledger := &UIDLedger{
		free:  1,
		index: make(map[string]uint16, MaximumUIDRecords),
	}
	for slot := uint16(1); slot <= MaximumUIDRecords; slot++ {
		ledger.records[slot].freeNext = slot + 1
	}
	ledger.records[MaximumUIDRecords].freeNext = 0
	return ledger
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
	if slot := ledger.index[uid]; slot != 0 {
		if ledger.records[slot].state == uidStateActive {
			return errors.New("jobmgr UID ledger: duplicate active UID")
		}
		return errors.New("jobmgr UID ledger: tombstoned UID")
	}
	if ledger.free == 0 {
		return errors.New("jobmgr UID ledger: capacity exhausted")
	}

	slot := ledger.free
	ledger.free = ledger.records[slot].freeNext
	ledger.records[slot] = uidRecord{
		state: uidStateActive,
		key:   strings.Clone(uid),
	}
	ledger.index[ledger.records[slot].key] = slot
	ledger.active++
	return nil
}

func (ledger *UIDLedger) Complete(uid string, tombstone bool, now time.Time) error {
	if now.IsZero() {
		return errors.New("jobmgr UID ledger: zero completion time")
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	slot := ledger.index[uid]
	if slot == 0 || ledger.records[slot].state != uidStateActive {
		return errors.New("jobmgr UID ledger: completion for inactive UID")
	}

	ledger.active--
	if !tombstone {
		ledger.removeRecord(slot)
		return nil
	}

	record := &ledger.records[slot]
	record.state = uidStateTombstone
	record.expires = now.Add(UIDTombstoneLifetime)
	record.prev = ledger.tombstoneTail
	record.next = 0
	if ledger.tombstoneTail == 0 {
		ledger.tombstoneHead = slot
	} else {
		ledger.records[ledger.tombstoneTail].next = slot
	}
	ledger.tombstoneTail = slot
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
	for count := 0; count < max && ledger.tombstoneHead != 0; count++ {
		ledger.removeRecord(ledger.tombstoneHead)
	}
	return ledger.tombstoneHead != 0, nil
}

func (ledger *UIDLedger) Census() (active, tombstones int, closed bool) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.active, ledger.tombstones, ledger.closed
}

func (ledger *UIDLedger) expireForAdmission(uid string, now time.Time) {
	remaining := UIDReturnBatch
	if slot := ledger.index[uid]; slot != 0 {
		record := &ledger.records[slot]
		if record.state == uidStateTombstone && !record.expires.After(now) {
			ledger.removeRecord(slot)
			remaining--
		}
	}
	for remaining > 0 && ledger.tombstoneHead != 0 {
		record := &ledger.records[ledger.tombstoneHead]
		if record.expires.After(now) {
			break
		}
		ledger.removeRecord(ledger.tombstoneHead)
		remaining--
	}
}

func (ledger *UIDLedger) removeRecord(slot uint16) {
	record := &ledger.records[slot]
	if record.state == uidStateTombstone {
		if record.prev == 0 {
			ledger.tombstoneHead = record.next
		} else {
			ledger.records[record.prev].next = record.next
		}
		if record.next == 0 {
			ledger.tombstoneTail = record.prev
		} else {
			ledger.records[record.next].prev = record.prev
		}
		ledger.tombstones--
	}
	delete(ledger.index, record.key)
	ledger.records[slot] = uidRecord{state: uidStateFree, freeNext: ledger.free}
	ledger.free = slot
}
