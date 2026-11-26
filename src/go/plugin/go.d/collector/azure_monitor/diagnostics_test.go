// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryDiagnostics(t *testing.T) {
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })
	const typ = "Microsoft.Storage/storageAccounts"
	rg := &mockResourceGraph{}
	for _, name := range []string{"group", "region", "tag", "z-excluded", "included"} {
		row := testResource(typ, name, map[string]any{"tags": map[string]any{"env": "private-filter-value"}})
		switch name {
		case "group":
			row["resourceGroup"] = "other"
		case "region":
			row["location"] = "other"
		case "tag", "z-excluded":
			row["tags"] = nil
		}
		rg.resources = append(rg.resources, row)
	}
	var logs bytes.Buffer
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Logger = logger.NewWithWriter(&logs)
	c.Config = testConfig()
	c.Profiles.Mode = " EXACT "
	c.Discovery.Mode = " FILTERS "
	c.Profiles.ModeExact = testProfilesModeEntries("storage_accounts", "front_door")
	c.Profiles.ModeExact.Entries[0].Filters = &ResourceFiltersConfig{
		ResourceGroups: []string{"rg-a"}, Regions: []string{"eastus"},
		Tags: map[string][]string{"env": {"private-filter-value"}},
	}
	now := time.Now()
	c.now = func() time.Time { return now }
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	assert.Contains(t, logs.String(), "mode=filters profile_mode=exact profile=storage_accounts candidates=5 matched=1")
	assert.Contains(t, logs.String(), "profile=front_door candidates=0 matched=0")
	for _, want := range []string{"included", "reason=resource_group", "reason=region", "reason=tag"} {
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, logs.String(), "private-filter-value")
	assert.NotContains(t, logs.String(), "z-excluded")

	// Identical refreshes are available in debug. A profile count change is
	// visible at info even when the number of candidates stays the same.
	logs.Reset()
	now = now.Add(time.Hour)
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "level=debug")
	assert.NotContains(t, logs.String(), "level=info")
	logger.Level.Set(slog.LevelInfo)
	logs.Reset()
	rg.resources[2]["tags"] = map[string]any{"env": "private-filter-value"}
	now = now.Add(time.Hour)
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "profile=storage_accounts candidates=5 matched=2")
	assert.NotContains(t, logs.String(), "discovery examples")
}

func TestBatchDiagnosticsMixedResponse(t *testing.T) {
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })
	const typ = "Microsoft.Storage/storageAccounts"
	active := testResource(typ, "active-account", nil)
	empty := testResource(typ, "empty-account", nil)
	missing := testResource(typ, "missing-account", nil)
	rg := &mockResourceGraph{resources: []map[string]any{active, empty, missing}}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mx := &mockMetricsClient{}
	mx.queryResponse.Values = []azmetrics.MetricData{
		{ResourceID: new(active["id"].(string)), Values: []azmetrics.Metric{metricWithAvg("Availability", now, 0)}},
		{ResourceID: new(empty["id"].(string)), Values: []azmetrics.Metric{
			{Name: &azmetrics.LocalizableString{Value: new("Transactions")}, ErrorCode: new("BadArgument"), ErrorMessage: new("private response details")},
			{ErrorMessage: new("private response details")},
		}},
		{Values: []azmetrics.Metric{{ErrorMessage: new("private response details")}}},
	}
	var logs bytes.Buffer
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("storage_accounts")
	c.now = func() time.Time { return now }
	c.Logger = logger.NewWithWriter(&logs)
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	logs.Reset()
	series, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	value, ok := seriesValue(series, "availability_average", "active-account")
	require.True(t, ok)
	assert.Zero(t, value)
	assert.Contains(t, logs.String(), "grain=PT1M resources=3 with_samples=1 empty=1 missing=1 unmatched_rows=1 metric_errors=3")
	for _, want := range []string{"empty-account", "missing-account", "Transactions", "BadArgument", "<unmatched>", "unspecified",
		"metrics=", "Availability", "start=2026-01-01T11:56:00Z end=2026-01-01T11:57:00Z"} {
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, logs.String(), "private response details")
}

func TestBatchFailureDiagnostics(t *testing.T) {
	rg := &mockResourceGraph{resources: []map[string]any{
		testResource("Microsoft.Storage/storageAccounts", "storage-example", nil),
		testResource("Microsoft.Cdn/profiles", "front-example", map[string]any{"location": "global"}),
	}}
	mx := &mockMetricsClient{}
	var logs bytes.Buffer
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("storage_accounts", "front_door")
	c.Logger = logger.NewWithWriter(&logs)
	now := time.Now()
	c.now = func() time.Time { return now }
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))

	for _, status := range []int{403, 403, 429} {
		mx.queryErr = &azcore.ResponseError{StatusCode: status, ErrorCode: "ExampleCode"}
		now = now.Add(time.Hour)
		_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
		require.ErrorContains(t, err, "all Azure Monitor batch queries failed")
	}
	for _, want := range []string{"profile=storage_accounts region=eastus", "profile=front_door region=global", "resources=1 class=http_403", "resources=1 class=http_429"} {
		assert.Equal(t, 2, strings.Count(logs.String(), want), want)
	}
	assert.Contains(t, logs.String(), "ExampleCode")

	// Secret-reference jobs attach a sanitizer to this same logger.
	c.Logger = c.Logger.WithMessageSanitizer(func(string, []any) string { return "details redacted" })
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })
	logs.Reset()
	mx.queryErr = context.DeadlineExceeded
	now = now.Add(time.Hour)
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.ErrorContains(t, err, "all Azure Monitor batch queries failed")
	assert.Contains(t, logs.String(), "details redacted")
	assert.NotContains(t, logs.String(), "storage-example")
	assert.NotContains(t, logs.String(), "profile=")
}
