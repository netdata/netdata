// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"fmt"
	"testing"
)

func TestSessionSampledIsStablePerSessionAndProportional(t *testing.T) {
	if !SessionSampled("any", 1) || SessionSampled("any", 0) {
		t.Fatal("rate 1 keeps every session, rate 0 none")
	}
	first := SessionSampled("s-42", 0.3)
	second := SessionSampled("s-42", 0.3)
	if first != second {
		t.Fatal("the decision must be the same every time for one session")
	}
	kept := 0
	for i := 0; i < 20000; i++ {
		if SessionSampled(fmt.Sprintf("session-%d", i), 0.1) {
			kept++
		}
	}
	if kept < 1700 || kept > 2300 {
		t.Fatalf("10%% of 20000 sessions kept = %d", kept)
	}
}

func TestHasPoorVital(t *testing.T) {
	for _, tc := range []struct {
		vitals []Vital
		want   bool
	}{
		{nil, false},
		{[]Vital{{Name: LCP, Value: 2400}, {Name: CLS, Value: 0.05}}, false},
		{[]Vital{{Name: LCP, Value: 4001}}, true},
		{[]Vital{{Name: INP, Value: 501}}, true},
		{[]Vital{{Name: CLS, Value: 0.26}}, true},
		{[]Vital{{Name: FCP, Value: 3001}}, true},
		{[]Vital{{Name: TTFB, Value: 1801}}, true},
		{[]Vital{{Name: LCP, Value: 4000}}, false},
	} {
		if got := (&Beacon{
			Vitals: tc.vitals,
		}).HasPoorVital(); got != tc.want {
			t.Errorf("%+v: got %v", tc.vitals, got)
		}
	}
}
