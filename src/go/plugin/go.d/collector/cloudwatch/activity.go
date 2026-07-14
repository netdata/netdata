// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	activityMetricPrefix         = "netdata.go.plugin.collector.cloudwatch"
	activityAPICallsMetric       = activityMetricPrefix + ".api_calls_total"
	activityMetricRequestsMetric = activityMetricPrefix + ".get_metric_data_metric_requests_total"
	activityQueriesMetric        = activityMetricPrefix + ".get_metric_data_queries_total"

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

// collectorActivity keeps AWS activity independently of the staged metrix
// frame. Calls made by a failed collection cycle therefore remain visible when
// a later cycle commits successfully.
type collectorActivity struct {
	mu sync.Mutex

	apiCalls       map[activityCallScope]uint64
	metricRequests map[activityScope]uint64
	queries        map[activityProfileScope]uint64

	apiCallsMetric       metrix.SnapshotCounterVec
	metricRequestsMetric metrix.SnapshotCounterVec
	queriesMetric        metrix.SnapshotCounterVec
}

func newCollectorActivity(store metrix.CollectorStore) *collectorActivity {
	meter := store.Write().SnapshotMeter(activityMetricPrefix)
	return &collectorActivity{
		apiCalls:       make(map[activityCallScope]uint64),
		metricRequests: make(map[activityScope]uint64),
		queries:        make(map[activityProfileScope]uint64),
		apiCallsMetric: meter.
			Vec("account_id", "region", "operation").
			Counter("api_calls_total"),
		metricRequestsMetric: meter.
			Vec("account_id", "region").
			Counter("get_metric_data_metric_requests_total"),
		queriesMetric: meter.
			Vec("account_id", "region", "profile").
			Counter("get_metric_data_queries_total"),
	}
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
	snapshot := a.snapshot()
	for _, item := range snapshot.apiCalls {
		a.apiCallsMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.operation).
			ObserveTotal(float64(item.total))
	}
	for _, item := range snapshot.metricRequests {
		a.metricRequestsMetric.
			WithLabelValues(item.key.accountID, item.key.region).
			ObserveTotal(float64(item.total))
	}
	for _, item := range snapshot.queries {
		a.queriesMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.profile).
			ObserveTotal(float64(item.total))
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
}

type activityTotal[T any] struct {
	key   T
	total uint64
}

type activitySnapshot struct {
	apiCalls       []activityTotal[activityCallScope]
	metricRequests []activityTotal[activityScope]
	queries        []activityTotal[activityProfileScope]
}

func (a *collectorActivity) snapshot() activitySnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot := activitySnapshot{
		apiCalls:       make([]activityTotal[activityCallScope], 0, len(a.apiCalls)),
		metricRequests: make([]activityTotal[activityScope], 0, len(a.metricRequests)),
		queries:        make([]activityTotal[activityProfileScope], 0, len(a.queries)),
	}
	for key, total := range a.apiCalls {
		snapshot.apiCalls = append(snapshot.apiCalls, activityTotal[activityCallScope]{key: key, total: total})
	}
	for key, total := range a.metricRequests {
		snapshot.metricRequests = append(snapshot.metricRequests, activityTotal[activityScope]{key: key, total: total})
	}
	for key, total := range a.queries {
		snapshot.queries = append(snapshot.queries, activityTotal[activityProfileScope]{key: key, total: total})
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
