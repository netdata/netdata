// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoreReaderForEachMatch_VisibilityAndPredicate(t *testing.T) {
	tests := map[string]struct {
		raw          bool
		target       string
		wantValues   []SampleValue
		wantObserved int
	}{
		"filtered read returns only latest successful series": {
			raw:          false,
			target:       "a",
			wantValues:   []SampleValue{30},
			wantObserved: 1,
		},
		"raw read includes stale committed series": {
			raw:          true,
			target:       "b",
			wantValues:   []SampleValue{20},
			wantObserved: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := NewCollectorStore()
			cc := cycleController(t, s)
			sm := s.Write().SnapshotMeter("svc")
			g := sm.Gauge("load")
			la := sm.LabelSet(Label{Key: "instance", Value: "a"})
			lb := sm.LabelSet(Label{Key: "instance", Value: "b"})

			cc.BeginCycle()
			g.Observe(10, la)
			g.Observe(20, lb)
			cc.CommitCycleSuccess()

			// Next successful cycle sees only instance=a.
			cc.BeginCycle()
			g.Observe(30, la)
			cc.CommitCycleSuccess()

			reader := s.Read()
			if tc.raw {
				reader = s.Read(ReadRaw())
			}

			var observed int
			var values []SampleValue
			reader.ForEachMatch("svc.load",
				func(labels LabelView) bool {
					observed++
					instance, ok := labels.Get("instance")
					return ok && instance == tc.target
				},
				func(_ LabelView, v SampleValue) {
					values = append(values, v)
				},
			)

			assert.Equal(t, tc.wantObserved, observed)
			assert.Equal(t, tc.wantValues, values)
		})
	}
}
