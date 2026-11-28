// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectedMetricsDistinguishesRequestValidationFromMetricIncompatibility(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		target     string
		message    string
		canIsolate bool
	}{
		{name: "unavailable metric", status: 400, code: "BadRequest", message: "Failed to find metric configuration for B", canIsolate: true},
		{name: "ambiguous", status: 400, code: "BadRequest", canIsolate: true},
		{name: "invalid time parameters", status: 400, code: "BadRequest", message: "Detected invalid time range parameters for API version 2023-10-01."},
		{name: "window outside retention", status: 400, code: "BadRequest", message: "Query endTime: 06/06/2021 00:00:00 is out of the Max metrics retention period: 93.00:00:00"},
		{name: "invalid subscription code", status: 400, code: "InvalidSubscriptionId"},
		{name: "request-field target", status: 400, code: "BadRequest", target: "starttime"},
		{name: "authorization", status: 403, code: "AuthorizationFailed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := azureTestResponse(nil, tc.status, map[string]any{"error": map[string]string{
				"code": tc.code, "target": tc.target, "message": tc.message,
			}})
			require.NoError(t, err)
			assert.Equal(t, tc.canIsolate, rejectedMetrics(runtime.NewResponseError(resp)))
		})
	}
}

func TestReduceQueryResponseSeparatesMetricErrorsFromEmptyAndMeasured(t *testing.T) {
	batch := testWireEvidenceBatch("A", "B", "C", "D", "E")
	res := batch.Resources[0]
	metricWithError := func(name, code, message string) azmetrics.Metric {
		metric := metricWithTotal(name, time.Now(), 100)
		metric.ErrorCode = new(code)
		metric.ErrorMessage = new(message)
		return metric
	}
	result := reduceQueryResponse([]azmetrics.MetricData{
		{ResourceID: &res.ID, Values: []azmetrics.Metric{
			metricWithTotal("A", time.Now(), 1),
			metricWithError("B", "InvalidSamplingType", "grain unsupported"),
			{Name: &azmetrics.LocalizableString{Value: new("C")}, ErrorCode: new("Success")},
		}},
		{ResourceID: &res.ID, Values: []azmetrics.Metric{
			metricWithError("A", "InvalidSamplingType", "duplicate conflicting row"),
			metricWithError("D", "AuthorizationFailed", "access denied"),
			metricWithError("E", "", "explicit failure without a code"),
		}},
	}, batch.Profile, queryBatchMetricIndex(batch), map[string]resourceInfo{res.idKey: res})
	require.Len(t, result.Samples, 1)
	assert.Equal(t, "A", result.Samples[0].Instrument)
	assert.EqualValues(t, 1, result.Samples[0].Value)
	assert.Equal(t, map[string]uint32{res.idKey: 1 << 1}, result.incompatible)
	assert.Equal(t, map[string]uint32{res.idKey: 1<<1 | 1<<3 | 1<<4}, result.metricErrors)
}

func TestServiceHoldsBackOffHeaderlessThrottlesAndMatchWireIdentity(t *testing.T) {
	batch := testWireEvidenceBatch("A", "B")
	batch.mask = 1 << 1
	var holds serviceHolds
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := start
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		holds.expire(at)
		holds.recordMetrics(batch, throttleReceipt{status: 529, at: at}, at)
		// Rediscovery and another compiled ordering leave the wire name unchanged.
		rediscovered := testWireEvidenceBatch("b", "a")
		rediscovered.Resources[0].ID = strings.ToUpper(rediscovered.Resources[0].ID)
		assert.EqualValues(t, 1, holds.metricMask(rediscovered, at.Add(delay-time.Second)))
		assert.Zero(t, holds.metricMask(rediscovered, at.Add(delay)))
		assert.False(t, holds.armHeld(batch.Resources[0].definitionKey(batch.Profile), batch.SubscriptionID, at))
		at = at.Add(delay)
	}
	holds.recordMetrics(batch, throttleReceipt{at: at}, at)
	holds.recordMetrics(batch, throttleReceipt{status: 529, at: at}, at)
	assert.Zero(t, holds.metricMask(batch, at.Add(time.Minute)), "successful retry reset the overload streak")
}

func testWireEvidenceBatch(names ...string) queryBatch {
	const namespace = "Microsoft.Cdn/profiles"
	id := "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Cdn/profiles/edge"
	profile := &profileRuntime{Name: "front_door", MetricNamespace: namespace, namespaceKey: strings.ToLower(namespace)}
	var metrics []*metricRuntime
	for _, name := range names {
		metrics = append(metrics, &metricRuntime{AzureName: name, Aggregation: "total", Instrument: name})
	}
	return queryBatch{
		chunk:          &metricChunk{metrics: metrics},
		mask:           1<<len(metrics) - 1,
		SubscriptionID: "sub-1",
		Profile:        profile,
		MetricNames:    names,
		Aggregation:    "total",
		TimeGrain:      "PT1M",
		TimeGrainEvery: time.Minute,
		Region:         "eastus",
		Resources:      []resourceInfo{{ID: id, idKey: strings.ToLower(id), UID: "edge"}},
	}
}
