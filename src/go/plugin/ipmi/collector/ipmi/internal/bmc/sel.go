// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

const (
	// maxSELReadBudget bounds the Get SEL Entry reads of one collection; a
	// quarter of the update interval applies when shorter. A full read of a
	// 672-record SEL over local KCS took 5.4 s, so the first read of a large SEL
	// spans several collections instead of delaying sensor collection.
	maxSELReadBudget = 2 * time.Second
	// selClockMaxAge refreshes the SEL clock and UTC offset, which change only
	// when the BMC clock is set or adjusted, and spaces out failed attempts.
	selClockMaxAge = 5 * time.Minute
	// lastSELRecordID asks Get SEL Entry for the newest record.
	lastSELRecordID = 0xffff
	// readEntireSELRecord asks Get SEL Entry for the whole record.
	readEntireSELRecord = 0xff
	// maxUTCOffsetMinutes bounds a valid UTC offset (IPMI 2.0 section 31.11a).
	// Larger values, including 0x7FF for an unspecified offset, report none.
	maxUTCOffsetMinutes = 1440
)

var errSELRecordGone = errors.New("SEL record not present")

// selCache is the Reader's System Event Log state between collections.
//
// records only grow at the end, lose a prefix, or are reallocated, so the
// slices published in earlier snapshots are never written.
type selCache struct {
	records []SELRecord
	// next is the next record ID to read; endOfChainRecordID once reading
	// reached the newest record.
	next uint16
	// loaded is set once a read of the whole log was verified (completeRead)
	// and stays set until a reset.
	loaded bool
	// unverified is set when records were added to the cache since the last
	// verified read: the BMC may have overwritten records meanwhile, and a read
	// that follows a reused record ID can skip records.
	unverified bool
	// info is the Get SEL Info response the records are current for; latest
	// is the latest answer, which the snapshot reports.
	info      storage.GetSELInfoResponse
	latest    storage.GetSELInfoResponse
	infoKnown bool
	// decodedWith is the inventory generation the records were decoded with.
	decodedWith uint64

	// seen is nil until the first verified read. Afterwards it retains the
	// latest observed occupant of each 16-bit record ID, bounding identities
	// even when rollover prevents the mirror from completing another read.
	seen map[uint16][selRecordSize]byte
	// pending holds counts no delivered snapshot reported yet.
	pending SELCounts

	reservation uint16

	utcOffset   *time.Duration
	clockSkew   *time.Duration
	clockReadAt time.Time
}

// reset forgets every record; the next read starts from the first record.
func (c *selCache) reset() {
	c.records = nil
	c.next = firstRecordID
	c.loaded = false
}

// complete marks the cache verified to hold the whole log. The first verified
// cache seeds event accounting; earlier reads are history and need no tracking.
// Later completion drops IDs absent from the verified cache, preserving newer
// observations of retained IDs instead of overwriting them with cached content.
func (c *selCache) complete() {
	c.loaded = true
	c.unverified = false
	if c.seen == nil || len(c.seen) > len(c.records) {
		seen := make(map[uint16][selRecordSize]byte, len(c.records))
		for _, rec := range c.records {
			raw, ok := c.seen[rec.ID]
			if !ok {
				raw = rec.Raw
			}
			seen[rec.ID] = raw
		}
		c.seen = seen
	}
}

// observe accounts for a record as it is read, before any cache change can
// drop it: after the baseline, a different occupant at a record ID is new.
func (c *selCache) observe(raw [selRecordSize]byte) {
	if c.seen == nil {
		return
	}
	id := binary.LittleEndian.Uint16(raw[:])
	if previous, ok := c.seen[id]; ok && previous == raw {
		return
	}
	c.seen[id] = raw
	c.pending.observe(raw)
}

func (c *selCache) last() *SELRecord {
	if len(c.records) == 0 {
		return nil
	}
	return &c.records[len(c.records)-1]
}

