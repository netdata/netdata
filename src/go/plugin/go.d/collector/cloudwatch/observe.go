// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type queryState struct {
	lastCompletedEnd time.Time
	retryWindowEnd   time.Time
	nextRetryAt      time.Time
	transientCount   int
	observationAt    time.Time
	observation      float64
	hasObservation   bool
	emitZero         bool
}

type queryOutcomeKind uint8

const (
	queryOutcomeTransient queryOutcomeKind = iota
	queryOutcomeComplete
	queryOutcomeForbidden
)

type queryOutcome struct {
	kind         queryOutcomeKind
	windowStart  time.Time
	windowEnd    time.Time
	completedAt  time.Time
	datapointAt  time.Time
	value        float64
	hasDatapoint bool
}

// observationStore owns per-query completion and retained numeric state. Query
// identity includes target, structural AWS query, series identity, and policy,
// so only an exactly equivalent rebuilt query can retain state.
type observationStore struct {
	store   metrix.CollectorStore
	queries map[string]queryState
}

func newObservationStore(store metrix.CollectorStore) *observationStore {
	return &observationStore{
		store:   store,
		queries: make(map[string]queryState),
	}
}

// reset clears the retention cache and schedule so a framework re-Init starts
// clean (the metrix store itself persists and is reused).
func (o *observationStore) reset() {
	o.queries = make(map[string]queryState)
}

func (o *observationStore) dueQueries(plan []plannedQuery, now time.Time) []plannedQuery {
	due := make([]plannedQuery, 0, len(plan))
	for _, query := range plan {
		_, end := queryWindow(now, query.policy)
		state, ok := o.queries[query.key]
		if queryIsDue(state, ok, end, now) {
			due = append(due, query)
		}
	}
	return due
}

func queryIsDue(state queryState, exists bool, windowEnd, now time.Time) bool {
	if !exists {
		return true
	}
	if !state.lastCompletedEnd.Before(windowEnd) || state.retryWindowEnd.After(windowEnd) {
		return false
	}
	return state.retryWindowEnd.Before(windowEnd) || !now.Before(state.nextRetryAt)
}

func (o *observationStore) applyOutcomes(due []plannedQuery, outcomes map[string]queryOutcome, retryBase time.Duration) error {
	if len(outcomes) != len(due) {
		return fmt.Errorf("CloudWatch query execution returned %d outcomes for %d due queries", len(outcomes), len(due))
	}
	for _, query := range due {
		outcome, ok := outcomes[query.key]
		if !ok {
			return fmt.Errorf("CloudWatch query execution omitted outcome for planned query %q", query.key)
		}
		if outcome.completedAt.IsZero() {
			return fmt.Errorf("CloudWatch query execution omitted completion time for planned query %q", query.key)
		}
	}

	for _, query := range due {
		outcome := outcomes[query.key]
		if outcome.kind == queryOutcomeTransient {
			o.markTransient(query, outcome, retryBase)
			continue
		}

		state := o.queries[query.key]
		state.retryWindowEnd = time.Time{}
		state.nextRetryAt = time.Time{}
		state.transientCount = 0
		state.lastCompletedEnd = outcome.windowEnd
		if outcome.kind == queryOutcomeForbidden {
			state.hasObservation = false
			state.emitZero = false
			o.queries[query.key] = state
			continue
		}

		if outcome.hasDatapoint && !outcome.datapointAt.Before(outcome.windowStart) && !outcome.datapointAt.Add(query.policy.period).After(outcome.windowEnd) &&
			(!state.hasObservation || !outcome.datapointAt.Before(state.observationAt)) {
			state.observation = outcome.value
			state.observationAt = outcome.datapointAt
			state.hasObservation = true
		}
		if state.hasObservation && state.observationAt.Before(outcome.windowStart) {
			state.hasObservation = false
		}
		state.emitZero = !state.hasObservation && query.nilAsZero
		o.queries[query.key] = state
	}
	return nil
}

func (o *observationStore) markTransient(query plannedQuery, outcome queryOutcome, retryBase time.Duration) {
	state := o.queries[query.key]
	if !state.retryWindowEnd.Equal(outcome.windowEnd) {
		state.transientCount = 0
	}
	state.transientCount++
	state.retryWindowEnd = outcome.windowEnd
	state.nextRetryAt = outcome.completedAt.Add(transientRetryDelay(retryBase, query.policy.period, state.transientCount))
	o.queries[query.key] = state
}

func transientRetryDelay(base, period time.Duration, count int) time.Duration {
	if base <= 0 {
		base = time.Minute
	}
	if period <= 0 {
		return base
	}
	if base >= period {
		return period
	}
	delay := base
	for range max(0, count-1) {
		if delay >= period/2 {
			return period
		}
		delay *= 2
	}
	return delay
}

func (o *observationStore) reconcilePlan(current []plannedQuery) {
	valid := make(map[string]struct{}, len(current))
	for _, query := range current {
		valid[query.key] = struct{}{}
	}
	for key := range o.queries {
		if _, ok := valid[key]; !ok {
			delete(o.queries, key)
		}
	}
}
