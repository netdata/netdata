// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	checkedSumResult    int
	checkedChargeResult int64
	checkedSumOK        bool
)

func TestCheckedSum(t *testing.T) {
	tests := map[string]struct {
		maximum int
		values  []int
		want    int
		ok      bool
	}{
		"empty":    {maximum: math.MaxInt, ok: true},
		"sum":      {maximum: math.MaxInt, values: []int{1, 2, 3}, want: 6, ok: true},
		"at limit": {maximum: 6, values: []int{1, 2, 3}, want: 6, ok: true},
		"overflow": {maximum: 5, values: []int{1, 2, 3}},
		"negative": {maximum: math.MaxInt, values: []int{1, -1}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := checkedSum(test.maximum, test.values...)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.ok, ok)
		})
	}
}

func TestCheckedSumWrappersPreserveErrors(t *testing.T) {
	tests := map[string]struct {
		check func() error
		want  string
	}{
		"result size": {
			check: func() error {
				_, err := checkedResultSize(math.MaxInt, 1)
				return err
			},
			want: "jobmgr lifecycle: Function result exceeds bound: result size overflow",
		},
		"plan charge": {
			check: func() error {
				_, err := checkedCharge(math.MaxInt64, 1)
				return err
			},
			want: "jobmgr lifecycle: Function result exceeds bound: plan charge overflow",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.EqualError(t, test.check(), test.want)
		})
	}
}

func TestCheckedSumDoesNotAllocate(t *testing.T) {
	allocations := testing.AllocsPerRun(1_000, func() {
		checkedSumResult, checkedSumOK = checkedSum(math.MaxInt, 1, 2, 3, 4)
		var err error
		checkedSumResult, err = checkedResultSize(1, 2, 3, 4)
		if err != nil {
			panic(err)
		}
		checkedChargeResult, err = checkedCharge(1, 2, 3, 4)
		if err != nil {
			panic(err)
		}
	})
	require.Zero(t, allocations)
	assert.Equal(t, 10, checkedSumResult)
	assert.EqualValues(t, 10, checkedChargeResult)
	assert.True(t, checkedSumOK)
}
