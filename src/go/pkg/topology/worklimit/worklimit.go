// SPDX-License-Identifier: GPL-3.0-or-later

// Package worklimit defines the optional work callback shared by topology
// build, shaping, enrichment, and rendering stages.
package worklimit

import (
	"errors"
	"math"
	"math/bits"
)

var ErrOverflow = errors.New("topology work estimate overflow")

// Limiter rejects work before a bounded stage executes. A nil limiter leaves
// trusted production callers on the unbounded path.
type Limiter func(units uint64) error

func (l Limiter) Charge(units uint64) error {
	if l == nil || units == 0 {
		return nil
	}
	return l(units)
}

func (l Limiter) ChargeProduct(factors ...uint64) error {
	if l == nil || len(factors) == 0 {
		return nil
	}
	product, err := Product(factors...)
	if err != nil {
		return err
	}
	return l.Charge(product)
}

// ChargeSort charges an n*ceil(log2(n)) comparison envelope.
func (l Limiter) ChargeSort(items uint64) error {
	if l == nil {
		return nil
	}
	units, err := SortEnvelope(items)
	if err != nil {
		return err
	}
	return l.Charge(units)
}

// SortEnvelope returns an n*ceil(log2(n)) comparison envelope.
func SortEnvelope(items uint64) (uint64, error) {
	if items < 2 {
		return 0, nil
	}
	return Product(items, uint64(bits.Len64(items-1)))
}

func Sum(values ...uint64) (uint64, error) {
	var sum uint64
	for _, value := range values {
		if sum > math.MaxUint64-value {
			return 0, ErrOverflow
		}
		sum += value
	}
	return sum, nil
}

func Product(factors ...uint64) (uint64, error) {
	if len(factors) == 0 {
		return 0, nil
	}
	product := uint64(1)
	for _, factor := range factors {
		if factor == 0 {
			return 0, nil
		}
		if product > math.MaxUint64/factor {
			return 0, ErrOverflow
		}
		product *= factor
	}
	return product, nil
}
