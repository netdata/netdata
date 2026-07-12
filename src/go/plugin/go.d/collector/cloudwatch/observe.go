// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type queryState struct {
	lastCompletedEnd time.Time
	valueAt          time.Time
	value            float64
	hasValue         bool
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
		if !ok || state.lastCompletedEnd.Before(end) {
			due = append(due, query)
		}
	}
	return due
}

func (o *observationStore) applyOutcomes(due []plannedQuery, outcomes map[string]queryOutcome) {
	for _, query := range due {
		outcome, ok := outcomes[query.key]
		if !ok || outcome.kind == queryOutcomeTransient {
			continue
		}

		state := o.queries[query.key]
		state.lastCompletedEnd = outcome.windowEnd
		if outcome.kind == queryOutcomeForbidden {
			state.hasValue = false
			o.queries[query.key] = state
			continue
		}

		if outcome.hasDatapoint && !outcome.datapointAt.Before(outcome.windowStart) && !outcome.datapointAt.Add(query.policy.period).After(outcome.windowEnd) &&
			(!state.hasValue || !outcome.datapointAt.Before(state.valueAt)) {
			state.value = outcome.value
			state.valueAt = outcome.datapointAt
			state.hasValue = true
		}
		if state.hasValue && state.valueAt.Before(outcome.windowStart) {
			state.hasValue = false
		}
		if !state.hasValue && query.nilAsZero {
			state.value = 0
			state.valueAt = outcome.windowEnd.Add(-query.policy.period)
			state.hasValue = true
		}
		o.queries[query.key] = state
	}
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
