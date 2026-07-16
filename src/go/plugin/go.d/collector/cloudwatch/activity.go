// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	activityMetricPrefix         = "netdata.go.plugin.collector.cloudwatch"
	activityAPICallsMetric       = activityMetricPrefix + ".api_calls"
	activityMetricRequestsMetric = activityMetricPrefix + ".get_metric_data_metric_requests"
	activityQueriesMetric        = activityMetricPrefix + ".get_metric_data_queries"

	activityOperationListMetrics   = "list_metrics"
	activityOperationGetMetricData = "get_metric_data"
)

type activityScope struct {
	accountID string
	region    string
}

type activityCallScope struct {
	activityScope
	operation string
}

type activityProfileScope struct {
	activityScope
	profile string
}

// collectorActivity retains AWS activity until metrix confirms that its staged
// frame committed. Failed cycles therefore carry their work into the next
// successfully committed interval.
type collectorActivity struct {
	mu sync.Mutex

	store metrix.CollectorStore

	apiCalls       map[activityCallScope]uint64
	metricRequests map[activityScope]uint64
	queries        map[activityProfileScope]uint64

	frameStaged           bool
	stagedAfterSuccessSeq uint64
	apiCallsMetric        metrix.SnapshotGaugeVec
	metricRequestsMetric  metrix.SnapshotGaugeVec
	queriesMetric         metrix.SnapshotGaugeVec
}

func newCollectorActivity(store metrix.CollectorStore) *collectorActivity {
	meter := store.Write().SnapshotMeter(activityMetricPrefix)
	return &collectorActivity{
		store:          store,
		apiCalls:       make(map[activityCallScope]uint64),
		metricRequests: make(map[activityScope]uint64),
		queries:        make(map[activityProfileScope]uint64),
		apiCallsMetric: meter.
			Vec("account_id", "region", "operation").
			Gauge("api_calls"),
		metricRequestsMetric: meter.
			Vec("account_id", "region").
			Gauge("get_metric_data_metric_requests"),
		queriesMetric: meter.
			Vec("account_id", "region", "profile").
			Gauge("get_metric_data_queries"),
	}
}

// beginCycle acknowledges the previously staged interval only after metrix has
// committed it. Keys remain allocated so quiet intervals publish zeroes.
func (a *collectorActivity) beginCycle() {
	if a == nil {
		return
	}
	lastSuccessSeq := a.store.Read().CollectMeta().LastSuccessSeq

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.frameStaged || lastSuccessSeq <= a.stagedAfterSuccessSeq {
		return
	}
	zeroValues(a.apiCalls)
	zeroValues(a.metricRequests)
	zeroValues(a.queries)
	a.frameStaged = false
}

func (a *collectorActivity) recordListMetrics(accountID, region string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.apiCalls[activityCallScope{
		activityScope: activityScope{accountID: accountID, region: region},
		operation:     activityOperationListMetrics,
	}]++
}

func (a *collectorActivity) recordGetMetricData(accountID, region string, queries []plannedQuery) {
	if a == nil {
		return
	}
	metricRequests := queryMetricRequestUnits(queries)

	a.mu.Lock()
	defer a.mu.Unlock()

	scope := activityScope{accountID: accountID, region: region}
	a.apiCalls[activityCallScope{activityScope: scope, operation: activityOperationGetMetricData}]++
	a.metricRequests[scope] += uint64(metricRequests)
	for _, query := range queries {
		a.queries[activityProfileScope{activityScope: scope, profile: query.profile}]++
	}
}

func (a *collectorActivity) write() {
	if a == nil {
		return
	}
	snapshot := a.stage()
	for _, item := range snapshot.apiCalls {
		a.apiCallsMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.operation).
			Observe(float64(item.count))
	}
	for _, item := range snapshot.metricRequests {
		a.metricRequestsMetric.
			WithLabelValues(item.key.accountID, item.key.region).
			Observe(float64(item.count))
	}
	for _, item := range snapshot.queries {
		a.queriesMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.profile).
			Observe(float64(item.count))
	}
}

func (a *collectorActivity) reset() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.apiCalls = make(map[activityCallScope]uint64)
	a.metricRequests = make(map[activityScope]uint64)
	a.queries = make(map[activityProfileScope]uint64)
	a.frameStaged = false
	a.stagedAfterSuccessSeq = 0
}

type activityCount[T any] struct {
	key   T
	count uint64
}

type activitySnapshot struct {
	apiCalls       []activityCount[activityCallScope]
	metricRequests []activityCount[activityScope]
	queries        []activityCount[activityProfileScope]
}

func (a *collectorActivity) snapshot() activitySnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *collectorActivity) stage() activitySnapshot {
	lastSuccessSeq := a.store.Read().CollectMeta().LastSuccessSeq

	a.mu.Lock()
	defer a.mu.Unlock()

	a.frameStaged = true
	a.stagedAfterSuccessSeq = lastSuccessSeq
	return a.snapshotLocked()
}

func (a *collectorActivity) snapshotLocked() activitySnapshot {
	snapshot := activitySnapshot{
		apiCalls:       make([]activityCount[activityCallScope], 0, len(a.apiCalls)),
		metricRequests: make([]activityCount[activityScope], 0, len(a.metricRequests)),
		queries:        make([]activityCount[activityProfileScope], 0, len(a.queries)),
	}
	for key, count := range a.apiCalls {
		snapshot.apiCalls = append(snapshot.apiCalls, activityCount[activityCallScope]{key: key, count: count})
	}
	for key, count := range a.metricRequests {
		snapshot.metricRequests = append(snapshot.metricRequests, activityCount[activityScope]{key: key, count: count})
	}
	for key, count := range a.queries {
		snapshot.queries = append(snapshot.queries, activityCount[activityProfileScope]{key: key, count: count})
	}
	sort.Slice(snapshot.apiCalls, func(i, j int) bool {
		a, b := snapshot.apiCalls[i].key, snapshot.apiCalls[j].key
		return a.accountID < b.accountID ||
			(a.accountID == b.accountID && a.region < b.region) ||
			(a.accountID == b.accountID && a.region == b.region && a.operation < b.operation)
	})
	sort.Slice(snapshot.metricRequests, func(i, j int) bool {
		a, b := snapshot.metricRequests[i].key, snapshot.metricRequests[j].key
		return a.accountID < b.accountID || (a.accountID == b.accountID && a.region < b.region)
	})
	sort.Slice(snapshot.queries, func(i, j int) bool {
		a, b := snapshot.queries[i].key, snapshot.queries[j].key
		return a.accountID < b.accountID ||
			(a.accountID == b.accountID && a.region < b.region) ||
			(a.accountID == b.accountID && a.region == b.region && a.profile < b.profile)
	})
	return snapshot
}

func zeroValues[K comparable](values map[K]uint64) {
	for key := range values {
		values[key] = 0
	}
}
