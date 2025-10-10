// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrueSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr       trueSelector
		lbs      labels.Labels
		expected bool
	}{
		"not empty labels": {
			lbs:      labels.Labels{{Name: labels.MetricName, Value: "name"}},
			expected: true,
		},
		"empty labels": {
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expected {
				assert.True(t, test.sr.Matches(test.lbs))
			} else {
				assert.False(t, test.sr.Matches(test.lbs))
			}
		})
	}
}

func TestFalseSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr       falseSelector
		lbs      labels.Labels
		expected bool
	}{
		"not empty labels": {
			lbs:      labels.Labels{{Name: labels.MetricName, Value: "name"}},
			expected: false,
		},
		"empty labels": {
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expected {
				assert.True(t, test.sr.Matches(test.lbs))
			} else {
				assert.False(t, test.sr.Matches(test.lbs))
			}
		})
	}
}

func TestNegSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr       negSelector
		lbs      labels.Labels
		expected bool
	}{
		"true matcher": {
			sr:       negSelector{trueSelector{}},
			lbs:      labels.Labels{{Name: labels.MetricName, Value: "name"}},
			expected: false,
		},
		"false matcher": {
			sr:       negSelector{falseSelector{}},
			lbs:      labels.Labels{{Name: labels.MetricName, Value: "name"}},
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expected {
				assert.True(t, test.sr.Matches(test.lbs))
			} else {
				assert.False(t, test.sr.Matches(test.lbs))
			}
		})
	}
}

func TestAndSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr       andSelector
		lbs      labels.Labels
		expected bool
	}{
		"true, true": {
			sr:       andSelector{lhs: trueSelector{}, rhs: trueSelector{}},
			expected: true,
		},
		"true, false": {
			sr:       andSelector{lhs: trueSelector{}, rhs: falseSelector{}},
			expected: false,
		},
		"false, true": {
			sr:       andSelector{lhs: trueSelector{}, rhs: falseSelector{}},
			expected: false,
		},
		"false, false": {
			sr:       andSelector{lhs: falseSelector{}, rhs: falseSelector{}},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.sr.Matches(test.lbs))
		})
	}
}

func TestOrSelector_Matches(t *testing.T) {
	tests := map[string]struct {
		sr       orSelector
		lbs      labels.Labels
		expected bool
	}{
		"true, true": {
			sr:       orSelector{lhs: trueSelector{}, rhs: trueSelector{}},
			expected: true,
		},
		"true, false": {
			sr:       orSelector{lhs: trueSelector{}, rhs: falseSelector{}},
			expected: true,
		},
		"false, true": {
			sr:       orSelector{lhs: trueSelector{}, rhs: falseSelector{}},
			expected: true,
		},
		"false, false": {
			sr:       orSelector{lhs: falseSelector{}, rhs: falseSelector{}},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.sr.Matches(test.lbs))
		})
	}
}

func Test_And(t *testing.T) {
	tests := map[string]struct {
		srs      []Selector
		expected Selector
	}{
		"2 selectors": {
			srs: []Selector{trueSelector{}, trueSelector{}},
			expected: andSelector{
				lhs: trueSelector{},
				rhs: trueSelector{},
			},
		},
		"4 selectors": {
			srs: []Selector{trueSelector{}, trueSelector{}, trueSelector{}, trueSelector{}},
			expected: andSelector{
				lhs: andSelector{
					lhs: andSelector{
						lhs: trueSelector{},
						rhs: trueSelector{},
					},
					rhs: trueSelector{},
				},
				rhs: trueSelector{}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.GreaterOrEqual(t, len(test.srs), 2)

			s := And(test.srs[0], test.srs[1], test.srs[2:]...)
			assert.Equal(t, test.expected, s)
		})
	}
}

func Test_Or(t *testing.T) {
	tests := map[string]struct {
		srs      []Selector
		expected Selector
	}{
		"2 selectors": {
			srs: []Selector{trueSelector{}, trueSelector{}},
			expected: orSelector{
				lhs: trueSelector{},
				rhs: trueSelector{},
			},
		},
		"4 selectors": {
			srs: []Selector{trueSelector{}, trueSelector{}, trueSelector{}, trueSelector{}},
			expected: orSelector{
				lhs: orSelector{
					lhs: orSelector{
						lhs: trueSelector{},
						rhs: trueSelector{},
					},
					rhs: trueSelector{},
				},
				rhs: trueSelector{}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.GreaterOrEqual(t, len(test.srs), 2)

			s := Or(test.srs[0], test.srs[1], test.srs[2:]...)
			assert.Equal(t, test.expected, s)
		})
	}
}
