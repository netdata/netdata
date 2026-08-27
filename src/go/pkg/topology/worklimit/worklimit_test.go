// SPDX-License-Identifier: GPL-3.0-or-later

package worklimit

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLimiterChargesExactProductsAndSortEnvelope(t *testing.T) {
	var charged []uint64
	limiter := Limiter(func(units uint64) error {
		charged = append(charged, units)
		return nil
	})

	require.NoError(t, limiter.ChargeProduct(3, 7))
	require.NoError(t, limiter.ChargeSort(9))
	require.Equal(t, []uint64{21, 36}, charged)
}

func TestLimiterRejectsOverflowBeforeCallback(t *testing.T) {
	called := false
	limiter := Limiter(func(uint64) error {
		called = true
		return nil
	})

	require.ErrorIs(t, limiter.ChargeProduct(math.MaxUint64, 2), ErrOverflow)
	require.ErrorIs(t, limiter.ChargeSort(math.MaxUint64), ErrOverflow)
	require.False(t, called)
	require.NoError(t, Limiter(nil).ChargeSort(math.MaxUint64), "the trusted nil path does no accounting")
}

func TestCheckedArithmetic(t *testing.T) {
	sum, err := Sum(1, 2, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(6), sum)
	_, err = Sum(math.MaxUint64, 1)
	require.ErrorIs(t, err, ErrOverflow)

	product, err := Product(3, 7)
	require.NoError(t, err)
	require.Equal(t, uint64(21), product)
	require.Zero(t, mustProduct(t, 3, 0, math.MaxUint64))
}

func mustProduct(t *testing.T, factors ...uint64) uint64 {
	t.Helper()
	value, err := Product(factors...)
	require.NoError(t, err)
	return value
}
