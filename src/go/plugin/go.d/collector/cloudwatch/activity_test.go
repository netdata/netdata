// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricRequestUnitsForBillingGroups(t *testing.T) {
	tests := map[string]struct {
		groups []int
		want   int
	}{
		"no queries":                           {want: 0},
		"one statistic":                        {groups: []int{1}, want: 1},
		"five statistics":                      {groups: []int{5}, want: 1},
		"six statistics":                       {groups: []int{6}, want: 2},
		"nine statistics":                      {groups: []int{9}, want: 2},
		"groups remain separate":               {groups: []int{5, 1}, want: 2},
		"each group rounds independently":      {groups: []int{6, 6}, want: 4},
		"several single-statistic AWS metrics": {groups: []int{1, 1, 1}, want: 3},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			groups := make(map[structuralID]int)
			for group, count := range tc.groups {
				groups[testStructuralID(fmt.Sprintf("metric-%d", group))] = count
			}
			assert.Equal(t, tc.want, metricRequestUnitsForBillingGroups(groups))
			assert.Len(t, billingUnitShapes(groups), tc.want, "activity accounting must match request packing")
		})
	}
}

func TestBuildQueryBatchActivity(t *testing.T) {
	tests := map[string]struct {
		queries []plannedQuery
		want    queryBatchActivity
	}{
		"one profile five statistics": {
			queries: makeActivityQueries(5),
			want: queryBatchActivity{
				calculatedMetricRequests: 1,
				profiles:                 []queryBatchProfileActivity{{profile: "alpha", metricRequestEstimate: 1, queryItems: 5}},
			},
		},
		"one profile six statistics": {
			queries: makeActivityQueries(6),
			want: queryBatchActivity{
				calculatedMetricRequests: 2,
				profiles:                 []queryBatchProfileActivity{{profile: "alpha", metricRequestEstimate: 2, queryItems: 6}},
			},
		},
		"cross-profile overlap is intentionally non-additive": {
			queries: makeActivityQueries(3, 2),
			want: queryBatchActivity{
				calculatedMetricRequests: 1,
				profiles: []queryBatchProfileActivity{
					{profile: "alpha", metricRequestEstimate: 1, queryItems: 3},
					{profile: "beta", metricRequestEstimate: 1, queryItems: 2},
				},
			},
		},
		"cross-profile one plus one": {
			queries: makeActivityQueries(1, 1),
			want: queryBatchActivity{
				calculatedMetricRequests: 1,
				profiles: []queryBatchProfileActivity{
					{profile: "alpha", metricRequestEstimate: 1, queryItems: 1},
					{profile: "beta", metricRequestEstimate: 1, queryItems: 1},
				},
			},
		},
		"cross-profile five plus five": {
			queries: makeActivityQueries(5, 5),
			want: queryBatchActivity{
				calculatedMetricRequests: 2,
				profiles: []queryBatchProfileActivity{
					{profile: "alpha", metricRequestEstimate: 1, queryItems: 5},
					{profile: "beta", metricRequestEstimate: 1, queryItems: 5},
				},
			},
		},
		"cross-profile six plus one": {
			queries: makeActivityQueries(6, 1),
			want: queryBatchActivity{
				calculatedMetricRequests: 2,
				profiles: []queryBatchProfileActivity{
					{profile: "alpha", metricRequestEstimate: 2, queryItems: 6},
					{profile: "beta", metricRequestEstimate: 1, queryItems: 1},
				},
			},
		},
		"one profile eleven statistics": {
			queries: makeActivityQueries(11),
			want: queryBatchActivity{
				calculatedMetricRequests: 3,
				profiles:                 []queryBatchProfileActivity{{profile: "alpha", metricRequestEstimate: 3, queryItems: 11}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildQueryBatchActivity(tc.queries))

			reversed := append([]plannedQuery(nil), tc.queries...)
			for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
				reversed[left], reversed[right] = reversed[right], reversed[left]
			}
			assert.Equal(t, tc.want, buildQueryBatchActivity(reversed), "activity is independent of query order")
		})
	}
}

