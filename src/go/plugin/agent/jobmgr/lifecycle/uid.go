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
	state uidState // record state (active / tombstone / free)
	key   string   // the UID

	expires time.Time  // tombstone expiry time
	prev    *uidRecord // previous record in the tombstone list
	next    *uidRecord // next record in the tombstone list

	freeNext *uidRecord // freelist link
}

type UIDLedger struct {
	mu     sync.Mutex // guards all fields
	closed bool       // ledger is closed (shutdown)

	free  *uidRecord            // head of the record freelist
	index map[string]*uidRecord // active + tombstoned records by UID

	tombstoneHead *uidRecord // head of the timed tombstone list
	tombstoneTail *uidRecord // tail of the timed tombstone list
	active        int        // count of active UIDs
	tombstones    int        // count of tombstoned UIDs
}

func NewUIDLedger() *UIDLedger {
	return &UIDLedger{
		index: make(map[string]*uidRecord),
	}
}

func (ul *UIDLedger) Admit(uid string, now time.Time) error {
	if uid == "" {
		return errors.New("jobmgr UID ledger: empty UID")
	}
	if now.IsZero() {
		return errors.New("jobmgr UID ledger: zero admission time")
	}

	ul.mu.Lock()
	defer ul.mu.Unlock()
	if ul.closed {
		return errors.New("jobmgr UID ledger: closed")
	}

	ul.expireForAdmission(uid, now)
	if record := ul.index[uid]; record != nil {
		if record.state == uidStateActive {
			return errors.New("jobmgr UID ledger: duplicate active UID")
		}
		return errors.New("jobmgr UID ledger: tombstoned UID")
	}
	record := ul.free
	if record == nil {
		record = &uidRecord{}
	} else {
		ul.free = record.freeNext
	}
	*record = uidRecord{
		state: uidStateActive,
		key:   strings.Clone(uid),
	}
	ul.index[record.key] = record
	ul.active++
	return nil
}

func (ul *UIDLedger) Complete(uid string, tombstone bool, now time.Time) error {
	if now.IsZero() {
		return errors.New("jobmgr UID ledger: zero completion time")
	}

	ul.mu.Lock()
	defer ul.mu.Unlock()
	record := ul.index[uid]
	if record == nil || record.state != uidStateActive {
		return errors.New("jobmgr UID ledger: completion for inactive UID")
	}

	ul.active--
	if !tombstone {
		ul.removeRecord(record)
		return nil
	}

	record.state = uidStateTombstone
	record.expires = now.Add(UIDTombstoneLifetime)
	record.prev = ul.tombstoneTail
	record.next = nil
	if ul.tombstoneTail == nil {
		ul.tombstoneHead = record
	} else {
		ul.tombstoneTail.next = record
	}
	ul.tombstoneTail = record
	ul.tombstones++
	return nil
}

func (ul *UIDLedger) CloseBatch(max int) (more bool, err error) {
	if max <= 0 || max > UIDReturnBatch {
		return false, errors.New("jobmgr UID ledger: invalid close batch")
	}

	ul.mu.Lock()
	defer ul.mu.Unlock()
	ul.closed = true
	if ul.active != 0 {
		return true, errors.New("jobmgr UID ledger: close with active UIDs")
	}
	for count := 0; count < max && ul.tombstoneHead != nil; count++ {
		ul.removeRecord(ul.tombstoneHead)
	}
	return ul.tombstoneHead != nil, nil
}

func (ul *UIDLedger) Census() (active, tombstones int, closed bool) {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	return ul.active, ul.tombstones, ul.closed
}

func (ul *UIDLedger) expireForAdmission(uid string, now time.Time) {
	remaining := UIDReturnBatch
	if record := ul.index[uid]; record != nil {
		if record.state == uidStateTombstone && !record.expires.After(now) {
			ul.removeRecord(record)
			remaining--
		}
	}
	for remaining > 0 && ul.tombstoneHead != nil {
		record := ul.tombstoneHead
		if record.expires.After(now) {
			break
		}
		ul.removeRecord(ul.tombstoneHead)
		remaining--
	}
}

func (ul *UIDLedger) removeRecord(record *uidRecord) {
	if record == nil {
		return
	}
	if record.state == uidStateTombstone {
		if record.prev == nil {
			ul.tombstoneHead = record.next
		} else {
			record.prev.next = record.next
		}
		if record.next == nil {
			ul.tombstoneTail = record.prev
		} else {
			record.next.prev = record.prev
		}
		ul.tombstones--
	}
	delete(ul.index, record.key)
	*record = uidRecord{
		state:    uidStateFree,
		freeNext: ul.free,
	}
	ul.free = record
}
