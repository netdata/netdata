// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	activityMetricPrefix                        = "netdata.go.plugin.collector.cloudwatch"
	activitySDKInvocationsMetric                = activityMetricPrefix + ".sdk_invocations"
	activityCalculatedMetricRequestsMetric      = activityMetricPrefix + ".get_metric_data_calculated_metric_requests"
	activityProfileMetricRequestEstimatesMetric = activityMetricPrefix + ".get_metric_data_profile_metric_request_estimates"
	activityQueryItemsMetric                    = activityMetricPrefix + ".get_metric_data_query_items"

	activityOperationListMetrics   = "list_metrics"
	activityOperationGetMetricData = "get_metric_data"
)

type activityScope struct {
	accountID string
	region    string
}

type activitySDKInvocationScope struct {
	activityScope
	operation string
}

type activityProfileScope struct {
	activityScope
	profile string
}

type queryBatchProfileActivity struct {
	profile               string
	metricRequestEstimate uint64
	queryItems            uint64
}

type queryBatchActivity struct {
	calculatedMetricRequests uint64
	profiles                 []queryBatchProfileActivity
}

func buildQueryBatchActivity(queries []plannedQuery) queryBatchActivity {
	type profileActivityBuilder struct {
		profile    string
		groups     map[structuralID]int
		queryItems uint64
	}

	overallGroups := make(map[structuralID]int)
	profileIndex := make(map[string]int)
	var builders []profileActivityBuilder
	for _, query := range queries {
		billingKey := queryMetricBillingKey(query)
		overallGroups[billingKey]++
		index, ok := profileIndex[query.profile]
		if !ok {
			index = len(builders)
			profileIndex[query.profile] = index
			builders = append(builders, profileActivityBuilder{profile: query.profile})
		}
		builders[index].queryItems++
	}

	for i := range builders {
		builders[i].groups = make(map[structuralID]int, builders[i].queryItems)
	}
	for _, query := range queries {
		builders[profileIndex[query.profile]].groups[queryMetricBillingKey(query)]++
	}
	sort.Slice(builders, func(i, j int) bool { return builders[i].profile < builders[j].profile })

	activity := queryBatchActivity{
		calculatedMetricRequests: uint64(metricRequestUnitsForBillingGroups(overallGroups)),
		profiles:                 make([]queryBatchProfileActivity, 0, len(builders)),
	}
	for _, builder := range builders {
		activity.profiles = append(activity.profiles, queryBatchProfileActivity{
			profile:               builder.profile,
			metricRequestEstimate: uint64(metricRequestUnitsForBillingGroups(builder.groups)),
			queryItems:            builder.queryItems,
		})
	}
	return activity
}

// collectorActivity retains AWS activity until metrix confirms that its staged
// frame committed. Failed cycles therefore carry their work into the next
// successfully committed interval.
type collectorActivity struct {
	mu sync.Mutex

	store metrix.CollectorStore

	sdkInvocations                map[activitySDKInvocationScope]uint64
	calculatedMetricRequests      map[activityScope]uint64
	profileMetricRequestEstimates map[activityProfileScope]uint64
	queryItems                    map[activityProfileScope]uint64

	frameStaged                         bool
	stagedAfterSuccessSeq               uint64
	sdkInvocationsMetric                metrix.SnapshotGaugeVec
	calculatedMetricRequestsMetric      metrix.SnapshotGaugeVec
	profileMetricRequestEstimatesMetric metrix.SnapshotGaugeVec
	queryItemsMetric                    metrix.SnapshotGaugeVec
}