func TestCollectorActivity_PreservesAbortedCycleAndAggregatesByAccount(t *testing.T) {
	const accountID = "000000000000"
	store := metrix.NewCollectorStore()
	activity := newCollectorActivity(store)

	queries := makeActivityQueries(5, 1)
	counts := buildQueryBatchActivity(queries)
	activity.recordListMetrics(accountID, "us-east-1")
	activity.recordGetMetricData(accountID, "us-east-1", counts)

	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	activity.beginCycle()
	activity.write()
	managed.CycleController().AbortCycle()

	_, found := store.Read().Value(activitySDKInvocationsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	})
	assert.False(t, found, "an aborted metrix frame must not publish partial activity")

	// A second target resolving to the same account records into the same account/region interval.
	activity.recordListMetrics(accountID, "us-east-1")
	activity.recordGetMetricData(accountID, "us-east-1", counts)
	activity.recordListMetrics(accountID, "eu-west-1")
	activity.recordGetMetricData(accountID, "eu-west-1", buildQueryBatchActivity(queries[:1]))

	managed.CycleController().BeginCycle()
	activity.beginCycle()
	activity.write()
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	reader := store.Read()
	assertActivityValue(t, reader, activitySDKInvocationsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationListMetrics,
	}, 2)
	assertActivityValue(t, reader, activitySDKInvocationsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	}, 2)
	assertActivityValue(t, reader, activityCalculatedMetricRequestsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1",
	}, 4)
	assertActivityValue(t, reader, activityProfileMetricRequestEstimatesMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "alpha",
	}, 2)
	assertActivityValue(t, reader, activityProfileMetricRequestEstimatesMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "beta",
	}, 2)
	assertActivityValue(t, reader, activityQueryItemsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "alpha",
	}, 10)
	assertActivityValue(t, reader, activityQueryItemsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "beta",
	}, 2)
	assertActivityValue(t, reader, activityCalculatedMetricRequestsMetric, metrix.Labels{
		"account_id": accountID, "region": "eu-west-1",
	}, 1)
	assertActivityValue(t, reader, activityProfileMetricRequestEstimatesMetric, metrix.Labels{
		"account_id": accountID, "region": "eu-west-1", "profile": "alpha",
	}, 1)
	assertActivityValue(t, reader, activityQueryItemsMetric, metrix.Labels{
		"account_id": accountID, "region": "eu-west-1", "profile": "alpha",
	}, 1)
}

func TestCollectorActivity_AcknowledgesOnlyCommittedFrames(t *testing.T) {
	tests := map[string]struct {
		failFirstActivityCommit bool
		wantRecoveredCount      metrix.SampleValue
	}{
		"successful frame is zero in the next quiet interval": {
			wantRecoveredCount: 0,
		},
		"failed commit carries activity into the next interval": {
			failFirstActivityCommit: true,
			wantRecoveredCount:      2,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const accountID = "000000000000"
			store := metrix.NewCollectorStore()
			activity := newCollectorActivity(store)
			managed, ok := metrix.AsCycleManagedStore(store)
			require.True(t, ok)

			if tc.failFirstActivityCommit {
				managed.CycleController().BeginCycle()
				store.Write().SnapshotMeter("test").Gauge("conflict").Observe(1)
				require.NoError(t, managed.CycleController().CommitCycleSuccess())
			}

			managed.CycleController().BeginCycle()
			activity.beginCycle()
			activity.recordListMetrics(accountID, "us-east-1")
			activity.write()
			if tc.failFirstActivityCommit {
				store.Write().SnapshotMeter("test").Gauge("conflict").Observe(2)
				store.Write().SnapshotMeter("test").Counter("conflict").ObserveTotal(3)
				require.Error(t, managed.CycleController().CommitCycleSuccess())
			} else {
				require.NoError(t, managed.CycleController().CommitCycleSuccess())
			}

			managed.CycleController().BeginCycle()
			activity.beginCycle()
			if tc.failFirstActivityCommit {
				activity.recordListMetrics(accountID, "us-east-1")
			}
			activity.write()
			require.NoError(t, managed.CycleController().CommitCycleSuccess())

			assertActivityValue(t, store.Read(), activitySDKInvocationsMetric, metrix.Labels{
				"account_id": accountID, "region": "us-east-1", "operation": activityOperationListMetrics,
			}, tc.wantRecoveredCount)
		})
	}
}

func TestCollectorActivity_ResetDiscardsPriorActivity(t *testing.T) {
	const accountID = "000000000000"
	store := metrix.NewCollectorStore()
	activity := newCollectorActivity(store)
	query := makeActivityQueries(1)[0]
	counts := buildQueryBatchActivity([]plannedQuery{query})
	activity.recordGetMetricData(accountID, "us-east-1", counts)
	commitActivity(t, store, activity)

	activity.reset()
	activity.recordGetMetricData(accountID, "us-east-1", counts)
	commitActivity(t, store, activity)

	reader := store.Read()
	assertActivityValue(t, reader, activitySDKInvocationsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	}, 1)
	assertActivityValue(t, reader, activityCalculatedMetricRequestsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1",
	}, 1)
}

