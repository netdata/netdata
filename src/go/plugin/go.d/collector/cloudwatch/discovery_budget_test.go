// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiscoveryBudget_FirstOperationReservations(t *testing.T) {
	_, err := newDiscoveryBudget(maxListMetricsOperationsPerRefresh, func(error) {})
	require.NoError(t, err)

	_, err = newDiscoveryBudget(maxListMetricsOperationsPerRefresh+1, func(error) {})
	assert.ErrorContains(t, err, "requires 101 first ListMetrics operations")
}

func TestDiscoveryBudget_ListMetricsOperations(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	budget, err := newDiscoveryBudget(defaultMaxDiscoveryGroups, cancel)
	require.NoError(t, err)

	for range maxListMetricsOperationsPerRefresh {
		require.NoError(t, budget.reserveListMetricsOperation())
	}
	err = budget.reserveListMetricsOperation()
	assert.ErrorContains(t, err, "more than 100 ListMetrics SDK operations")
	assert.Equal(t, err, context.Cause(ctx))
}

func TestDiscoveryBudget_WorkReservations(t *testing.T) {
	tests := map[string]struct {
		limit   int
		reserve func(*discoveryBudget, int) error
		message string
	}{
		"scanned metrics": {
			limit: maxScannedMetricsPerRefresh,
			reserve: func(b *discoveryBudget, n int) error {
				return b.reserveScannedMetrics(n)
			},
			message: "scans more than 50000 metrics",
		},
		"matcher evaluations": {
			limit: maxDiscoveryMatcherEvaluationsPerRefresh,
			reserve: func(b *discoveryBudget, n int) error {
				return b.reserveMatcherEvaluations(n)
			},
			message: "more than 1000000 residual profile matches",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			budget := testDiscoveryBudget(1)
			require.NoError(t, test.reserve(budget, test.limit))
			assert.ErrorContains(t, test.reserve(budget, 1), test.message)
		})
	}
}

func TestDiscoveryBudget_CandidateReservationIsTransactional(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		budget := testDiscoveryBudget(1)
		for range maxCandidateInstancesPerRefresh {
			require.NoError(t, budget.reserveCandidate(1))
		}
		assert.ErrorContains(t, budget.reserveCandidate(1), "more than 20000 candidate instances")
		assert.Equal(t, maxCandidateInstancesPerRefresh, budget.candidateInstances)
		assert.Equal(t, maxCandidateInstancesPerRefresh, budget.retainedCandidateBytes)
	})

	t.Run("weighted bytes", func(t *testing.T) {
		budget := testDiscoveryBudget(1)
		require.NoError(t, budget.reserveCandidate(maxRetainedCandidateBytesPerRefresh))
		assert.ErrorContains(t, budget.reserveCandidate(1), "more than 64 MiB")
		assert.Equal(t, 1, budget.candidateInstances)
		assert.Equal(t, maxRetainedCandidateBytesPerRefresh, budget.retainedCandidateBytes)
	})
}

func TestDiscoveryBudget_ConcurrentCandidateReservationsDoNotOvershoot(t *testing.T) {
	budget := testDiscoveryBudget(1)
	const reservation = 1 << 20
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if budget.reserveCandidate(reservation) == nil {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(maxRetainedCandidateBytesPerRefresh/reservation), admitted.Load())
	assert.Equal(t, maxRetainedCandidateBytesPerRefresh, budget.retainedCandidateBytes)
	assert.Equal(t, int(admitted.Load()), budget.candidateInstances)
}

func TestRetainedCandidateBytes(t *testing.T) {
	assert.Equal(t,
		len("one\x1ftwo")+retainedCandidateBaseBytes+2*retainedCandidatePerDimensionBytes,
		retainedCandidateBytes("one\x1ftwo", 2),
	)
}