func newCollectorActivity(store metrix.CollectorStore) *collectorActivity {
	meter := store.Write().SnapshotMeter(activityMetricPrefix)
	return &collectorActivity{
		store:                         store,
		sdkInvocations:                make(map[activitySDKInvocationScope]uint64),
		calculatedMetricRequests:      make(map[activityScope]uint64),
		profileMetricRequestEstimates: make(map[activityProfileScope]uint64),
		queryItems:                    make(map[activityProfileScope]uint64),
		sdkInvocationsMetric: meter.
			Vec("account_id", "region", "operation").
			Gauge("sdk_invocations"),
		calculatedMetricRequestsMetric: meter.
			Vec("account_id", "region").
			Gauge("get_metric_data_calculated_metric_requests"),
		profileMetricRequestEstimatesMetric: meter.
			Vec("account_id", "region", "profile").
			Gauge("get_metric_data_profile_metric_request_estimates"),
		queryItemsMetric: meter.
			Vec("account_id", "region", "profile").
			Gauge("get_metric_data_query_items"),
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
	zeroValues(a.sdkInvocations)
	zeroValues(a.calculatedMetricRequests)
	zeroValues(a.profileMetricRequestEstimates)
	zeroValues(a.queryItems)
	a.frameStaged = false
}

func (a *collectorActivity) recordListMetrics(accountID, region string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.sdkInvocations[activitySDKInvocationScope{
		activityScope: activityScope{accountID: accountID, region: region},
		operation:     activityOperationListMetrics,
	}]++
}

func (a *collectorActivity) recordGetMetricData(accountID, region string, activity queryBatchActivity) {
	if a == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	scope := activityScope{accountID: accountID, region: region}
	a.sdkInvocations[activitySDKInvocationScope{activityScope: scope, operation: activityOperationGetMetricData}]++
	a.calculatedMetricRequests[scope] += activity.calculatedMetricRequests
	for _, profile := range activity.profiles {
		profileScope := activityProfileScope{activityScope: scope, profile: profile.profile}
		a.profileMetricRequestEstimates[profileScope] += profile.metricRequestEstimate
		a.queryItems[profileScope] += profile.queryItems
	}
}

func (a *collectorActivity) write() {
	if a == nil {
		return
	}
	snapshot := a.stage()
	for _, item := range snapshot.sdkInvocations {
		a.sdkInvocationsMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.operation).
			Observe(float64(item.count))
	}
	for _, item := range snapshot.calculatedMetricRequests {
		a.calculatedMetricRequestsMetric.
			WithLabelValues(item.key.accountID, item.key.region).
			Observe(float64(item.count))
	}
	for _, item := range snapshot.profileMetricRequestEstimates {
		a.profileMetricRequestEstimatesMetric.
			WithLabelValues(item.key.accountID, item.key.region, item.key.profile).
			Observe(float64(item.count))
	}
	for _, item := range snapshot.queryItems {
		a.queryItemsMetric.
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

	a.sdkInvocations = make(map[activitySDKInvocationScope]uint64)
	a.calculatedMetricRequests = make(map[activityScope]uint64)
	a.profileMetricRequestEstimates = make(map[activityProfileScope]uint64)
	a.queryItems = make(map[activityProfileScope]uint64)
	a.frameStaged = false
	a.stagedAfterSuccessSeq = 0
}

type activityCount[T any] struct {
	key   T
	count uint64
}

type activitySnapshot struct {
	sdkInvocations                []activityCount[activitySDKInvocationScope]
	calculatedMetricRequests      []activityCount[activityScope]
	profileMetricRequestEstimates []activityCount[activityProfileScope]
	queryItems                    []activityCount[activityProfileScope]
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
		sdkInvocations:                make([]activityCount[activitySDKInvocationScope], 0, len(a.sdkInvocations)),
		calculatedMetricRequests:      make([]activityCount[activityScope], 0, len(a.calculatedMetricRequests)),
		profileMetricRequestEstimates: make([]activityCount[activityProfileScope], 0, len(a.profileMetricRequestEstimates)),
		queryItems:                    make([]activityCount[activityProfileScope], 0, len(a.queryItems)),
	}
	for key, count := range a.sdkInvocations {
		snapshot.sdkInvocations = append(snapshot.sdkInvocations, activityCount[activitySDKInvocationScope]{key: key, count: count})
	}
	for key, count := range a.calculatedMetricRequests {
		snapshot.calculatedMetricRequests = append(snapshot.calculatedMetricRequests, activityCount[activityScope]{key: key, count: count})
	}
	for key, count := range a.profileMetricRequestEstimates {
		snapshot.profileMetricRequestEstimates = append(snapshot.profileMetricRequestEstimates, activityCount[activityProfileScope]{key: key, count: count})
	}
	for key, count := range a.queryItems {
		snapshot.queryItems = append(snapshot.queryItems, activityCount[activityProfileScope]{key: key, count: count})
	}
	sort.Slice(snapshot.sdkInvocations, func(i, j int) bool {
		a, b := snapshot.sdkInvocations[i].key, snapshot.sdkInvocations[j].key
		return a.accountID < b.accountID ||
			(a.accountID == b.accountID && a.region < b.region) ||
			(a.accountID == b.accountID && a.region == b.region && a.operation < b.operation)
	})
	sort.Slice(snapshot.calculatedMetricRequests, func(i, j int) bool {
		a, b := snapshot.calculatedMetricRequests[i].key, snapshot.calculatedMetricRequests[j].key
		return a.accountID < b.accountID || (a.accountID == b.accountID && a.region < b.region)
	})
	sortActivityProfileCounts(snapshot.profileMetricRequestEstimates)
	sortActivityProfileCounts(snapshot.queryItems)
	return snapshot
}

func sortActivityProfileCounts(counts []activityCount[activityProfileScope]) {
	sort.Slice(counts, func(i, j int) bool {
		a, b := counts[i].key, counts[j].key
		return a.accountID < b.accountID ||
			(a.accountID == b.accountID && a.region < b.region) ||
			(a.accountID == b.accountID && a.region == b.region && a.profile < b.profile)
	})
}

func zeroValues[K comparable](values map[K]uint64) {
	for key := range values {
		values[key] = 0
	}
}