// readSEL returns the SEL entry count and the SEL. Any SEL failure degrades
// only SEL data: the count is nil when Get SEL Info fails, and the returned
// SEL then keeps the records of earlier collections. The SEL is nil until the
// BMC first answers Get SEL Info.
func (r *Reader) readSEL(ctx context.Context, warnings *collectionWarnings) (*int, *SEL) {
	c := &r.sel
	var info storage.GetSELInfoResponse
	if err := r.exchange(ctx, &storage.GetSELInfoRequest{}, &info); err != nil {
		warnings.selUnavailable = true
		if !c.infoKnown {
			return nil, nil
		}
		return nil, c.snapshot(true)
	}
	c.latest = info
	r.redecodeSEL()
	updateErr := r.updateSEL(ctx, info)
	if updateErr != nil {
		warnings.selRecords = true
	}
	// The clock does not depend on the records: a failed record read must not
	// leave the UTC offset unknown, and a failed clock read keeps the earlier
	// values without making the records stale.
	if err := r.refreshSELClock(ctx); err != nil {
		warnings.selClock = true
	}
	s := c.snapshot(updateErr != nil)
	if s.NewKnown {
		s.New = c.pending
	}
	// Completing a read asks Get SEL Info again; the count and the usage are
	// from the same, latest answer.
	entries := int(c.latest.Entries)
	return &entries, s
}

func (c *selCache) snapshot(failed bool) *SEL {
	s := &SEL{
		Records:    c.records,
		Loaded:     c.loaded,
		Entries:    int(c.latest.Entries),
		FreeBytes:  int(c.latest.FreeBytes),
		Overflow:   c.latest.OperationSupport.Overflow,
		UTCOffset:  c.utcOffset,
		ClockSkew:  c.clockSkew,
		ReadFailed: failed,
	}
	s.NewKnown = c.seen != nil && !failed
	return s
}

// delivered forgets the counts of a snapshot the Reader returned.
func (c *selCache) delivered(s *SEL) {
	if s != nil && s.NewKnown {
		c.pending = SELCounts{}
	}
}

// redecodeSEL resolves the cached records against a replaced inventory, so a
// renamed or retyped sensor renames its records. The published slice is
// replaced only when a record changed.
func (r *Reader) redecodeSEL() {
	c := &r.sel
	if c.decodedWith == r.inventoryGeneration {
		return
	}
	c.decodedWith = r.inventoryGeneration
	sensors := sensorsByAddress(r.inventory)
	var records []SELRecord
	for i, rec := range c.records {
		decoded := decodeSELRecord(rec.Raw, sensors)
		if records == nil {
			if decoded == rec {
				continue
			}
			records = make([]SELRecord, len(c.records))
			copy(records, c.records[:i])
		}
		records[i] = decoded
	}
	if records != nil {
		c.records = records
	}
}

// updateSEL brings the cache up to date with info. A changed erase time means
// the SEL was cleared. A changed entry count or addition time makes reading
// resume after the newest cached record (resumeAfterChange). Reading is bounded
// by the read budget and continues next collection. A read that reaches the
// newest record is complete only once verified (completeRead).
func (r *Reader) updateSEL(ctx context.Context, info storage.GetSELInfoResponse) error {
	c := &r.sel
	if c.infoKnown && !info.RecentEraseTime.Equal(c.info.RecentEraseTime) {
		c.reset()
	}
	changed := !c.infoKnown || info.Entries != c.info.Entries ||
		!info.RecentAdditionTime.Equal(c.info.RecentAdditionTime)

	switch {
	case info.Entries == 0:
		c.records, c.next = nil, endOfChainRecordID
		c.complete()
	case changed:
		if err := r.resumeAfterChange(ctx); err != nil {
			return err
		}
	}
	c.info, c.infoKnown = info, true

	if c.next != endOfChainRecordID {
		if err := r.walkSEL(ctx); err != nil {
			return err
		}
	}
	switch {
	case c.next == endOfChainRecordID && c.unverified:
		return r.completeRead(ctx)
	case c.next != endOfChainRecordID && len(c.records) > int(info.Entries):
		// A read still in progress drops the records the BMC overwrote, so a
		// log changing faster than it is read cannot grow the cache past the
		// log's size.
		_, err := r.dropOverwritten(ctx)
		return err
	}
	return nil
}

