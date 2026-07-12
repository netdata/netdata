// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stateTestQuery(key string, nilAsZero bool) plannedQuery {
	return plannedQuery{
		key: testStructuralID(key), nilAsZero: nilAsZero,
		policy: queryPolicy{period: 5 * time.Minute, lookback: 15 * time.Minute, publicationDelay: 0},
	}
}

func applyStateOutcomes(store *observationStore, due []plannedQuery, outcomes map[structuralID]queryOutcome) {
	completedAt := time.Unix(1, 0)
	explicit := make(map[structuralID]queryOutcome, len(due))
	for _, query := range due {
		outcome, ok := outcomes[query.key]
		if !ok {
			start, end := queryWindow(completedAt, query.policy)
			outcome = queryOutcome{kind: queryOutcomeTransient, windowStart: start, windowEnd: end}
		}
		if outcome.completedAt.IsZero() {
			outcome.completedAt = completedAt
		}
		explicit[query.key] = outcome
	}
	if err := store.applyOutcomes(due, explicit, time.Minute); err != nil {
		panic(err)
	}
}

func TestObservationStore_PerQueryAlignedCompletion(t *testing.T) {
	store := newObservationStore(nil)
	first := stateTestQuery("first", false)
	second := stateTestQuery("second", false)
	now := time.Unix(1_000_000_000, 0)
	_, end := queryWindow(now, first.policy)
	store.queries[first.key] = queryState{lastCompletedEnd: end}

	due := store.dueQueries([]plannedQuery{first, second}, now)
	require.Len(t, due, 1)
	assert.Equal(t, second.key, due[0].key, "a new sibling does not invalidate completed queries")

	later := now.Add(5 * time.Minute)
	due = store.dueQueries([]plannedQuery{first}, later)
	require.Len(t, due, 1)
	start, laterEnd := queryWindow(later, first.policy)
	applyStateOutcomes(store, due, map[structuralID]queryOutcome{first.key: {
		kind: queryOutcomeComplete, windowStart: start, windowEnd: laterEnd,
	}})
	assert.Empty(t, store.dueQueries([]plannedQuery{first}, later))
}

func TestObservationStore_RollingLookbackAndCorrection(t *testing.T) {
	store := newObservationStore(nil)
	query := stateTestQuery("gauge", false)
	base := time.Unix(0, 0)

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base, windowEnd: base.Add(15 * time.Minute),
		datapointAt: base.Add(10 * time.Minute), value: 1, hasDatapoint: true,
	}})
	assert.Equal(t, float64(1), store.queries[query.key].observation)

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base.Add(5 * time.Minute), windowEnd: base.Add(20 * time.Minute),
		datapointAt: base.Add(10 * time.Minute), value: 2, hasDatapoint: true,
	}})
	assert.Equal(t, float64(2), store.queries[query.key].observation, "same-timestamp corrections replace the cached value")

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base.Add(10 * time.Minute), windowEnd: base.Add(25 * time.Minute),
	}})
	assert.True(t, store.queries[query.key].hasObservation, "the boundary datapoint remains eligible")

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base.Add(15 * time.Minute), windowEnd: base.Add(30 * time.Minute),
	}})
	assert.False(t, store.queries[query.key].hasObservation, "a successful later window expires the old datapoint")
}

func TestObservationStore_TransientForbiddenAndZeroTransitions(t *testing.T) {
	store := newObservationStore(nil)
	query := stateTestQuery("rate", true)
	base := time.Unix(0, 0)
	store.queries[query.key] = queryState{hasObservation: true, observation: 5, observationAt: base, lastCompletedEnd: base.Add(5 * time.Minute)}

	applyStateOutcomes(store, []plannedQuery{query}, nil)
	assert.Equal(t, float64(5), store.queries[query.key].observation, "transient outcomes preserve cached state beyond lookback")

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base.Add(time.Hour), windowEnd: base.Add(75 * time.Minute),
	}})
	state := store.queries[query.key]
	assert.False(t, state.hasObservation)
	assert.True(t, state.emitZero, "complete no-data becomes zero presentation for nil_as_zero")

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeForbidden, windowStart: base.Add(65 * time.Minute), windowEnd: base.Add(80 * time.Minute),
	}})
	state = store.queries[query.key]
	assert.False(t, state.hasObservation)
	assert.False(t, state.emitZero)
	assert.Equal(t, base.Add(80*time.Minute), state.lastCompletedEnd, "Forbidden advances only the attempted window")
	assert.Empty(t, store.dueQueries([]plannedQuery{query}, base.Add(80*time.Minute)))
	assert.Len(t, store.dueQueries([]plannedQuery{query}, base.Add(85*time.Minute)), 1, "Forbidden retries at the next eligible window")
}

