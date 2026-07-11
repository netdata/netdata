// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// observedSeries caches the last value written for a series so that not-due
// series (e.g. daily S3 metrics queried far less often than the collect cycle)
// are re-emitted every cycle and stay continuously visible.
type observedSeries struct {
	seriesName string
	labels     []metrix.Label
	tagLabels  []metrix.Label // non-identity enrichment; re-emitted with the series, not in observedKey
	value      float64
	groupKey   queryGroupKey // (target, region, effective period) — the scheduling unit
}

// observationStore owns the metric write path together with the per-series
// retention cache and the per-(target, region, period) query schedule. Keeping these
// together isolates retention/scheduling from the rest of the collector: it
// re-emits not-due series every cycle so daily metrics stay visible, and prunes
// series whose instance has disappeared from discovery.
type observationStore struct {
	store        metrix.CollectorStore
	lastObserved map[string]observedSeries   // last value per series, for retention re-emit
	nextQueryAt  map[queryGroupKey]time.Time // next due time per (target, region, effective period)
}

func newObservationStore(store metrix.CollectorStore) *observationStore {
	return &observationStore{
		store:        store,
		lastObserved: make(map[string]observedSeries),
		nextQueryAt:  make(map[queryGroupKey]time.Time),
	}
}

// reset clears the retention cache and schedule so a framework re-Init starts
// clean (the metrix store itself persists and is reused).
func (o *observationStore) reset() {
	o.lastObserved = make(map[string]observedSeries)
	o.nextQueryAt = make(map[queryGroupKey]time.Time)
}

// dueGroups returns the set of (target, region, period) groups due for querying at now.
// It does NOT advance the schedule — the caller advances nextQueryAt only for
// groups that actually succeeded, so a transient failure is retried next cycle
// rather than skipped for a full period. Scheduling is per (target, region, period) so a
// failure in one region does not force healthy regions of the same period to
// re-query early. A group not yet scheduled is always due (first cycle).
func (o *observationStore) dueGroups(groups []queryGroupKey, now time.Time) map[queryGroupKey]bool {
	due := make(map[queryGroupKey]bool, len(groups))
	for _, key := range groups {
		next, scheduled := o.nextQueryAt[key]
		if !scheduled || !now.Before(next) {
			due[key] = true
		}
	}
	return due
}

// advanceSchedule pushes each successfully-queried group's next-query time
// forward by its period.
func (o *observationStore) advanceSchedule(queried map[queryGroupKey]bool, now time.Time) {
	for key := range queried {
		o.nextQueryAt[key] = now.Add(time.Duration(key.period) * time.Second)
	}
}

func filterDueQueries(groups []queryGroupKey, queriesByGroup map[queryGroupKey][]plannedQuery, due map[queryGroupKey]bool) []plannedQuery {
	count := 0
	for _, key := range groups {
		if due[key] {
			count += len(queriesByGroup[key])
		}
	}
	out := make([]plannedQuery, 0, count)
	for _, key := range groups {
		if due[key] {
			out = append(out, queriesByGroup[key]...)
		}
	}
	return out
}