// completeRead verifies a read that reached the newest record. Neither the end
// of the chain nor the entry count proves the cache holds the log, because a
// read that follows a record ID the BMC gave to a newer record skips the
// records in between. The BMC frees an ID in two ways, which the verification
// covers: rollover overwrites its oldest records, so its oldest record must be
// cached (dropOverwritten), and a deletion changes the erase time, which must
// not have changed since this collection's Get SEL Info. A BMC that deletes
// records without changing the erase time is not supported.
func (r *Reader) completeRead(ctx context.Context) error {
	c := &r.sel
	if found, err := r.dropOverwritten(ctx); !found || err != nil {
		return err
	}
	var info storage.GetSELInfoResponse
	if err := r.exchange(ctx, &storage.GetSELInfoRequest{}, &info); err != nil {
		return err
	}
	c.latest = info
	if !info.RecentEraseTime.Equal(c.info.RecentEraseTime) {
		c.reset()
		return nil
	}
	c.complete()
	return nil
}

// dropOverwritten drops the cached records older than the oldest record the
// BMC holds, which overwrote them on rollover, and reports whether the cache
// holds that record. A cache without it is read again.
func (r *Reader) dropOverwritten(ctx context.Context) (bool, error) {
	c := &r.sel
	oldest, _, err := r.readSELRecord(ctx, firstRecordID)
	switch {
	case errors.Is(err, errSELRecordGone):
		c.reset()
		return false, nil
	case err != nil:
		return false, err
	}
	for i, rec := range c.records {
		if rec.Raw == oldest {
			for _, gone := range c.records[:i] {
				if c.seen[gone.ID] == gone.Raw {
					delete(c.seen, gone.ID)
				}
			}
			c.records = c.records[i:]
			return true, nil
		}
	}
	c.reset()
	return false, nil
}

// resumeAfterChange sets where reading continues after the log changed: after
// the newest cached record, which must still be held unchanged. On a full log
// the BMC can give a new record the ID of an overwritten one, so a next-record
// ID kept by an unfinished read could lead past the records logged since; the
// unchanged newest cached record proves which records follow. A read that had
// reached the newest record first compares it with the BMC's newest record and
// has nothing to read when they match. A newest cached record that is gone or
// changed means the log was cleared, rewritten or overwritten past it, and the
// log is read again from the start.
func (r *Reader) resumeAfterChange(ctx context.Context) error {
	c := &r.sel
	last := c.last()
	if last == nil {
		c.next = firstRecordID
		return nil
	}
	if c.next == endOfChainRecordID {
		newest, _, err := r.readSELRecord(ctx, lastSELRecordID)
		switch {
		case errors.Is(err, errSELRecordGone):
			// Emptied after Get SEL Info.
			c.reset()
			return nil
		case err != nil:
			return err
		case newest == last.Raw:
			return nil
		}
	}
	cached, next, err := r.readSELRecord(ctx, last.ID)
	switch {
	case errors.Is(err, errSELRecordGone) || (err == nil && cached != last.Raw):
		c.reset()
	case err != nil:
		return err
	default:
		c.next = next
	}
	return nil
}

// walkSEL reads records from c.next until the newest one or until the read
// budget is spent. A record ID already cached with other content was reused
// by the BMC and replaces the cached record; reaching a cached record with the
// same content means the chain loops back, which ends the read. A record that
// disappears while reading fails the read and restarts it from the first
// record next collection.
func (r *Reader) walkSEL(ctx context.Context) error {
	c := &r.sel
	cached := make(map[uint16][selRecordSize]byte, len(c.records))
	for _, rec := range c.records {
		cached[rec.ID] = rec.Raw
	}
	sensors := sensorsByAddress(r.inventory)
	deadline := r.now().Add(r.cfg.selReadBudget())

	var added []SELRecord
	reused := make(map[uint16]bool)
	var err error
	for c.next != endOfChainRecordID {
		var raw [selRecordSize]byte
		var next uint16
		raw, next, err = r.readSELRecord(ctx, c.next)
		if errors.Is(err, errSELRecordGone) {
			c.reset()
			return err
		}
		if err != nil {
			break
		}
		rec := decodeSELRecord(raw, sensors)
		if prev, ok := cached[rec.ID]; ok {
			if prev == raw {
				c.next = endOfChainRecordID
				break
			}
			reused[rec.ID] = true
		}
		cached[rec.ID] = raw
		added = append(added, rec)
		c.next = next
		if !r.now().Before(deadline) {
			break
		}
	}
	if len(added) > 0 {
		c.append(added, reused)
		c.unverified = true
	}
	return err
}