func TestObservationStore_LateDatapointReplacesSyntheticZero(t *testing.T) {
	store := newObservationStore(nil)
	query := stateTestQuery("rate", true)
	base := time.Unix(0, 0)

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base, windowEnd: base.Add(15 * time.Minute),
	}})
	state := store.queries[query.key]
	require.False(t, state.hasObservation)
	require.True(t, state.emitZero)

	applyStateOutcomes(store, []plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: base.Add(5 * time.Minute), windowEnd: base.Add(20 * time.Minute),
		datapointAt: base.Add(5 * time.Minute), value: 7, hasDatapoint: true,
	}})
	state = store.queries[query.key]
	assert.True(t, state.hasObservation)
	assert.False(t, state.emitZero)
	assert.Equal(t, float64(7), state.observation, "a presentation fallback must never outrank a real eligible datapoint")
	assert.Equal(t, base.Add(5*time.Minute), state.observationAt)
}

func TestObservationStore_TransientRetryBackoffAndTerminalReset(t *testing.T) {
	store := newObservationStore(nil)
	query := stateTestQuery("gauge", false)
	base := time.Unix(1_000_000_000, 0)

	start, windowEnd := queryWindow(base, query.policy)
	require.NoError(t, store.applyOutcomes([]plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeTransient, windowStart: start, windowEnd: windowEnd, completedAt: base,
	}}, time.Minute))
	state := store.queries[query.key]
	assert.Equal(t, windowEnd, state.retryWindowEnd)
	assert.Equal(t, base.Add(time.Minute), state.nextRetryAt)
	assert.Empty(t, store.dueQueries([]plannedQuery{query}, base.Add(time.Minute-time.Second)))
	assert.Len(t, store.dueQueries([]plannedQuery{query}, base.Add(time.Minute)), 1)

	require.NoError(t, store.applyOutcomes([]plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeTransient, windowStart: start, windowEnd: windowEnd, completedAt: base.Add(time.Minute),
	}}, time.Minute))
	state = store.queries[query.key]
	assert.Equal(t, base.Add(3*time.Minute), state.nextRetryAt, "the second delay doubles within the same window")

	start, end := queryWindow(base, query.policy)
	require.NoError(t, store.applyOutcomes([]plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
		kind: queryOutcomeComplete, windowStart: start, windowEnd: end, completedAt: base.Add(3 * time.Minute),
	}}, time.Minute))
	state = store.queries[query.key]
	assert.Zero(t, state.transientCount)
	assert.True(t, state.retryWindowEnd.IsZero())
	assert.True(t, state.nextRetryAt.IsZero())
}

func TestObservationStore_RejectsIncompleteExecution(t *testing.T) {
	store := newObservationStore(nil)
	first := stateTestQuery("first", false)
	second := stateTestQuery("second", false)
	completedAt := time.Unix(1_000_000_000, 0)
	start, end := queryWindow(completedAt, first.policy)

	err := store.applyOutcomes([]plannedQuery{first, second}, map[structuralID]queryOutcome{first.key: {
		kind: queryOutcomeComplete, windowStart: start, windowEnd: end, completedAt: completedAt,
	}}, time.Minute)
	require.ErrorContains(t, err, "1 outcomes for 2 due queries")
	assert.Empty(t, store.queries, "validation must finish before any query state is mutated")

	err = store.applyOutcomes([]plannedQuery{first}, map[structuralID]queryOutcome{first.key: {
		kind: queryOutcomeComplete, windowStart: start, windowEnd: end,
	}}, time.Minute)
	require.ErrorContains(t, err, "omitted completion time")
	assert.Empty(t, store.queries, "invalid completion provenance must not mutate state")
}

func TestObservationStore_InvariantErrorsDoNotExposeQueryIdentity(t *testing.T) {
	const sensitiveIdentity = "SENSITIVE_ACCOUNT/SENSITIVE_RESOURCE"
	query := stateTestQuery(sensitiveIdentity, false)
	store := newObservationStore(nil)
	completedAt := time.Unix(1_000_000_000, 0)
	start, end := queryWindow(completedAt, query.policy)

	t.Run("missing outcome", func(t *testing.T) {
		err := store.applyOutcomes([]plannedQuery{query}, map[structuralID]queryOutcome{testStructuralID("other"): {
			kind: queryOutcomeComplete, windowStart: start, windowEnd: end, completedAt: completedAt,
		}}, time.Minute)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sensitiveIdentity)
	})

	t.Run("missing completion time", func(t *testing.T) {
		err := store.applyOutcomes([]plannedQuery{query}, map[structuralID]queryOutcome{query.key: {
			kind: queryOutcomeComplete, windowStart: start, windowEnd: end,
		}}, time.Minute)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sensitiveIdentity)
	})
}

func TestTransientRetryDelay(t *testing.T) {
	tests := map[string]struct {
		base, period time.Duration
		count        int
		want         time.Duration
	}{
		"first":                  {base: time.Minute, period: 10 * time.Minute, count: 1, want: time.Minute},
		"doubles":                {base: time.Minute, period: 10 * time.Minute, count: 3, want: 4 * time.Minute},
		"caps at period":         {base: time.Minute, period: 5 * time.Minute, count: 4, want: 5 * time.Minute},
		"base above period caps": {base: 10 * time.Minute, period: 5 * time.Minute, count: 1, want: 5 * time.Minute},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, transientRetryDelay(tc.base, tc.period, tc.count))
		})
	}
}
