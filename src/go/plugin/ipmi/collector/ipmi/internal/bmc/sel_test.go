// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	selRecordsWarning = "SEL records unavailable"
	selClockWarning   = "SEL clock unavailable"
)

// emptySEL is the SEL of newFakeBMC: no records, no new records, no UTC
// offset, and a clock in step with the Agent.
func emptySEL() *SEL {
	return &SEL{
		Loaded:    true,
		ClockSkew: new(time.Duration(0)),
		NewKnown:  true,
	}
}

// tempEvent is an Upper Non-critical going high assertion of newFakeBMC's CPU
// Temp sensor; its timestamp tells records with the same ID apart.
func tempEvent(id uint16, timestamp uint32) [selRecordSize]byte {
	return selEvent(id, timestamp, 0x0020, 0x01, 10, 0x01, 0x07, 0, 0)
}

func tempEvents(ids ...uint16) [][selRecordSize]byte {
	var out [][selRecordSize]byte
	for _, id := range ids {
		out = append(out, tempEvent(id, 0x60000000+uint32(id)))
	}
	return out
}

// newSELTestReader returns a Reader whose clock advances by entryCost with
// every Get SEL Entry the fake answers.
func newSELTestReader(fake *fakeBMC, updateEvery, entryCost time.Duration) *Reader {
	r := newTestReader(fake)
	r.cfg.UpdateEvery = updateEvery
	now := testNow
	r.now = func() time.Time { return now }
	fake.sel.onEntry = func() { now = now.Add(entryCost) }
	return r
}

// collectSEL runs one collection that must succeed without warnings.
func collectSEL(t *testing.T, r *Reader) *SEL {
	t.Helper()
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Empty(t, got.Warnings)
	require.NotNil(t, got.SEL)
	return got.SEL
}

func recordsRaw(s *SEL) [][selRecordSize]byte {
	var raw [][selRecordSize]byte
	for _, rec := range s.Records {
		raw = append(raw, rec.Raw)
	}
	return raw
}

func recordIDs(s *SEL) []uint16 {
	var ids []uint16
	for _, rec := range s.Records {
		ids = append(ids, rec.ID)
	}
	return ids
}

// entryReads returns the record IDs requested by Get SEL Entry since the last call.
func entryReads(fake *fakeBMC) []uint16 {
	var ids []uint16
	for _, q := range fake.selEntryRequests() {
		ids = append(ids, q.RecordID)
	}
	fake.requests = nil
	return ids
}

func TestReader_SEL_InitialLoadIsBounded(t *testing.T) {
	tests := map[string]struct {
		updateEvery time.Duration
		perCycle    []int // records read per collection until loaded
	}{
		"quarter of a short interval": {
			updateEvery: 4 * time.Second,
			perCycle:    []int{4, 4, 2},
		},
		"two second budget": {
			updateEvery: 20 * time.Second,
			perCycle:    []int{7, 3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
			r := newSELTestReader(fake, tc.updateEvery, 300*time.Millisecond)

			var snapshots []*SEL
			var copies [][]SELRecord
			read := 0
			for i, n := range tc.perCycle {
				sel := collectSEL(t, r)
				read += n
				wantReads := n
				if i == len(tc.perCycle)-1 {
					wantReads++ // the oldest record, which verifies the read
				}
				assert.Len(t, entryReads(fake), wantReads, "collection %d", i)
				assert.Equal(t, []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}[:read], recordIDs(sel))
				assert.Equal(t, i == len(tc.perCycle)-1, sel.Loaded)
				assert.Equal(t, sel.Loaded, sel.NewKnown, "no new-record count before the first complete read")
				assert.Empty(t, sel.New, "records of the first read are history")
				assert.Equal(t, 10, sel.Entries)
				snapshots = append(snapshots, sel)
				copies = append(copies, slices.Clone(sel.Records))
			}

			assert.Empty(t, entryReads(fake), "a loaded, unchanged SEL is not read")
			collectSEL(t, r)
			assert.Empty(t, entryReads(fake))
			for i, sel := range snapshots {
				assert.Equal(t, copies[i], sel.Records, "published records never change")
			}
			assert.Zero(t, fake.sel.reserves, "no reservation without 0xC5")
		})
	}
}

