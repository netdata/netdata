// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampleClockNext(t *testing.T) {
	at := func(sec, usec int64) time.Time { return time.Unix(sec, usec*1000) }
	tests := map[string]struct {
		times []time.Time
		want  []int64 // issued unix microseconds
	}{
		"increasing times pass through": {
			times: []time.Time{at(100, 200000), at(101, 200001), at(102, 199999)},
			want:  []int64{100200000, 101200001, 102199999},
		},
		"sub-microsecond part is dropped": {
			times: []time.Time{time.Unix(100, 200000999)},
			want:  []int64{100200000},
		},
		"equal time moves one microsecond": {
			times: []time.Time{at(100, 0), at(100, 0)},
			want:  []int64{100000000, 100000001},
		},
		"step back holds just past the latest time until caught up": {
			times: []time.Time{at(100, 0), at(97, 0), at(98, 0), at(101, 0)},
			want:  []int64{100000000, 100000001, 100000002, 101000000},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var clock SampleClock
			var got []int64
			for _, tm := range tc.times {
				got = append(got, clock.Next(tm).Time().UnixMicro())
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// One clock serves every job of a plugin process: concurrent callers get distinct
// times, and each caller's times increase.
func TestSampleClockConcurrent(t *testing.T) {
	var clock SampleClock
	const callers, calls = 8, 500
	now := time.Unix(1700000000, 0)
	issued := make([][]int64, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			for range calls {
				issued[i] = append(issued[i], clock.Next(now).Time().UnixMicro())
			}
		})
	}
	wg.Wait()
	var all []int64
	for _, own := range issued {
		require.True(t, slices.IsSorted(own))
		all = append(all, own...)
	}
	slices.Sort(all)
	assert.Equal(t, callers*calls, len(slices.Compact(all)))
}

func TestENDTimed(t *testing.T) {
	var clock SampleClock
	tests := map[string]struct {
		sample SampleTime
		want   string
	}{
		"time with microseconds": {
			sample: clock.Next(time.Unix(1700000000, 250000000)),
			want:   "END 1700000000 250000 1\n\n",
		},
		"microseconds are not padded": {
			sample: clock.Next(time.Unix(1700000001, 5000)),
			want:   "END 1700000001 5 1\n\n",
		},
		"zero time writes a plain END": {
			sample: SampleTime{},
			want:   "END\n\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var w bytes.Buffer
			New(&w).ENDTimed(tc.sample)
			assert.Equal(t, tc.want, w.String())
		})
	}
}

func TestSampleClockNextDoesNotAllocate(t *testing.T) {
	var clock SampleClock
	now := time.Unix(1700000000, 0)
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = clock.Next(now) }))
}