func TestCollectorActivity_ListMetricsPagesAndFailures(t *testing.T) {
	const accountID = "000000000000"
	profile := cwprofiles.ResolvedProfile{Name: "test", Config: dimProfile("AWS/Test", 300, "Id")}
	tests := map[string]struct {
		client  *fakeCloudWatch
		wantErr bool
		calls   uint64
	}{
		"pagination counts every page": {
			client: &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{
				page(nil, "next"), page(nil, ""),
			}},
			calls: 2,
		},
		"failed request is still activity": {
			client:  &fakeCloudWatch{err: assert.AnError},
			wantErr: true,
			calls:   1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			activity := newCollectorActivity(store)
			scanner := newDiscoveryGroupScanner(discoveryGroup{
				Target: "base", AccountID: accountID, Region: "us-east-1", Namespace: "AWS/Test",
				Profiles: []cwprofiles.ResolvedProfile{profile},
			}, activity)
			budget := testDiscoveryBudget(1)
			var err error
			for !scanner.done && err == nil {
				err = scanner.scanPage(context.Background(), tc.client, budget)
			}
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.calls, activitySDKInvocationCount(activity.snapshot(), accountID, "us-east-1", activityOperationListMetrics))
		})
	}
}

func makeActivityQueries(profileCounts ...int) []plannedQuery {
	profiles := []string{"alpha", "beta", "gamma"}
	var queries []plannedQuery
	for profileIndex, count := range profileCounts {
		billingKey := testStructuralID("shared-metric")
		for statistic := range count {
			query := testPlannedQuery(fmt.Sprintf("%s-%d", profiles[profileIndex], statistic), "base", "us-east-1", "AWS/Test", 300)
			query.profile = profiles[profileIndex]
			query.billingKey = billingKey
			queries = append(queries, query)
		}
	}
	return queries
}

func commitActivity(t *testing.T, store metrix.CollectorStore, activity *collectorActivity) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	activity.beginCycle()
	activity.write()
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
}

func assertActivityValue(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, want metrix.SampleValue) {
	t.Helper()
	got, ok := reader.Value(name, labels)
	require.True(t, ok, "metric %s with labels %v not found", name, labels)
	assert.Equal(t, want, got)
}

func activitySDKInvocationCount(snapshot activitySnapshot, accountID, region, operation string) uint64 {
	for _, item := range snapshot.sdkInvocations {
		if item.key == (activitySDKInvocationScope{
			activityScope: activityScope{accountID: accountID, region: region}, operation: operation,
		}) {
			return item.count
		}
	}
	return 0
}

func activityCalculatedMetricRequestCount(snapshot activitySnapshot, accountID, region string) uint64 {
	for _, item := range snapshot.calculatedMetricRequests {
		if item.key == (activityScope{accountID: accountID, region: region}) {
			return item.count
		}
	}
	return 0
}

func activityProfileMetricRequestEstimate(snapshot activitySnapshot, accountID, region, profile string) uint64 {
	return activityProfileCount(snapshot.profileMetricRequestEstimates, accountID, region, profile)
}

func activityQueryItemCount(snapshot activitySnapshot, accountID, region, profile string) uint64 {
	return activityProfileCount(snapshot.queryItems, accountID, region, profile)
}

func activityProfileCount(counts []activityCount[activityProfileScope], accountID, region, profile string) uint64 {
	for _, item := range counts {
		if item.key == (activityProfileScope{
			activityScope: activityScope{accountID: accountID, region: region}, profile: profile,
		}) {
			return item.count
		}
	}
	return 0
}

func BenchmarkCollectorActivityRecordGetMetricData(b *testing.B) {
	queries := activityBenchmarkQueries()
	activity := newCollectorActivity(metrix.NewCollectorStore())
	counts := buildQueryBatchActivity(queries)

	b.ReportAllocs()
	for range b.N {
		activity.recordGetMetricData("000000000000", "us-east-1", counts)
	}
}

func BenchmarkBuildQueryBatchActivity(b *testing.B) {
	queries := activityBenchmarkQueries()

	b.ReportAllocs()
	for range b.N {
		buildQueryBatchActivity(queries)
	}
}

func activityBenchmarkQueries() []plannedQuery {
	const (
		profileCount = 50
		queryCount   = maxQueriesPerRequest
	)
	queries := make([]plannedQuery, queryCount)
	for i := range queries {
		query := testPlannedQuery(fmt.Sprintf("query-%d", i), "base", "us-east-1", "AWS/Test", 300)
		query.profile = fmt.Sprintf("profile_%d", i%profileCount)
		query.billingKey = testStructuralID(fmt.Sprintf("metric-%d", i/maxStatisticsPerMetricBillingUnit))
		queries[i] = query
	}
	return queries
}
