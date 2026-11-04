// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_SEL_ProbeAheadOfTrim(t *testing.T) {
	fake := newFakeBMC()
	fake.sel.records = tempEvents(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	accounted := make(map[[selRecordSize]byte]bool)
	for _, raw := range fake.sel.records {
		accounted[raw] = true
	}
	receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
	r := newSELTestReader(fake, 4*time.Second, 0)
	r.newTransport = func() (transport, error) { return receiving, nil }
	collectSEL(t, r)
	now := testNow
	r.now = func() time.Time { return now }
	fake.sel.onEntry = func() { now = now.Add(time.Second) }

	// FIFO rollover uses new IDs for four entries, then reuses ID 5. The
	// newest probe sees its replacement before the one-entry walk reaches it;
	// trimming the overwritten prefix must not forget that newer observation.
	replacement := selEvent(5, 0x70000005, 0x0020, 0x07, 100, 0x6f, 0, 0xff, 0xff)
	fake.sel.records = fake.sel.records[5:]
	fake.sel.add(append(tempEvents(11, 12, 13, 14), replacement)...)
	for cycle := range 6 {
		got := collectSEL(t, r)
		want := newlyReceivedSEL(receiving, accounted)
		require.True(t, got.NewKnown)
		require.Equal(t, want, countNew(got.New), "cycle %d", cycle)
		if cycle == 0 {
			assert.Equal(t, 1, countNew(got.New)[newRecordKey{false, StateCritical}])
			assert.Equal(t, replacement, r.sel.seen[5])
		}
	}
}

func TestReader_SEL_BaselineVerificationFailures(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "read error"
		if canceled {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(1, 2, 3)
			r := newSELTestReader(fake, 0, 0)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			infoReads := 0
			fake.onExchange = func(req types.Request) error {
				if _, ok := req.(*storage.GetSELInfoRequest); ok {
					if infoReads++; infoReads == 2 {
						if canceled {
							cancel()
						} else {
							return errors.New("baseline verification failed")
						}
					}
				}
				return nil
			}
			got, err := r.Collect(ctx, true)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				require.False(t, got.SEL.NewKnown)
			}
			require.Nil(t, r.sel.seen)
			fake.onExchange = nil
			require.Empty(t, collectSEL(t, r).New)
			fake.sel.add(tempEvents(4)...)
			assert.Equal(t, countRaw(tempEvents(4)), countNew(collectSEL(t, r).New))
			assert.Empty(t, collectSEL(t, r).New)
		})
	}
}

func TestReader_SEL_ObservedOccupantReturnsToEarlierContent(t *testing.T) {
	fake := newFakeBMC()
	first := tempEvent(1, 0x60000001)
	second := tempEvent(1, 0x70000001)
	fake.sel.records = [][selRecordSize]byte{first}
	r := newSELTestReader(fake, 0, 0)
	collectSEL(t, r)
	// Observing an intervening occupant distinguishes the final replacement,
	// even if its bytes match an earlier record (for example after a clock reset).
	for _, raw := range [][selRecordSize]byte{second, first} {
		fake.sel.records = nil
		fake.sel.add(raw)
		assert.Equal(t, countRaw([][selRecordSize]byte{raw}), countNew(collectSEL(t, r).New))
		assert.Empty(t, collectSEL(t, r).New)
	}
}