func TestReader_SEL_Changes(t *testing.T) {
	changedThree := tempEvent(3, 0x70000000)
	reusedOne := tempEvent(1, 0x70000001)
	reusedTwo := tempEvent(2, 0x70000002)
	tests := map[string]struct {
		change    func(*fakeSEL)
		wantReads []uint16
		wantIDs   []uint16
		// wantRaw, when set, is the expected content of the cached records.
		wantRaw [][selRecordSize]byte
		// wantNew is the content of the records the change logged.
		wantNew [][selRecordSize]byte
	}{
		"unchanged": {
			change:  func(*fakeSEL) {},
			wantIDs: []uint16{1, 2, 3},
		},
		"appended": {
			change:    func(s *fakeSEL) { s.add(tempEvents(4, 5)...) },
			wantReads: []uint16{0xffff, 3, 4, 5, 0x0000},
			wantIDs:   []uint16{1, 2, 3, 4, 5},
			wantNew:   tempEvents(4, 5),
		},
		"cleared": {
			change: func(s *fakeSEL) {
				s.clear()
				s.add(reusedOne)
			},
			wantReads: []uint16{0x0000, 0x0000},
			wantRaw:   [][selRecordSize]byte{reusedOne},
			wantNew:   [][selRecordSize]byte{reusedOne},
		},
		"emptied": {
			change: func(s *fakeSEL) { s.clear() },
		},
		"one record deleted": {
			// Delete SEL Entry also changes the erase time.
			change: func(s *fakeSEL) {
				s.records = append(s.records[:1], s.records[2])
				s.eraseTime++
			},
			wantReads: []uint16{0x0000, 3, 0x0000},
			wantIDs:   []uint16{1, 3},
		},
		"rewritten without an erase time change": {
			change: func(s *fakeSEL) {
				s.records = nil
				s.add(tempEvents(7, 8)...)
			},
			wantReads: []uint16{0xffff, 3, 0x0000, 8, 0x0000},
			wantIDs:   []uint16{7, 8},
			wantNew:   tempEvents(7, 8),
		},
		"newest record changed in place": {
			change: func(s *fakeSEL) {
				s.records = s.records[:2]
				s.add(changedThree)
			},
			wantReads: []uint16{0xffff, 3, 0x0000, 2, 3, 0x0000},
			wantRaw:   append(tempEvents(1, 2), changedThree),
			wantNew:   [][selRecordSize]byte{changedThree},
		},
		"rollover drops the overwritten records": {
			change: func(s *fakeSEL) {
				s.records = s.records[2:]
				s.add(tempEvents(4, 5)...)
			},
			wantReads: []uint16{0xffff, 3, 4, 5, 0x0000},
			wantIDs:   []uint16{3, 4, 5},
			wantNew:   tempEvents(4, 5),
		},
		"rollover reuses a record ID": {
			change: func(s *fakeSEL) {
				s.records = s.records[1:]
				s.add(reusedOne)
			},
			wantReads: []uint16{0xffff, 3, 1, 0x0000},
			wantRaw:   append(tempEvents(2, 3), reusedOne),
			wantNew:   [][selRecordSize]byte{reusedOne},
		},
		"deleted record ID reused for a new event": {
			change: func(s *fakeSEL) {
				s.records = append(s.records[:1], s.records[2])
				s.add(reusedTwo)
			},
			wantReads: []uint16{0xffff, 3, 2, 0x0000},
			wantRaw:   append(tempEvents(1, 3), reusedTwo),
			wantNew:   [][selRecordSize]byte{reusedTwo},
		},
		"chain loops back to a cached record": {
			change: func(s *fakeSEL) {
				s.add(tempEvents(4)...)
				s.next = map[uint16]uint16{4: 2}
			},
			wantReads: []uint16{0xffff, 3, 4, 2, 0x0000},
			wantIDs:   []uint16{1, 2, 3, 4},
			wantNew:   tempEvents(4),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3)
			r := newSELTestReader(fake, 0, 0)
			before := collectSEL(t, r)
			require.True(t, before.Loaded)
			require.True(t, before.NewKnown)
			require.Empty(t, before.New)
			beforeRecords := slices.Clone(before.Records)
			entryReads(fake)

			tc.change(&fake.sel)
			got := collectSEL(t, r)
			assert.Equal(t, tc.wantReads, entryReads(fake))
			assert.True(t, got.Loaded)
			if tc.wantRaw != nil {
				assert.Equal(t, tc.wantRaw, recordsRaw(got))
			} else {
				assert.Equal(t, tc.wantIDs, recordIDs(got))
			}
			assert.Equal(t, beforeRecords, before.Records, "published records never change")
			assert.True(t, got.NewKnown)
			assert.Equal(t, countRaw(tc.wantNew), countNew(got.New), "records read again are not new")

			got = collectSEL(t, r)
			assert.Empty(t, entryReads(fake), "the next unchanged collection reads no record")
			assert.True(t, got.NewKnown)
			assert.Empty(t, got.New, "new records are reported once")
		})
	}
}

func TestReader_SEL_EventDuringRead(t *testing.T) {
	// The BMC logs an event after Get SEL Info, while records are read, so the
	// read ends with more records than the reported entry count.
	tests := map[string]struct {
		// appended is logged before the collection; none means the event
		// interrupts the first read.
		appended  [][selRecordSize]byte
		event     [selRecordSize]byte
		wantReads []uint16
		want      [][selRecordSize]byte
		// wantNew are the new records of the collection; an event during the
		// first read is history.
		wantNew []uint16
	}{
		"first read": {
			event:     tempEvent(4, 0x60000004),
			wantReads: []uint16{0x0000, 2, 3, 4, 0x0000},
			want:      tempEvents(1, 2, 3, 4),
		},
		"appending": {
			appended:  tempEvents(4),
			event:     tempEvent(5, 0x60000005),
			wantReads: []uint16{0xffff, 3, 4, 5, 0x0000},
			want:      tempEvents(1, 2, 3, 4, 5),
			wantNew:   []uint16{4, 5},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3)
			r := newSELTestReader(fake, 0, 0)
			if tc.appended != nil {
				collectSEL(t, r)
				entryReads(fake)
				fake.sel.add(tc.appended...)
			}
			reads := 0
			fake.sel.onEntry = func() {
				if reads++; reads == 2 {
					fake.sel.add(tc.event)
				}
			}

			got := collectSEL(t, r)
			assert.Equal(t, tc.wantReads, entryReads(fake), "the oldest record shows that nothing was overwritten")
			assert.Equal(t, tc.want, recordsRaw(got), "no record the BMC holds is dropped")
			assert.True(t, got.NewKnown)
			assert.Equal(t, countRaw(tempEvents(tc.wantNew...)), countNew(got.New))

			got = collectSEL(t, r)
			assert.Equal(t, []uint16{0xffff}, entryReads(fake), "the new count leads to one check of the newest record")
			assert.Equal(t, tc.want, recordsRaw(got))
			assert.True(t, got.NewKnown)
			assert.Empty(t, got.New, "the event is counted once")
			collectSEL(t, r)
			assert.Empty(t, entryReads(fake))
		})
	}
}

