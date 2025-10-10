// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: write better tests

const (
	testName1 = "logback_events_total"
	testName2 = "jvm_threads_peak"
)

func newTestSeries() Series {
	return Series{
		{
			Value: 10,
			Labels: labels.Labels{
				{Name: "__name__", Value: testName1},
				{Name: "level", Value: "error"},
			},
		},
		{
			Value: 20,
			Labels: labels.Labels{
				{Name: "__name__", Value: testName1},
				{Name: "level", Value: "warn"},
			},
		},
		{
			Value: 5,
			Labels: labels.Labels{
				{Name: "__name__", Value: testName1},
				{Name: "level", Value: "info"},
			},
		},
		{
			Value: 15,
			Labels: labels.Labels{
				{Name: "__name__", Value: testName1},
				{Name: "level", Value: "debug"},
			},
		},
		{
			Value: 26,
			Labels: labels.Labels{
				{Name: "__name__", Value: testName2},
			},
		},
	}
}

func TestSeries_Name(t *testing.T) {
	m := newTestSeries()

	assert.Equal(t, testName1, m[0].Name())
	assert.Equal(t, testName1, m[1].Name())

}

func TestSeries_Add(t *testing.T) {
	m := newTestSeries()

	require.Len(t, m, 5)
	m.Add(SeriesSample{})
	assert.Len(t, m, 6)
}

func TestSeries_FindByName(t *testing.T) {
	m := newTestSeries()
	m.Sort()
	assert.Len(t, Series{}.FindByName(testName1), 0)
	assert.Len(t, m.FindByName(testName1), len(m)-1)
}

func TestSeries_FindByNames(t *testing.T) {
	m := newTestSeries()
	m.Sort()
	assert.Len(t, m.FindByNames(), 0)
	assert.Len(t, m.FindByNames(testName1), len(m)-1)
	assert.Len(t, m.FindByNames(testName1, testName2), len(m))
}

func TestSeries_Len(t *testing.T) {
	m := newTestSeries()

	assert.Equal(t, len(m), m.Len())
}

func TestSeries_Less(t *testing.T) {
	m := newTestSeries()

	assert.False(t, m.Less(0, 1))
	assert.True(t, m.Less(4, 0))
}

func TestSeries_Max(t *testing.T) {
	m := newTestSeries()

	assert.Equal(t, float64(26), m.Max())

}

func TestSeries_Reset(t *testing.T) {
	m := newTestSeries()
	m.Reset()

	assert.Len(t, m, 0)

}

func TestSeries_Sort(t *testing.T) {
	{
		m := newTestSeries()
		m.Sort()
		assert.Equal(t, testName2, m[0].Name())
	}
	{
		m := Series{}
		assert.Equal(t, 0.0, m.Max())
	}
}

func TestSeries_Swap(t *testing.T) {
	m := newTestSeries()

	m0 := m[0]
	m1 := m[1]

	m.Swap(0, 1)

	assert.Equal(t, m0, m[1])
	assert.Equal(t, m1, m[0])
}
