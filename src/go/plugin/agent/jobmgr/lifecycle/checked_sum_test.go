// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	checkedSumResult int
	checkedSumOK     bool
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

func TestCheckedSumDoesNotAllocate(t *testing.T) {
	allocations := testing.AllocsPerRun(1_000, func() {
		checkedSumResult, checkedSumOK = checkedSum(math.MaxInt, 1, 2, 3, 4)
	})
	require.Zero(t, allocations)
	assert.Equal(t, 10, checkedSumResult)
	assert.True(t, checkedSumOK)
}