func TestReader_SEL_RolloverDuringRead(t *testing.T) {
	// A SEL with room for 10 records overwrites its oldest records while a read
	// that spans several collections is in progress: the surplus appears only
	// when the read reaches the new records, after Get SEL Info stopped changing.
	tests := map[string]struct {
		initial []uint16
		// loaded reads the whole log before the rollover.
		loaded    bool
		rollover  func(*fakeSEL)
		entryCost time.Duration
		// wantIDs are the records after each collection that follows the rollover.
		wantIDs [][]uint16
		// wantNewKnown and wantNew are the new-record report of each of those
		// collections. The newest record counts in the collection that first
		// reads it, when the change is checked.
		wantNewKnown []bool
		wantNew      [][]uint16
	}{
		"first read": {
			initial: []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			rollover: func(s *fakeSEL) {
				s.records = s.records[2:]
				s.add(tempEvents(11, 12)...)
			},
			entryCost: 300 * time.Millisecond, // 4 records per collection
			wantIDs: [][]uint16{
				{1, 2, 3, 4, 5, 6, 7, 8},
				{3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			},
			wantNewKnown: []bool{false, true},
			wantNew:      [][]uint16{nil, nil},
		},
		"catching up after the first read": {
			initial: []uint16{1, 2, 3, 4, 5, 6, 7, 8},
			loaded:  true,
			rollover: func(s *fakeSEL) {
				s.records = s.records[4:]
				s.add(tempEvents(9, 10, 11, 12, 13, 14)...)
			},
			entryCost: 500 * time.Millisecond, // 2 records per collection
			wantIDs: [][]uint16{
				{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				{5, 6, 7, 8, 9, 10, 11, 12},
				{5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
			},
			wantNewKnown: []bool{true, true, true},
			wantNew:      [][]uint16{{14, 9, 10}, {11, 12}, {13}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(tc.initial...)
			r := newSELTestReader(fake, 4*time.Second, tc.entryCost)
			sel := collectSEL(t, r)
			for tc.loaded && !sel.Loaded {
				sel = collectSEL(t, r)
			}
			tc.rollover(&fake.sel)

			for i, want := range tc.wantIDs {
				sel = collectSEL(t, r)
				assert.Equal(t, want, recordIDs(sel), "collection %d after the rollover", i+1)
				assert.Equal(t, tc.wantNewKnown[i], sel.NewKnown, "collection %d after the rollover", i+1)
				assert.Equal(t, countRaw(tempEvents(tc.wantNew[i]...)), countNew(sel.New), "collection %d after the rollover", i+1)
			}
			assert.True(t, sel.Loaded)
			assert.Equal(t, 10, sel.Entries)
		})
	}
}

func TestReader_SEL_OldestRecordUnknown(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3)
	r := newSELTestReader(fake, 0, 0)
	collectSEL(t, r)
	entryReads(fake)

	// The BMC's oldest record is none of the cached ones: the log was rewritten.
	fake.sel.records = append([][selRecordSize]byte{tempEvent(9, 0x70000009)}, fake.sel.records[2])
	fake.sel.add(tempEvents(4)...)
	got := collectSEL(t, r)
	assert.Equal(t, []uint16{0xffff, 3, 4, 0x0000}, entryReads(fake))
	assert.Empty(t, got.Records)
	assert.False(t, got.Loaded, "the log is read again")
	assert.True(t, got.NewKnown)
	assert.Equal(t, countRaw(tempEvents(4, 9)), countNew(got.New))

	got = collectSEL(t, r)
	assert.Equal(t, []uint16{0x0000, 3, 4, 0x0000}, entryReads(fake))
	assert.Equal(t, []uint16{9, 3, 4}, recordIDs(got))
	assert.True(t, got.Loaded)
	assert.True(t, got.NewKnown)
	assert.Empty(t, got.New, "records read again are not new")
}

func TestReader_SEL_InventoryChange(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2)
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }
	before := collectSEL(t, r)
	require.Equal(t, "CPU Temp", before.Records[0].Sensor)

	// The periodic rediscovery of an unchanged repository keeps the published records.
	now = now.Add(inventoryMaxAge)
	same := collectSEL(t, r)
	assert.Same(t, &before.Records[0], &same.Records[0], "unchanged names do not replace the records")

	// A renamed sensor renames its cached records; the SEL itself is unchanged.
	fake.records[0] = fullRecord(1, 10, "Renamed")
	fake.additionTime++
	fake.requests = nil
	got := collectSEL(t, r)
	assert.Empty(t, entryReads(fake))
	assert.Equal(t, []string{"Renamed", "Renamed"}, []string{got.Records[0].Sensor, got.Records[1].Sensor})
	assert.Equal(t, "CPU Temp", before.Records[0].Sensor, "published records never change")
}

func TestReader_SEL_UsageAfterFailedCheck(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2)
	fake.sel.freeBytes = 100
	r := newSELTestReader(fake, 0, 0)
	collectSEL(t, r)

	fake.sel.add(tempEvents(3)...)
	fake.sel.freeBytes = 84
	fake.sel.overflow = true
	fake.onExchange = func(req types.Request) error {
		if q, ok := req.(*storage.GetSELEntryRequest); ok && q.RecordID == 0xffff {
			return errors.New("transport failed")
		}
		return nil
	}
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, []string{selRecordsWarning}, got.Warnings)
	assert.Equal(t, new(3), got.SELEntries)
	assert.Equal(t, []uint16{1, 2}, recordIDs(got.SEL), "the records wait for the next read")
	assert.Equal(t, []any{3, 84, true}, []any{got.SEL.Entries, got.SEL.FreeBytes, got.SEL.Overflow},
		"usage is the latest Get SEL Info answer")

	fake.onExchange = nil
	assert.Equal(t, []uint16{1, 2, 3}, recordIDs(collectSEL(t, r)), "the failed check is retried")
}

func TestReader_SEL_Reservation(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2)
	fake.sel.requireReservation = true
	r := newSELTestReader(fake, 0, 0)

	reserves := func() int {
		n := 0
		for _, req := range fake.requests {
			if _, ok := req.(*storage.ReserveSELRequest); ok {
				n++
			}
		}
		fake.requests = nil
		return n
	}

	assert.Equal(t, []uint16{1, 2}, recordIDs(collectSEL(t, r)))
	assert.Equal(t, 1, reserves(), "reserved once after 0xC5, then reused")

	fake.sel.add(tempEvents(3)...)
	assert.Equal(t, []uint16{1, 2, 3}, recordIDs(collectSEL(t, r)))
	assert.Equal(t, 1, reserves(), "a change cancels the reservation")

	fake.onExchange = func(req types.Request) error {
		if _, ok := req.(*storage.GetSELEntryRequest); ok {
			return types.NewResponseError(types.CodeReservationCanceled, "reservation canceled")
		}
		return nil
	}
	fake.sel.add(tempEvents(4)...)
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, []string{selRecordsWarning}, got.Warnings)
	assert.True(t, got.SEL.ReadFailed)
	assert.Equal(t, []uint16{1, 2, 3}, recordIDs(got.SEL))
	assert.Equal(t, maxReservationRetries, reserves(), "reservation retries are bounded")
}