func TestReader_SEL_EventsDuringSustainedRollover(t *testing.T) {
	for _, failReads := range []bool{false, true} {
		name := "successful rescans"
		if failReads {
			name = "repeated partial failures"
		}
		t.Run(name, func(t *testing.T) {
			const capacity = 128
			fake := newFakeBMC()
			accounted := make(map[[selRecordSize]byte]bool)
			for id := range uint16(capacity) {
				raw := tempEvent(id+1, 0x60000000+uint32(id))
				fake.sel.records = append(fake.sel.records, raw)
				accounted[raw] = true
			}
			receiving := &receivingTransport{fakeBMC: fake, received: make(map[[selRecordSize]byte]int)}
			r := newSELTestReader(fake, 10*time.Second, 0)
			r.newTransport = func() (transport, error) { return receiving, nil }
			require.True(t, collectSEL(t, r).NewKnown)
			now := testNow
			r.now = func() time.Time { return now }
			fake.sel.onEntry = func() { now = now.Add(600 * time.Millisecond) }

			pending := make(map[newRecordKey]int)
			timestamp := uint32(0x70000000)
			rescanned := false
			var criticalDuringRescan uint64
			for cycle := range 1000 {
				for range 5 {
					timestamp++
					raw := fake.sel.overwrite(timestamp, false)
					// Alternate critical threshold crossings and processor IERRs.
					if timestamp%2 == 0 {
						raw = selEvent(binary16(raw), timestamp, 0x0020, 0x01, 10, 0x01, 0x09, 0xff, 0xff)
					} else {
						raw = selEvent(binary16(raw), timestamp, 0x0020, 0x07, 100, 0x6f, 0, 0xff, 0xff)
					}
					fake.sel.records[len(fake.sel.records)-1] = raw
				}
				reads := 0
				fake.onExchange = func(req types.Request) error {
					if _, ok := req.(*storage.GetSELEntryRequest); ok && failReads {
						if reads++; reads == 2 {
							return errors.New("entry read failed")
						}
					}
					return nil
				}
				got, err := r.Collect(t.Context(), true)
				require.NoError(t, err)
				addNewCounts(pending, newlyReceivedSEL(receiving, accounted))
				if failReads {
					require.True(t, got.SEL.ReadFailed)
					require.False(t, got.SEL.NewKnown)
					require.Empty(t, got.SEL.New)
				} else {
					require.True(t, got.SEL.NewKnown, "observed events remain known during rescan; cycle %d", cycle)
					require.Equal(t, pending, countNew(got.SEL.New), "cycle %d", cycle)
					clear(pending)
					rescanned = rescanned || !got.SEL.Loaded
					if !got.SEL.Loaded {
						criticalDuringRescan += got.SEL.New.Discrete.Critical
					}
				}
				require.LessOrEqual(t, len(r.sel.seen), capacity, "only the latest occupant of each slot is retained")
				fake.requests = nil
			}
			if !failReads {
				require.True(t, rescanned, "the workload must exercise rescans")
				require.Positive(t, criticalDuringRescan, "critical events are delivered before the rescan completes")
			}
			t.Logf("1000 collections: identities=%d, critical events delivered during rescan=%d, pending=%+v",
				len(r.sel.seen), criticalDuringRescan, r.sel.pending)

			fake.onExchange = nil
			var settled *SEL
			for range capacity {
				settled = collectSEL(t, r)
				addNewCounts(pending, newlyReceivedSEL(receiving, accounted))
				require.True(t, settled.NewKnown)
				require.Equal(t, pending, countNew(settled.New), "unpublished events survive failure without recounting")
				clear(pending)
				if settled.Loaded {
					break
				}
			}
			require.True(t, settled.Loaded)
			assert.Equal(t, fake.sel.records, recordsRaw(settled))
			assert.Empty(t, collectSEL(t, r).New)
		})
	}
}

// The oracle retains received wire identities independently of the Reader's
// mirror and deduplication state. These generated workloads give every new
// occupant unique content, so each first observation contributes once.
func newlyReceivedSEL(receiving *receivingTransport, accounted map[[selRecordSize]byte]bool) map[newRecordKey]int {
	var fresh [][selRecordSize]byte
	for raw := range receiving.received {
		if !accounted[raw] {
			accounted[raw] = true
			fresh = append(fresh, raw)
		}
	}
	return countRaw(fresh)
}

func addNewCounts(dst, src map[newRecordKey]int) {
	for key, n := range src {
		dst[key] += n
	}
}