// observe writes this cycle's samples, reconciles due series that returned no
// datapoint, and re-emits cached values for series whose period was NOT queried
// this cycle (not due, or due-but-failed) so long-period metrics stay visible.
//
// dueQueries is every query issued this cycle; `queried` is the subset of
// (target, region, period) groups that succeeded; `noData` is the set of query ids that
// got a usable result with no datapoint. A due series with no sample is recorded
// as 0 only when it is a genuine no-data result (in noData) AND its metric opts
// into nil-as-zero; otherwise (a gauge, or a per-result error/absent id) its
// cached value is dropped so it gaps until fresh data — no stale value, no false
// 0. A cached value otherwise survives across cycles until its instance leaves
// discovery and pruneObserved drops it.
func (o *observationStore) observe(dueQueries []plannedQuery, samples []querySample, noData map[string]bool, queried map[queryGroupKey]bool) {
	meter := o.store.Write().SnapshotMeter("")

	observedThisCycle := make(map[string]bool, len(samples))
	for _, s := range samples {
		key := observedKey(s.seriesName, s.labels)
		writeSample(meter, s.seriesName, s.labels, s.tagLabels, s.value)
		o.lastObserved[key] = observedSeries{
			seriesName: s.seriesName,
			labels:     s.labels,
			tagLabels:  s.tagLabels,
			value:      s.value,
			groupKey:   s.groupKey(),
		}
		observedThisCycle[key] = true
	}

	// Reconcile due series with no sample this cycle. Zero-fill only a genuine
	// no-datapoint result (in noData) for a nil-as-zero metric; a per-result error
	// (InternalError/Forbidden, not in noData) or a gauge drops its cache so the
	// series gaps rather than recording a false 0.
	for _, pq := range dueQueries {
		if !queried[pq.groupKey()] {
			continue // group failed this cycle: keep cache, re-emit below, retry next cycle
		}
		key := observedKey(pq.seriesName, pq.labels)
		if observedThisCycle[key] {
			continue // had a datapoint
		}
		if pq.nilAsZero && noData[pq.id] {
			writeSample(meter, pq.seriesName, pq.labels, pq.tagLabels, 0)
			o.lastObserved[key] = observedSeries{
				seriesName: pq.seriesName,
				labels:     pq.labels,
				tagLabels:  pq.tagLabels,
				value:      0,
				groupKey:   pq.groupKey(),
			}
			observedThisCycle[key] = true
		} else {
			delete(o.lastObserved, key) // genuine gap: stop re-emitting the last value
		}
	}

	for key, obs := range o.lastObserved {
		if observedThisCycle[key] || queried[obs.groupKey] {
			continue
		}
		writeSample(meter, obs.seriesName, obs.labels, obs.tagLabels, obs.value)
	}
}

func writeSample(meter metrix.SnapshotMeter, seriesName string, labels, tagLabels []metrix.Label, value float64) {
	// Identity labels are shared read-only across an instance's queries, so tag
	// enrichment is concatenated into a FRESH slice, never appended onto labels.
	// tagLabels are non-identity (not part of observedKey), so a tag change never
	// churns retention/scheduling.
	all := labels
	if len(tagLabels) > 0 {
		all = make([]metrix.Label, 0, len(labels)+len(tagLabels))
		all = append(all, labels...)
		all = append(all, tagLabels...)
	}
	// WithFloat marks the metric float-native; chartengine inherits that onto the
	// chart dimension, so CloudWatch's fractional values render at full precision
	// without injecting options.float per dimension.
	meter.WithLabels(all...).Gauge(seriesName, metrix.WithFloat(true)).Observe(value)
}

// pruneObserved drops both the retention-cache entries and the per-(target, region,
// period) schedule entries that are no longer in the current query plan. Dropping
// the retention cache stops a removed resource from being re-emitted. Dropping the
// schedule ensures a (target, region, period) group that fully left discovery and later
// reappears is treated as unscheduled — and so queried on its first cycle back —
// instead of waiting for a stale nextQueryAt (up to a full period, e.g. ~24h for a
// daily group) to expire.
func (o *observationStore) pruneObserved(plan []plannedQuery) {
	validSeries := make(map[string]struct{}, len(plan))
	validGroups := make(map[queryGroupKey]struct{})
	for _, pq := range plan {
		validSeries[observedKey(pq.seriesName, pq.labels)] = struct{}{}
		validGroups[pq.groupKey()] = struct{}{}
	}
	for key := range o.lastObserved {
		if _, ok := validSeries[key]; !ok {
			delete(o.lastObserved, key)
		}
	}
	for key := range o.nextQueryAt {
		if _, ok := validGroups[key]; !ok {
			delete(o.nextQueryAt, key)
		}
	}
}

// observedKey is the stable identity of a series: its name plus its label values
// in declared order ({account_id, region, <identifying dimension labels>}).
func observedKey(seriesName string, labels []metrix.Label) string {
	var b strings.Builder
	b.WriteString(seriesName)
	for _, l := range labels {
		b.WriteString(instanceKeySep)
		b.WriteString(l.Value)
	}
	return b.String()
}