func TestReader_SEL_Clock(t *testing.T) {
	logged := uint32(testNow.Unix())
	tests := map[string]struct {
		utcOffset    uint16
		utcOffsetErr error
		wantOffset   *time.Duration
		wantTime     time.Time
		wantWarning  bool
	}{
		"positive offset": {
			utcOffset:  120,
			wantOffset: new(2 * time.Hour),
			wantTime:   testNow.Add(-2 * time.Hour),
		},
		"negative offset": {
			utcOffset:  uint16(0x10000 - 300),
			wantOffset: new(-5 * time.Hour),
			wantTime:   testNow.Add(5 * time.Hour),
		},
		"unspecified offset": {
			utcOffset: 0x07ff,
			wantTime:  testNow,
		},
		"out of range offset": {
			utcOffset: 1441,
			wantTime:  testNow,
		},
		"command not supported": {
			utcOffsetErr: types.NewResponseError(types.CodeInvalidCommand, "invalid command"),
			wantTime:     testNow,
		},
		"transport failure": {
			utcOffsetErr: errors.New("transport failed"),
			wantWarning:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = [][selRecordSize]byte{tempEvent(1, logged)}
			fake.sel.utcOffset, fake.sel.utcOffsetErr = tc.utcOffset, tc.utcOffsetErr
			fake.sel.clock = uint32(testNow.Add(2*time.Hour + 400*time.Millisecond).Unix())
			r := newSELTestReader(fake, 0, 0)

			got, err := r.Collect(t.Context(), true)
			require.NoError(t, err)
			sel := got.SEL
			if tc.wantWarning {
				assert.Equal(t, []string{selClockWarning}, got.Warnings)
				assert.False(t, sel.ReadFailed, "the records are current")
				assert.Len(t, sel.Records, 1)
				assert.Nil(t, sel.ClockSkew)
				return
			}
			assert.Empty(t, got.Warnings)
			assert.Equal(t, tc.wantOffset, sel.UTCOffset)
			assert.Equal(t, new(2*time.Hour), sel.ClockSkew)
			require.Len(t, sel.Records, 1)
			at, ok := sel.Time(sel.Records[0])
			require.True(t, ok)
			assert.Equal(t, tc.wantTime, at)
		})
	}
}

func TestReader_SEL_ClockReadDespiteRecordFailure(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3)
	fake.sel.utcOffset = 120
	fake.onExchange = func(req types.Request) error {
		if q, ok := req.(*storage.GetSELEntryRequest); ok && q.RecordID == 2 {
			return types.NewResponseError(0xff, "unspecified error")
		}
		return nil
	}
	got, err := newSELTestReader(fake, 0, 0).Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, []string{selRecordsWarning}, got.Warnings)
	assert.Equal(t, []uint16{1}, recordIDs(got.SEL))
	assert.Equal(t, new(2*time.Hour), got.SEL.UTCOffset, "the clock read does not depend on the records")
	assert.NotNil(t, got.SEL.ClockSkew)
}

func TestReader_SEL_ClockRefresh(t *testing.T) {
	fake := newFakeBMC()
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }
	clockReads := func() int {
		n := 0
		for _, req := range fake.requests {
			switch req.(type) {
			case *storage.GetSELTimeRequest, *storage.GetSELTimeUTCOffsetRequest:
				n++
			}
		}
		fake.requests = nil
		return n
	}

	assert.Nil(t, collectSEL(t, r).UTCOffset)
	assert.Equal(t, 2, clockReads())

	fake.sel.utcOffset = 60
	now = now.Add(selClockMaxAge - time.Second)
	assert.Nil(t, collectSEL(t, r).UTCOffset)
	assert.Zero(t, clockReads(), "the clock is read at most every selClockMaxAge")

	now = now.Add(time.Second)
	assert.Equal(t, new(time.Hour), collectSEL(t, r).UTCOffset)
	assert.Equal(t, 2, clockReads())
}

func TestReader_SEL_ClockFailureRetry(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.utcOffset = 60
	fake.sel.utcOffsetErr = errors.New("transport failed")
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }
	offsetReads := func() int {
		n := 0
		for _, req := range fake.requests {
			if _, ok := req.(*storage.GetSELTimeUTCOffsetRequest); ok {
				n++
			}
		}
		fake.requests = nil
		return n
	}

	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, []string{selClockWarning}, got.Warnings)
	assert.Equal(t, 1, offsetReads())

	now = now.Add(selClockMaxAge - time.Second)
	sel := collectSEL(t, r)
	assert.Zero(t, offsetReads(), "a failed clock read is not retried every collection")
	assert.Nil(t, sel.ClockSkew)

	fake.sel.utcOffsetErr = nil
	now = now.Add(time.Second)
	assert.Equal(t, new(time.Hour), collectSEL(t, r).UTCOffset)
	assert.Equal(t, 1, offsetReads())
}

