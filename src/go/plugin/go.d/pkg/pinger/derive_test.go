// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcMeanJitter(t *testing.T) {
	tests := map[string]struct {
		rtts []time.Duration
		want time.Duration
	}{
		"empty": {
			want: 0,
		},
		"single": {
			rtts: []time.Duration{10 * time.Millisecond},
			want: 0,
		},
		"two samples": {
			rtts: []time.Duration{10 * time.Millisecond, 15 * time.Millisecond},
			want: 5 * time.Millisecond,
		},
		"five samples": {
			rtts: []time.Duration{
				10 * time.Millisecond,
				12 * time.Millisecond,
				15 * time.Millisecond,
				18 * time.Millisecond,
				20 * time.Millisecond,
			},
			want: 2500 * time.Microsecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, calcMeanJitter(tc.rtts))
		})
	}
}

func TestStateStore_Update(t *testing.T) {
	state := newStateStore()
	cfg := AnalysisConfig{
		JitterEWMASamples: 16,
		JitterSMAWindow:   3,
	}

	ewma, sma := state.update("host", 2500*time.Microsecond, cfg)
	assert.Equal(t, time.Duration(156250), ewma)
	assert.Equal(t, 2500*time.Microsecond, sma)

	ewma, sma = state.update("host", 2500*time.Microsecond, cfg)
	assert.Equal(t, time.Duration(302734), ewma)
	assert.Equal(t, 2500*time.Microsecond, sma)

	_, sma = state.update("host", 4000*time.Microsecond, cfg)
	assert.Equal(t, time.Duration(3000000), sma)
}

func TestRTTSummaryVariance(t *testing.T) {
	rtt := RTTSummary{
		Valid:  true,
		StdDev: 5 * time.Millisecond,
	}

	assert.Equal(t, int64(25000000), rtt.VarianceMicrosecondsSquared())
	assert.InDelta(t, 25.0, rtt.VarianceMillisecondsSquared(), 1e-9)
}
