// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_SEL_BaselineDuringSustainedRollover(t *testing.T) {
	const capacity = 128
	fake := newFakeBMC()
	for id := range uint16(capacity) {
		fake.sel.records = append(fake.sel.records, tempEvent(id+1, 0x60000000+uint32(id)))
	}
	// Four records fit the read budget; five new records each interval keep
	// rollover ahead of the initial read without any transport failures.
	r := newSELTestReader(fake, 10*time.Second, 600*time.Millisecond)
	timestamp := uint32(0x70000000)
	for cycle := range 1000 {
		for range 5 {
			timestamp++
			fake.sel.overwrite(timestamp, false)
		}
		got := collectSEL(t, r)
		require.False(t, got.Loaded, "cycle %d", cycle)
		require.False(t, got.NewKnown, "cycle %d", cycle)
		require.Empty(t, got.New)
		require.Empty(t, r.sel.pending, "baseline reads are history")
		require.LessOrEqual(t, len(r.sel.records), capacity)
		require.LessOrEqual(t, len(r.sel.seen), capacity, "discarded history must not accumulate; cycle %d", cycle)
		fake.requests = nil
	}

	// When rollover stops, the surviving log becomes history. Seeding event
	// accounting must neither report that history nor count a later record twice.
	var baseline *SEL
	for range capacity {
		baseline = collectSEL(t, r)
		require.Empty(t, baseline.New)
		if baseline.Loaded {
			break
		}
	}
	require.True(t, baseline.Loaded)
	require.True(t, baseline.NewKnown)
	assert.Equal(t, fake.sel.records, recordsRaw(baseline))

	timestamp++
	newRecord := fake.sel.overwrite(timestamp, false)
	got := collectSEL(t, r)
	require.True(t, got.NewKnown)
	assert.Equal(t, countRaw([][selRecordSize]byte{newRecord}), countNew(got.New))
	assert.Empty(t, collectSEL(t, r).New)
}

func TestReader_SEL_BaselineStartsNewRecordAccounting(t *testing.T) {
	for name, history := range map[string][]uint16{
		"empty":     nil,
		"populated": {1, 2, 3},
	} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.sel.records = tempEvents(history...)
			r := newSELTestReader(fake, 0, 0)
			baseline := collectSEL(t, r)
			require.True(t, baseline.NewKnown)
			require.Empty(t, baseline.New)

			fake.sel.add(tempEvents(4)...)
			got := collectSEL(t, r)
			require.True(t, got.NewKnown)
			assert.Equal(t, countRaw(tempEvents(4)), countNew(got.New))
			assert.Empty(t, collectSEL(t, r).New)
		})
	}
}
