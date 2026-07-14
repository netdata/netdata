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

func TestQueryMetricRequestUnits(t *testing.T) {
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
			var queries []plannedQuery
			for group, count := range tc.groups {
				billingKey := testStructuralID(fmt.Sprintf("metric-%d", group))
				for statistic := range count {
					query := testPlannedQuery(fmt.Sprintf("query-%d-%d", group, statistic), "base", "us-east-1", "AWS/Test", 300)
					query.billingKey = billingKey
					queries = append(queries, query)
				}
			}
			assert.Equal(t, tc.want, queryMetricRequestUnits(queries))
		})
	}
}

func TestCollectorActivity_PreservesAbortedCycleAndAggregatesByAccount(t *testing.T) {
	const accountID = "000000000000"
	store := metrix.NewCollectorStore()
	activity := newCollectorActivity(store)

	queries := makeActivityQueries(5, 1)
	activity.recordListMetrics(accountID, "us-east-1")
	activity.recordGetMetricData(accountID, "us-east-1", queries)

	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	activity.beginCycle()
	activity.write()
	managed.CycleController().AbortCycle()

	_, found := store.Read().Value(activityAPICallsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	})
	assert.False(t, found, "an aborted metrix frame must not publish partial activity")

	// A second target resolving to the same account records into the same account/region interval.
	activity.recordListMetrics(accountID, "us-east-1")
	activity.recordGetMetricData(accountID, "us-east-1", queries)
	activity.recordListMetrics(accountID, "eu-west-1")
	activity.recordGetMetricData(accountID, "eu-west-1", queries[:1])

	managed.CycleController().BeginCycle()
	activity.beginCycle()
	activity.write()
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	reader := store.Read()
	assertActivityValue(t, reader, activityAPICallsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationListMetrics,
	}, 2)
	assertActivityValue(t, reader, activityAPICallsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	}, 2)
	assertActivityValue(t, reader, activityMetricRequestsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1",
	}, 4)
	assertActivityValue(t, reader, activityQueriesMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "alpha",
	}, 10)
	assertActivityValue(t, reader, activityQueriesMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "profile": "beta",
	}, 2)
	assertActivityValue(t, reader, activityMetricRequestsMetric, metrix.Labels{
		"account_id": accountID, "region": "eu-west-1",
	}, 1)
	assertActivityValue(t, reader, activityQueriesMetric, metrix.Labels{
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

			assertActivityValue(t, store.Read(), activityAPICallsMetric, metrix.Labels{
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
	activity.recordGetMetricData(accountID, "us-east-1", []plannedQuery{query})
	commitActivity(t, store, activity)

	activity.reset()
	activity.recordGetMetricData(accountID, "us-east-1", []plannedQuery{query})
	commitActivity(t, store, activity)

	reader := store.Read()
	assertActivityValue(t, reader, activityAPICallsMetric, metrix.Labels{
		"account_id": accountID, "region": "us-east-1", "operation": activityOperationGetMetricData,
	}, 1)
	assertActivityValue(t, reader, activityMetricRequestsMetric, metrix.Labels{
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
			assert.Equal(t, tc.calls, activityCallCount(activity.snapshot(), accountID, "us-east-1", activityOperationListMetrics))
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

func activityCallCount(snapshot activitySnapshot, accountID, region, operation string) uint64 {
	for _, item := range snapshot.apiCalls {
		if item.key == (activityCallScope{
			activityScope: activityScope{accountID: accountID, region: region}, operation: operation,
		}) {
			return item.count
		}
	}
	return 0
}

func activityMetricRequestCount(snapshot activitySnapshot, accountID, region string) uint64 {
	for _, item := range snapshot.metricRequests {
		if item.key == (activityScope{accountID: accountID, region: region}) {
			return item.count
		}
	}
	return 0
}

func activityQueryCount(snapshot activitySnapshot, accountID, region, profile string) uint64 {
	for _, item := range snapshot.queries {
		if item.key == (activityProfileScope{
			activityScope: activityScope{accountID: accountID, region: region}, profile: profile,
		}) {
			return item.count
		}
	}
	return 0
}
