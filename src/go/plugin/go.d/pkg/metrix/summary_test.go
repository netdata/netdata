// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSummary(t *testing.T) {
	s := NewSummary().(*summary)
	assert.EqualValues(t, 0, s.count)
	assert.Equal(t, 0.0, s.sum)
	s.Observe(3.14)
	assert.Equal(t, 3.14, s.min)
	assert.Equal(t, 3.14, s.max)
}

func TestSummary_WriteTo(t *testing.T) {
	s := NewSummary()

	m1 := map[string]int64{}
	s.WriteTo(m1, "pi", 100, 1)
	assert.Len(t, m1, 2)
	assert.Contains(t, m1, "pi_count")
	assert.Contains(t, m1, "pi_sum")
	assert.EqualValues(t, 0, m1["pi_count"])
	assert.EqualValues(t, 0, m1["pi_sum"])

	s.Observe(3.14)
	s.Observe(2.71)
	s.Observe(-10)

	m2 := map[string]int64{}
	s.WriteTo(m1, "pi", 100, 1)
	s.WriteTo(m2, "pi", 100, 1)
	assert.Equal(t, m1, m2)
	assert.Len(t, m1, 5)
	assert.EqualValues(t, 3, m1["pi_count"])
	assert.EqualValues(t, -415, m1["pi_sum"])
	assert.EqualValues(t, -1000, m1["pi_min"])
	assert.EqualValues(t, 314, m1["pi_max"])
	assert.EqualValues(t, -138, m1["pi_avg"])

	s.Reset()
	s.WriteTo(m1, "pi", 100, 1)
	assert.Len(t, m1, 2)
	assert.Contains(t, m1, "pi_count")
	assert.Contains(t, m1, "pi_sum")
	assert.EqualValues(t, 0, m1["pi_count"])
	assert.EqualValues(t, 0, m1["pi_sum"])
}

func TestSummary_Reset(t *testing.T) {
	s := NewSummary().(*summary)
	s.Observe(1)
	s.Reset()
	assert.EqualValues(t, 0, s.count)
}

func BenchmarkSummary_Observe(b *testing.B) {
	s := NewSummary()
	for i := 0; i < b.N; i++ {
		s.Observe(2.5)
	}
}

func BenchmarkSummary_WriteTo(b *testing.B) {
	s := NewSummary()
	s.Observe(2.5)
	s.Observe(3.5)
	s.Observe(4.5)
	m := map[string]int64{}
	for i := 0; i < b.N; i++ {
		s.WriteTo(m, "pi", 100, 1)
	}
}