func TestReader_SEL_Records(t *testing.T) {
	const at = 0x60000000
	lunSensor := fullRecord(3, 10, "LUN1 Temp")
	lunSensor[recordOwnerLUN] = 0x01
	tests := map[string]struct {
		record [selRecordSize]byte
		want   SELRecord
		// wantUndated: SEL.Time reports no date.
		wantUndated bool
	}{
		"threshold event with trigger values": {
			record: selEvent(1, at, 0x0020, 0x01, 10, 0x01, 0x59, 90, 85),
			want: SELRecord{
				Sensor:     "CPU Temp",
				SensorType: "Temperature",
				Generator:  "BMC (0x20)",
				Event:      "Upper Critical - going high (reading 90 Celsius, threshold 85 Celsius)",
				Direction:  "Assertion",
				State:      StateCritical,
				Threshold:  true,
			},
		},
		"threshold deassertion without trigger values": {
			record: selEvent(1, at, 0x0020, 0x01, 10, 0x81, 0x09, 90, 85),
			want: SELRecord{
				Sensor:     "CPU Temp",
				SensorType: "Temperature",
				Generator:  "BMC (0x20)",
				Event:      "Upper Critical - going high",
				Direction:  "Deassertion",
				State:      StateWarning,
				Threshold:  true,
			},
		},
		"sensor on another LUN": {
			record: selEvent(1, at, 0x0120, 0x01, 10, 0x01, 0x07, 0, 0),
			want: SELRecord{
				Sensor:     "LUN1 Temp",
				SensorType: "Temperature",
				Generator:  "BMC (0x20)",
				Event:      "Upper Non-critical - going high",
				Direction:  "Assertion",
				State:      StateWarning,
				Threshold:  true,
			},
		},
		"discrete SDR sensor": {
			record: selEvent(1, at, 0x0020, 0x07, 11, 0x6f, 0x0a, 0, 0),
			want: SELRecord{
				Sensor:     "CPU Presence",
				SensorType: "Processor",
				Generator:  "BMC (0x20)",
				Event:      "Processor Automatically Throttled",
				Direction:  "Assertion",
				State:      StateWarning,
			},
		},
		"BMC sensor without an SDR": {
			record: selEvent(1, at, 0x0020, 0x12, 255, 0x6f, 0x05, 0, 0),
			want: SELRecord{
				Sensor:     "BMC #255",
				SensorType: "System Event",
				Generator:  "BMC (0x20)",
				Event:      "Timestamp Clock Synch",
				Direction:  "Assertion",
				State:      StateWarning,
			},
		},
		"BIOS": {
			record: selEvent(1, at, 0x0001, 0x1f, 34, 0x6f, 0x06, 0, 0),
			want: SELRecord{
				Sensor:     "BIOS #34",
				SensorType: "OS Boot",
				Generator:  "BIOS (0x01)",
				Event:      "boot completed - boot device not specified",
				Direction:  "Assertion",
				State:      StateWarning,
			},
		},
		"SMI handler": {
			record: selEvent(1, at, 0x0021, 0x0c, 0, 0x6f, 0x00, 0, 0),
			want: SELRecord{
				Sensor:     "SMI Handler #0",
				SensorType: "Memory",
				Generator:  "SMI Handler (0x21)",
				Event:      "Correctable ECC / other correctable memory error",
				Direction:  "Assertion",
				State:      StateWarning,
			},
		},
		"power unit deassertion": {
			record: selEvent(1, at, 0x0041, 0x09, 3, 0xef, 0x05, 0, 0),
			want: SELRecord{
				Sensor:     "System Management Software #3",
				SensorType: "Power Unit",
				Generator:  "System Management Software (0x41)",
				Event:      "Soft Power Control Failure",
				Direction:  "Deassertion",
				State:      StateCritical,
			},
		},
		"other IPMB controller": {
			record: selEvent(1, at, 0x002c, 0x01, 5, 0x01, 0x07, 0, 0),
			want: SELRecord{
				Sensor:     "Controller 0x2c #5",
				SensorType: "Temperature",
				Generator:  "Controller 0x2c",
				Event:      "Upper Non-critical - going high",
				Direction:  "Assertion",
				State:      StateWarning,
				Threshold:  true,
			},
		},
		"unrecognized OEM event": {
			record: selEvent(1, at, 0x0061, 0xc0, 2, 0x70, 0x03, 0, 0),
			want: SELRecord{
				Sensor:     "OEM Software #2",
				SensorType: "OEM",
				Generator:  "OEM Software (0x61)",
				Event:      "Unrecognized event: type 0x70, offset 3",
				Direction:  "Assertion",
				State:      StateUnknown,
			},
		},
		"timestamped OEM record": {
			record: oemRecord(1, 0xc1, at, 0x0a, 0x3c, 0x00),
			want: SELRecord{
				Event: "OEM record type 0xc1, manufacturer ID 15370",
				State: StateUnknown,
			},
		},
		"non-timestamped OEM record": {
			record: oemRecord(1, 0xe0, at, 0, 0, 0),
			want: SELRecord{
				Event: "OEM record type 0xe0",
				State: StateUnknown,
			},
			wantUndated: true,
		},
		"reserved record type with the system event layout": {
			// Real BMC firmware logs power-off events from the BIOS this way.
			record: withRecordType(selEvent(1, at, 0x0001, 0x09, 255, 0x6f, 0x00, 0, 0), 0x04),
			want: SELRecord{
				Sensor:     "BIOS #255",
				SensorType: "Power Unit",
				Generator:  "BIOS (0x01)",
				Event:      "Power Off / Power Down [record type 0x04]",
				Direction:  "Assertion",
				State:      StateUnknown,
			},
		},
		"pre-init timestamp": {
			record: selEvent(1, 35, 0x0001, 0x12, 0, 0x6f, 0x01, 0, 0),
			want: SELRecord{
				Sensor:     "BIOS #0",
				SensorType: "System Event",
				Generator:  "BIOS (0x01)",
				Event:      "OEM System Boot Event",
				Direction:  "Assertion",
				State:      StateNominal,
			},
			wantUndated: true,
		},
		"unspecified timestamp": {
			record: selEvent(1, 0xffffffff, 0x008f, 0x12, 0, 0x6f, 0x01, 0, 0),
			want: SELRecord{
				Sensor:     "Terminal Mode Remote Console #0",
				SensorType: "System Event",
				Generator:  "Terminal Mode Remote Console (0x8f)",
				Event:      "OEM System Boot Event",
				Direction:  "Assertion",
				State:      StateNominal,
			},
			wantUndated: true,
		},
		"remote console": {
			record: selEvent(1, at, 0x0083, 0x12, 0, 0x6f, 0x01, 0, 0),
			want: SELRecord{
				Sensor:     "Remote Console #0",
				SensorType: "System Event",
				Generator:  "Remote Console (0x83)",
				Event:      "OEM System Boot Event",
				Direction:  "Assertion",
				State:      StateNominal,
			},
		},
		"reserved software ID": {
			record: selEvent(1, at, 0x0091, 0x12, 0, 0x6f, 0x01, 0, 0),
			want: SELRecord{
				Sensor:     "Software #0",
				SensorType: "System Event",
				Generator:  "Software (0x91)",
				Event:      "OEM System Boot Event",
				Direction:  "Assertion",
				State:      StateNominal,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.records = append(fake.records, lunSensor)
			fake.sel.records = [][selRecordSize]byte{tc.record}
			sel := collectSEL(t, newSELTestReader(fake, 0, 0))

			want := tc.want
			want.ID = 1
			want.Timestamp = binary.LittleEndian.Uint32(tc.record[3:])
			// Only OEM record types from E0h carry no BMC timestamp.
			want.Timestamped = tc.record[2] < 0xe0
			want.Raw = tc.record
			require.Len(t, sel.Records, 1)
			assert.Equal(t, want, sel.Records[0])
			_, dated := sel.Time(sel.Records[0])
			assert.Equal(t, !tc.wantUndated, dated)
		})
	}
}

