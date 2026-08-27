// SPDX-License-Identifier: GPL-3.0-or-later

package worklimit

import (
	"errors"
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

func TestOperationOwnedSortsRejectBeforeExecution(t *testing.T) {
	limitErr := errors.New("limit")
	compared := false
	values := []int{2, 1}
	err := SortFunc(func(uint64) error { return limitErr }, values, func(a, b int) int {
		compared = true
		return a - b
	})
	require.ErrorIs(t, err, limitErr)
	require.False(t, compared)
	require.Equal(t, []int{2, 1}, values)

	strings := []string{"b", "a"}
	err = SortStrings(func(uint64) error { return limitErr }, strings)
	require.ErrorIs(t, err, limitErr)
	require.Equal(t, []string{"b", "a"}, strings)
}

func TestOperationOwnedSortsPreserveNilLimiterBehavior(t *testing.T) {
	values := []string{"b", "a", "c"}
	require.NoError(t, SortStrings(nil, values))
	require.Equal(t, []string{"a", "b", "c"}, values)

	keys, err := SortedStringKeys(nil, map[string]int{"b": 2, "a": 1})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, keys)
}

func TestPreparedStringKeySortRejectsBeforePreparationAndPreparesOnce(t *testing.T) {
	limitErr := errors.New("limit")
	values := []string{"b", "a", "c"}
	prepared := 0
	err := SortStableByPreparedStringKey(func(uint64) error { return limitErr }, values, func(value string) (string, error) {
		prepared++
		return value, nil
	})
	require.ErrorIs(t, err, limitErr)
	require.Zero(t, prepared)
	require.Equal(t, []string{"b", "a", "c"}, values)

	var charged uint64
	err = SortStableByPreparedStringKey(func(units uint64) error {
		charged += units
		return nil
	}, values, func(value string) (string, error) {
		prepared++
		return value, nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, prepared)
	require.Positive(t, charged)
	require.Equal(t, []string{"a", "b", "c"}, values)
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
