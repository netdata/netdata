// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
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
	next   uint16
	loaded bool
	// checkOldest is set by a Get SEL Info change, after which the cache may
	// hold records the BMC overwrote, until dropOverwritten succeeds.
	checkOldest bool
	// info is the Get SEL Info response the records are current for.
	info      storage.GetSELInfoResponse
	infoKnown bool

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
	entries := int(info.Entries)
	updateErr := r.updateSEL(ctx, info)
	// The clock does not depend on the records: a failed record read must not
	// leave the UTC offset unknown.
	clockErr := r.refreshSELClock(ctx)
	if updateErr != nil || clockErr != nil {
		warnings.selRecords = true
		return &entries, c.snapshot(true)
	}
	return &entries, c.snapshot(false)
}

func (c *selCache) snapshot(failed bool) *SEL {
	return &SEL{
		Records:    c.records,
		Loaded:     c.loaded,
		Entries:    int(c.info.Entries),
		FreeBytes:  int(c.info.FreeBytes),
		Overflow:   c.info.OperationSupport.Overflow,
		UTCOffset:  c.utcOffset,
		ClockSkew:  c.clockSkew,
		ReadFailed: failed,
	}
}

// updateSEL brings the cache up to date with info. A changed erase time means
// the SEL was cleared. Once up to date, a changed entry count or addition time
// starts a read of the records after the newest cached one; if that record is
// gone or changed, the SEL was cleared or rewritten and is read again from the
// start. Reading is bounded by the read budget and resumes next collection.
// More cached records than the reported entry count are checked against the
// oldest record the BMC holds before any is dropped.
func (r *Reader) updateSEL(ctx context.Context, info storage.GetSELInfoResponse) error {
	c := &r.sel
	if c.infoKnown && !info.RecentEraseTime.Equal(c.info.RecentEraseTime) {
		c.reset()
	}
	changed := !c.infoKnown || info.Entries != c.info.Entries ||
		!info.RecentAdditionTime.Equal(c.info.RecentAdditionTime)
	c.checkOldest = c.checkOldest || changed

	switch {
	case info.Entries == 0:
		c.records, c.next, c.loaded = nil, endOfChainRecordID, true
	case c.next == endOfChainRecordID && changed:
		if err := r.resumeAfterNewest(ctx); err != nil {
			return err
		}
	}
	c.info, c.infoKnown = info, true

	if c.next != endOfChainRecordID {
		if err := r.walkSEL(ctx); err != nil {
			return err
		}
	}
	if c.checkOldest && len(c.records) > int(info.Entries) {
		if err := r.dropOverwritten(ctx); err != nil {
			return err
		}
	}
	c.checkOldest = false
	return nil
}

// dropOverwritten drops the cached records older than the oldest record the
// BMC holds: on rollover it overwrites its oldest records. The entry count
// alone cannot tell, because records logged after Get SEL Info were read too.
// A cache without the BMC's oldest record was rewritten and is read again.
func (r *Reader) dropOverwritten(ctx context.Context) error {
	c := &r.sel
	oldest, _, err := r.readSELRecord(ctx, firstRecordID)
	switch {
	case errors.Is(err, errSELRecordGone):
		c.reset()
		return nil
	case err != nil:
		return err
	}
	for i, rec := range c.records {
		if rec.Raw == oldest {
			c.records = c.records[i:]
			return nil
		}
	}
	c.reset()
	return nil
}

// resumeAfterNewest sets the next record to read after a change, or resets the
// cache when the newest cached record no longer exists unchanged.
func (r *Reader) resumeAfterNewest(ctx context.Context) error {
	c := &r.sel
	last := c.last()
	if last == nil {
		c.next = firstRecordID
		return nil
	}
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
	if c.next == endOfChainRecordID {
		c.loaded = true
	}
	c.append(added, reused)
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
// Like FreeIPMI, it reserves the SEL only when the BMC reports the reservation
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
		return [selRecordSize]byte(res.Data), res.NextRecordID, nil
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