// oemRecord encodes an OEM record; timestamped types carry the manufacturer ID.
func oemRecord(id uint16, recordType byte, timestamp uint32, manufacturer ...byte) [selRecordSize]byte {
	var r [selRecordSize]byte
	binary.LittleEndian.PutUint16(r[0:], id)
	r[2] = recordType
	binary.LittleEndian.PutUint32(r[3:], timestamp)
	copy(r[7:], manufacturer)
	return r
}

func TestReader_SEL_Failures(t *testing.T) {
	transportFailure := errors.New("transport failed")
	tests := map[string]struct {
		// fail breaks the fake after the first three records were cached.
		fail func(*fakeBMC)
		// wantIDs are the records published by the failing collection.
		wantIDs      []uint16
		wantLoaded   bool
		wantWarnings []string
		wantEntries  bool
		// wantNew are the new records reported after recovery.
		wantNew []uint16
	}{
		"SEL Info fails": {
			fail:         func(b *fakeBMC) { b.selErr = transportFailure },
			wantIDs:      []uint16{1, 2, 3},
			wantLoaded:   true,
			wantWarnings: []string{"SEL entry count unavailable"},
		},
		"record read fails while appending": {
			fail: func(b *fakeBMC) {
				b.sel.add(tempEvents(4, 5)...)
				b.onExchange = func(req types.Request) error {
					if q, ok := req.(*storage.GetSELEntryRequest); ok && q.RecordID == 5 {
						return transportFailure
					}
					return nil
				}
			},
			wantIDs:      []uint16{1, 2, 3, 4},
			wantLoaded:   true,
			wantWarnings: []string{selRecordsWarning},
			wantEntries:  true,
			wantNew:      []uint16{4, 5},
		},
		"truncated record": {
			fail: func(b *fakeBMC) {
				b.sel.add(tempEvents(4)...)
				b.sel.truncate = true
			},
			wantIDs:      []uint16{1, 2, 3},
			wantLoaded:   true,
			wantWarnings: []string{selRecordsWarning},
			wantEntries:  true,
			wantNew:      []uint16{4},
		},
		"oldest record read fails after a rollover": {
			fail: func(b *fakeBMC) {
				b.sel.records = b.sel.records[2:]
				b.sel.add(tempEvents(4, 5)...)
				b.onExchange = func(req types.Request) error {
					if q, ok := req.(*storage.GetSELEntryRequest); ok && q.RecordID == 0x0000 {
						return transportFailure
					}
					return nil
				}
			},
			wantIDs:      []uint16{1, 2, 3, 4, 5},
			wantLoaded:   true,
			wantWarnings: []string{selRecordsWarning},
			wantEntries:  true,
			wantNew:      []uint16{4, 5},
		},
		"next record vanished": {
			fail: func(b *fakeBMC) {
				b.sel.add(tempEvents(4)...)
				b.sel.next = map[uint16]uint16{4: 9}
			},
			wantWarnings: []string{selRecordsWarning},
			wantEntries:  true,
			wantNew:      []uint16{4},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3)
			r := newSELTestReader(fake, 0, 0)
			collectSEL(t, r)

			tc.fail(fake)
			got, err := r.Collect(t.Context(), true)
			require.NoError(t, err, "SEL failures do not fail the collection")
			assert.Equal(t, tc.wantWarnings, got.Warnings)
			assert.Equal(t, tc.wantEntries, got.SELEntries != nil)
			require.NotNil(t, got.SEL)
			assert.True(t, got.SEL.ReadFailed)
			assert.Equal(t, tc.wantLoaded, got.SEL.Loaded)
			assert.Equal(t, tc.wantIDs, recordIDs(got.SEL))
			assert.False(t, got.SEL.NewKnown, "a failed read leaves a gap")
			assert.Empty(t, got.SEL.New)

			fake.selErr, fake.onExchange, fake.sel.truncate, fake.sel.next = nil, nil, false, nil
			recovered := collectSEL(t, r)
			assert.True(t, recovered.Loaded)
			assert.Equal(t, recordIDs(&SEL{
				Records: decodeAll(fake.sel.records),
			}), recordIDs(recovered))
			assert.True(t, recovered.NewKnown)
			assert.Equal(t, countRaw(tempEvents(tc.wantNew...)), countNew(recovered.New), "records read before the failure count once")
			assert.Empty(t, collectSEL(t, r).New)
		})
	}
}

func decodeAll(records [][selRecordSize]byte) []SELRecord {
	var out []SELRecord
	for _, raw := range records {
		out = append(out, decodeSELRecord(raw, nil))
	}
	return out
}

func TestReader_SEL_Disabled(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1)
	got, err := newTestReader(fake).Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Nil(t, got.SEL)
	assert.Nil(t, got.SELEntries)
	for _, req := range fake.requests {
		switch req.(type) {
		case *storage.GetSELInfoRequest, *storage.GetSELEntryRequest, *storage.ReserveSELRequest,
			*storage.GetSELTimeRequest, *storage.GetSELTimeUTCOffsetRequest:
			t.Errorf("SEL command %T sent with SEL collection disabled", req)
		}
	}
}

func TestReader_SEL_CanceledDuringRecordRead(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2)
	r := newTestReader(fake)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fake.onExchange = func(req types.Request) error {
		if _, ok := req.(*storage.GetSELEntryRequest); ok {
			cancel()
		}
		return nil
	}

	got, err := r.Collect(ctx, true)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
	assert.Equal(t, 1, fake.closes)
}

