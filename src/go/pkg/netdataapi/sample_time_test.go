// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestENDTimed(t *testing.T) {
	tests := map[string]struct {
		sample SampleTime
		want   string
	}{
		"time with microseconds": {
			sample: NewSampleTime(time.Unix(1700000000, 250000000)),
			want:   "END 1700000000 250000 1\n\n",
		},
		"microseconds are not padded": {
			sample: NewSampleTime(time.Unix(1700000001, 5000)),
			want:   "END 1700000001 5 1\n\n",
		},
		"sub-microsecond part is dropped": {
			sample: NewSampleTime(time.Unix(1700000002, 200000999)),
			want:   "END 1700000002 200000 1\n\n",
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

// A run's time is its start time as given, also when it is not after an earlier
// one: the Agent, not the plugin, decides what to do with a stale sample.
func TestNewSampleTimeKeepsTheGivenTime(t *testing.T) {
	times := []time.Time{time.Unix(100, 0), time.Unix(97, 300000000), time.Unix(97, 300000000)}
	var got []time.Time
	for _, tm := range times {
		got = append(got, NewSampleTime(tm).Time())
	}
	assert.Equal(t, times, got)
}

func TestNewSampleTimeDoesNotAllocate(t *testing.T) {
	now := time.Unix(1700000000, 0)
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = NewSampleTime(now) }))
}