// append adds records read in log order. Reused IDs drop the cached record
// and any earlier record read in the same pass under that ID.
func (c *selCache) append(added []SELRecord, reused map[uint16]bool) {
	if len(reused) == 0 {
		c.records = append(c.records, added...)
		return
	}
	lastIndex := make(map[uint16]int, len(added))
	for i, rec := range added {
		lastIndex[rec.ID] = i
	}
	records := make([]SELRecord, 0, len(c.records)+len(added))
	for _, rec := range c.records {
		if !reused[rec.ID] {
			records = append(records, rec)
		}
	}
	for i, rec := range added {
		if lastIndex[rec.ID] == i {
			records = append(records, rec)
		}
	}
	c.records = records
}

// readSELRecord reads one whole record and returns it with the next record ID.
// Every record read is accounted here, whatever the caller does with it. Like
// FreeIPMI, it reserves the SEL only when the BMC reports the reservation
// cancelled or invalid, and retries a bounded number of times.
func (r *Reader) readSELRecord(ctx context.Context, id uint16) ([selRecordSize]byte, uint16, error) {
	c := &r.sel
	for retries := 0; ; retries++ {
		req := &storage.GetSELEntryRequest{
			ReservationID: c.reservation,
			RecordID:      id,
			ReadBytes:     readEntireSELRecord,
		}
		var res storage.GetSELEntryResponse
		err := r.exchange(ctx, req, &res)
		if cc, ok := types.IsResponseError(err); ok {
			switch {
			case cc.CompletionCode() == types.CodeReservationCanceled && retries < maxReservationRetries:
				var reservation storage.ReserveSELResponse
				if err := r.exchange(ctx, &storage.ReserveSELRequest{}, &reservation); err != nil {
					return [selRecordSize]byte{}, 0, err
				}
				c.reservation = reservation.ReservationID
				continue
			case cc.CompletionCode() == types.CodeRequestedDataNotPresent:
				return [selRecordSize]byte{}, 0, errSELRecordGone
			}
		}
		if err != nil {
			return [selRecordSize]byte{}, 0, err
		}
		if len(res.Data) != selRecordSize {
			return [selRecordSize]byte{}, 0, fmt.Errorf("SEL record %#04x has %d bytes", id, len(res.Data))
		}
		raw := [selRecordSize]byte(res.Data)
		c.observe(raw)
		return raw, res.NextRecordID, nil
	}
}

// refreshSELClock reads the SEL UTC offset and measures the BMC SEL clock
// against the Agent clock when the last attempt is older than selClockMaxAge.
// A failed attempt also waits for the next refresh, so a BMC that does not
// answer cannot delay every collection by the command timeout; earlier values
// stay.
func (r *Reader) refreshSELClock(ctx context.Context) error {
	c := &r.sel
	start := r.now()
	if !c.clockReadAt.IsZero() && start.Sub(c.clockReadAt) < selClockMaxAge {
		return nil
	}
	err := r.readSELClock(ctx)
	if ctx.Err() == nil {
		c.clockReadAt = start
	}
	return err
}

// readSELClock reads the clock values. A BMC that rejects Get SEL Time UTC
// Offset, an IPMI 2.0 addition, reports no offset.
func (r *Reader) readSELClock(ctx context.Context) error {
	var offset *time.Duration
	var res storage.GetSELTimeUTCOffsetResponse
	err := r.exchange(ctx, &storage.GetSELTimeUTCOffsetRequest{}, &res)
	switch {
	case isCompletionCodeError(err):
	case err != nil:
		return err
	case res.MinutesOffset >= -maxUTCOffsetMinutes && res.MinutesOffset <= maxUTCOffsetMinutes:
		offset = new(time.Duration(res.MinutesOffset) * time.Minute)
	}

	var clock storage.GetSELTimeResponse
	if err := r.exchange(ctx, &storage.GetSELTimeRequest{}, &clock); err != nil {
		return err
	}
	c := &r.sel
	c.utcOffset = offset
	c.clockSkew = new(clock.Time.Sub(r.now()).Round(time.Second))
	return nil
}

// selReadBudget is the time one collection may spend reading SEL records.
func (c Config) selReadBudget() time.Duration {
	if c.UpdateEvery <= 0 {
		return maxSELReadBudget
	}
	return min(maxSELReadBudget, c.UpdateEvery/4)
}