func TestReader_SEL_NewRecordsAfterCanceledCollection(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2)
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }
	collectSEL(t, r)

	// The collection is canceled after the new record was read, while the
	// due clock refresh runs.
	fake.sel.add(tempEvents(3)...)
	now = now.Add(selClockMaxAge)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fake.onExchange = func(req types.Request) error {
		if _, ok := req.(*storage.GetSELTimeUTCOffsetRequest); ok {
			cancel()
		}
		return nil
	}
	_, err := r.Collect(ctx, true)
	require.ErrorIs(t, err, context.Canceled)

	fake.onExchange = nil
	got := collectSEL(t, r)
	assert.True(t, got.NewKnown)
	assert.Equal(t, countRaw(tempEvents(3)), countNew(got.New), "a record of a canceled collection is reported later")
	assert.Empty(t, collectSEL(t, r).New)
}

func TestReader_SEL_NewRecordsWhileReadingAgain(t *testing.T) {
	// A rewritten newest record resets the cache; the log is read again over
	// several collections, and only the records it did not hold are new.
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3, 4, 5, 6, 7, 8)
	r := newSELTestReader(fake, 4*time.Second, 300*time.Millisecond) // 4 records per collection
	for !collectSEL(t, r).Loaded {
	}

	changed := tempEvent(8, 0x70000008)
	fake.sel.records = fake.sel.records[:7]
	fake.sel.add(changed, tempEvent(9, 0x60000009))
	var known []bool
	reported := make(map[newRecordKey]int)
	var got *SEL
	for got == nil || !got.Loaded {
		got = collectSEL(t, r)
		known = append(known, got.NewKnown)
		addNewCounts(reported, countNew(got.New))
	}
	assert.Equal(t, []bool{true, true, true}, known, "observed events are reported during rescans")
	want := [][selRecordSize]byte{changed, tempEvent(9, 0x60000009)}
	assert.Equal(t, countRaw(want), reported)
	assert.Len(t, r.sel.seen, len(got.Records), "a complete read keeps only the content of the cached records")
	assert.Empty(t, collectSEL(t, r).New)
}

func TestReader_SEL_ResetWhileReadingAgain(t *testing.T) {
	// A record vanishes while the log is read again after a rewrite: the
	// second reset must keep the records known before the first one.
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3, 4, 5, 6, 7, 8)
	r := newSELTestReader(fake, 4*time.Second, 300*time.Millisecond) // 4 records per collection
	for !collectSEL(t, r).Loaded {
	}

	changed := tempEvent(8, 0x70000008)
	fake.sel.records = fake.sel.records[:7]
	fake.sel.add(changed, tempEvent(9, 0x60000009))
	first := collectSEL(t, r)
	require.False(t, first.Loaded)
	reported := countNew(first.New)
	fake.sel.next = map[uint16]uint16{5: 0x99}
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	require.Equal(t, []string{selRecordsWarning}, got.Warnings)

	fake.sel.next = nil
	for range 4 {
		addNewCounts(reported, countNew(collectSEL(t, r).New))
	}
	assert.Equal(t, countRaw(tempEvents(8, 9)), reported)
}

func TestReader_SEL_ReadRecordsSurviveClear(t *testing.T) {
	// The BMC clears its log in the middle of a collection, after the
	// collection read the new records below; it then logs that the log was
	// cleared. Every record read is reported once.
	ierr := selEvent(4, 0x60000004, 0x0020, 0x07, 100, 0x6f, 0x00, 0xff, 0xff)
	cleared := selEvent(1, 0x60000010, 0x0020, 0x10, 101, 0x6f, 0x02, 0xff, 0xff)
	tests := map[string]struct {
		appended [][selRecordSize]byte
		// clearBefore is the Get SEL Entry request the BMC answers after the clear.
		clearBefore uint16
		// want are the records read before the clear (the newest first, from the
		// check of the change) and the record of the clear.
		want [][selRecordSize]byte
	}{
		"while reading the new records": {
			appended:    [][selRecordSize]byte{ierr, tempEvent(5, 0x60000005)},
			clearBefore: 5,
			want:        [][selRecordSize]byte{tempEvent(5, 0x60000005), ierr, cleared},
		},
		"while checking where to resume": {
			appended:    [][selRecordSize]byte{ierr},
			clearBefore: 3,
			want:        [][selRecordSize]byte{ierr, cleared},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3)
			r := newSELTestReader(fake, 0, 0)
			collectSEL(t, r)

			fake.sel.add(tc.appended...)
			fake.onExchange = func(req types.Request) error {
				q, ok := req.(*storage.GetSELEntryRequest)
				if ok && q.RecordID == tc.clearBefore && len(fake.sel.records) > 1 {
					fake.sel.clear()
					fake.sel.add(cleared)
				}
				return nil
			}
			got, err := r.Collect(t.Context(), true)
			require.NoError(t, err)
			fake.onExchange = nil

			reported := make(map[newRecordKey]int)
			addNewCounts(reported, countNew(got.SEL.New))
			for range 3 {
				addNewCounts(reported, countNew(collectSEL(t, r).New))
			}
			assert.Equal(t, countRaw(tc.want), reported)
		})
	}
}

func TestReader_SEL_OverwrittenPastTheRead(t *testing.T) {
	// A full log of four records whose new records take the overwritten
	// records' IDs. A burst that overwrites records ahead of a read and reuses
	// their IDs must not let the read end early: no new record is skipped and
	// no overwritten record stays cached.
	type collection struct {
		logged int // records logged before the collection
		// hidden records leave Get SEL Info unchanged, as records logged in
		// the second of its previous answer on a full log do.
		hidden bool
		// erased changes the erase time before the collection, so the log is
		// read again from its oldest record.
		erased bool
		// duringRead records are logged after the collection's second Get SEL Entry.
		duringRead int
	}
	tests := map[string]struct {
		entryCost   time.Duration
		collections []collection
	}{
		"between the collections of a read": {
			entryCost:   500 * time.Millisecond, // 2 records per collection
			collections: []collection{{logged: 3}, {logged: 4}, {logged: 1}, {}, {}, {}},
		},
		"in the second of the previous Get SEL Info answer": {
			entryCost:   500 * time.Millisecond,
			collections: []collection{{logged: 3}, {logged: 4, hidden: true}, {}, {logged: 1}, {}, {}, {}},
		},
		"while reading": {
			collections: []collection{{logged: 1, erased: true, duringRead: 3}, {}, {}, {}, {}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3, 4)
			receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
			r := newSELTestReader(fake, 4*time.Second, tc.entryCost)
			r.newTransport = func() (transport, error) { return receiving, nil }
			for !collectSEL(t, r).Loaded {
			}
			advance := fake.sel.onEntry

			ts := uint32(0x70000000)
			var logged [][selRecordSize]byte
			overwrite := func(n int, hidden bool) {
				for range n {
					ts++
					logged = append(logged, fake.sel.overwrite(ts, hidden))
				}
			}
			reported := make(map[newRecordKey]int)
			var sel *SEL
			for _, c := range tc.collections {
				overwrite(c.logged, c.hidden)
				if c.erased {
					fake.sel.eraseTime++
				}
				reads := 0
				fake.sel.onEntry = func() {
					advance()
					if reads++; reads == 3 {
						overwrite(c.duringRead, false)
					}
				}
				got, err := r.Collect(t.Context(), true)
				require.NoError(t, err)
				sel = got.SEL
				addNewCounts(reported, countNew(sel.New))
			}

			assertReportedOnce(t, fake, receiving, sel, logged, reported)
		})
	}
}

func TestReader_SEL_DeletedAheadOfTheRead(t *testing.T) {
	// While new records are read, the BMC deletes the next one (the erase time
	// changes) and gives its ID to a new record, so the read jumps to the
	// newest record. The BMC's oldest record is still cached; the erase time
	// shows the read skipped records.
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3)
	receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
	r := newSELTestReader(fake, 0, 0)
	r.newTransport = func() (transport, error) { return receiving, nil }
	collectSEL(t, r)

	logged := tempEvents(4, 5, 6, 7)
	fake.sel.add(logged...)
	reusing := tempEvent(5, 0x70000005)
	logged = append(logged, reusing)
	fake.onExchange = func(req types.Request) error {
		if q, ok := req.(*storage.GetSELEntryRequest); ok && q.RecordID == 5 && !slices.Contains(fake.sel.records, reusing) {
			fake.sel.records = slices.DeleteFunc(slices.Clone(fake.sel.records), func(rec [selRecordSize]byte) bool {
				return binary.LittleEndian.Uint16(rec[:]) == 5
			})
			fake.sel.eraseTime++
			fake.sel.add(reusing)
		}
		return nil
	}

	reported := make(map[newRecordKey]int)
	var sel *SEL
	for range 4 {
		sel = collectSEL(t, r)
		addNewCounts(reported, countNew(sel.New))
	}
	assertReportedOnce(t, fake, receiving, sel, logged, reported)
}

func TestReader_SEL_StormKeepsTheCacheWithinTheLog(t *testing.T) {
	// A full log of twenty records with rollover gains one record more per
	// collection than a collection reads, so the read falls further behind:
	// the cache never grows past the log's size, and once the storm ends every
	// new record still held is reported once.
	const capacity = 20
	fake := newFakeBMC()
	for id := range uint16(capacity) {
		fake.sel.records = append(fake.sel.records, tempEvent(id+1, 0x60000000+uint32(id)))
	}
	receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
	r := newSELTestReader(fake, 4*time.Second, 300*time.Millisecond) // 4 records per collection
	r.newTransport = func() (transport, error) { return receiving, nil }
	for !collectSEL(t, r).Loaded {
	}

	var logged [][selRecordSize]byte
	reported := make(map[newRecordKey]int)
	var sel *SEL
	for collection := range 50 {
		if collection < 30 {
			for range 5 {
				rec := tempEvent(uint16(capacity+len(logged)+1), 0x70000000+uint32(len(logged)))
				fake.sel.add(rec)
				fake.sel.records = fake.sel.records[1:]
				logged = append(logged, rec)
			}
		}
		sel = collectSEL(t, r)
		require.LessOrEqual(t, len(sel.Records), capacity, "collection %d", collection)
		addNewCounts(reported, countNew(sel.New))
	}
	assertReportedOnce(t, fake, receiving, sel, logged, reported)
}

// assertReportedOnce checks a settled SEL against the fake BMC: the cache
// holds the BMC's log, and of the records logged during the test, each the
// Reader received or the BMC still holds is reported exactly once and no other
// record is reported.
func assertReportedOnce(
	t *testing.T,
	fake *fakeBMC,
	receiving *receivingTransport,
	sel *SEL,
	logged [][selRecordSize]byte,
	reported map[newRecordKey]int,
) {
	t.Helper()
	assert.Equal(t, fake.sel.records, recordsRaw(sel), "the cache holds the BMC's log")
	var want [][selRecordSize]byte
	for _, rec := range logged {
		if receiving.received[rec] > 0 || slices.Contains(fake.sel.records, rec) {
			want = append(want, rec)
		}
	}
	assert.Equal(t, countRaw(want), reported, "every new record received or still held is reported once")
}

func TestReader_SEL_ResumeFindsNewestRecord(t *testing.T) {
	// A read spanning collections resumes after a change at its newest cached
	// record; when the BMC reports no record after it, the read is complete.
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3, 4)
	r := newSELTestReader(fake, 4*time.Second, 500*time.Millisecond) // 2 records per collection
	require.False(t, collectSEL(t, r).Loaded)

	fake.sel.records = fake.sel.records[:2]
	fake.sel.additionTime++
	got := collectSEL(t, r)
	assert.True(t, got.Loaded)
	assert.True(t, got.NewKnown)
	assert.Equal(t, []uint16{1, 2}, recordIDs(got))
}

func TestReader_SEL_EntryCountIsTheLatestAnswer(t *testing.T) {
	// A record logged while the collection reads is in the second Get SEL
	// Info answer, which completes the read; the count and the usage agree.
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3)
	r := newSELTestReader(fake, 0, 0)
	fake.sel.onEntry = func() {
		if len(fake.sel.records) == 3 {
			fake.sel.add(tempEvent(4, 0x60000004))
		}
	}
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	require.NotNil(t, got.SELEntries)
	assert.Equal(t, 4, *got.SELEntries)
	assert.Equal(t, 4, got.SEL.Entries)
}
