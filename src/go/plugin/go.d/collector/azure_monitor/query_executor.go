// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"math/bits"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
)

type queryExecutor struct {
	maxConcurrency int
	timeout        time.Duration
	cloudCfg       azcloud.Configuration
	credential     azcore.TokenCredential
	newClient      func(endpoint string, cred azcore.TokenCredential, cloud azcloud.Configuration) (metricsQueryClient, error)

	lookupDefinitions func(context.Context, definitionKey) (metricDefinitions, error)

	clientsMu sync.Mutex
	clients   map[string]metricsQueryClient
}

func newQueryExecutor(maxConcurrency int, timeout time.Duration, credential azcore.TokenCredential, cloudCfg azcloud.Configuration, newClient func(endpoint string, cred azcore.TokenCredential, cloud azcloud.Configuration) (metricsQueryClient, error)) *queryExecutor {
	return &queryExecutor{
		maxConcurrency: maxConcurrency,
		timeout:        timeout,
		cloudCfg:       cloudCfg,
		credential:     credential,
		newClient:      newClient,
		clients:        make(map[string]metricsQueryClient),
	}
}

func (e *queryExecutor) reset() {
	e.clientsMu.Lock()
	defer e.clientsMu.Unlock()
	e.clients = make(map[string]metricsQueryClient)
}

// The coordinator handles throttling between dispatches. Preserve azcore's
// network and other transient HTTP retries for ordinary requests.
func retryPrimaryMetrics(resp *http.Response, err error) bool {
	if resp != nil && throttleStatus(resp.StatusCode) {
		return false
	}
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (e *queryExecutor) executeQueryBatch(ctx context.Context, batch queryBatch, queryNow time.Time, queryOffsetSeconds int, debug bool) queryBatchResult {
	if err := ctx.Err(); err != nil {
		return queryBatchResult{Err: err}
	}
	client, err := e.getMetricsClient(batch.Region)
	if err != nil {
		return queryBatchResult{Err: err}
	}

	resourceIDs, resourceByID := queryBatchResourceIndex(batch.Resources)
	startTime, endTime, interval, aggregation := queryBatchWindow(batch, queryNow, queryOffsetSeconds)

	reqCtx, cancel := withOptionalTimeout(ctx, e.timeout)
	defer cancel()
	reqCtx, op := queryOperationContext(reqCtx)
	op.deadline, _ = reqCtx.Deadline()

	resp, err := client.QueryResources(
		reqCtx,
		batch.SubscriptionID,
		batch.Profile.MetricNamespace,
		batch.MetricNames,
		azmetrics.ResourceIDList{ResourceIDs: resourceIDs},
		&azmetrics.QueryResourcesOptions{
			StartTime:   &startTime,
			EndTime:     &endTime,
			Interval:    &interval,
			Aggregation: &aggregation,
		},
	)
	if err != nil {
		return queryBatchResult{Err: preserveCapturedThrottle(err, op.captured)}
	}

	result := reduceQueryResponse(resp.Values, batch.Profile, queryBatchMetricIndex(batch), resourceByID)
	if debug {
		result.Diagnostics = summarizeBatchResponse(batch.Resources, resourceByID, resp.Values, result.Samples)
	}
	return result
}

type queryMetricMatch struct {
	series *metricRuntime
	bit    uint32
}

func queryBatchMetricIndex(batch queryBatch) map[string]queryMetricMatch {
	result := make(map[string]queryMetricMatch, bits.OnesCount32(batch.mask))
	for remaining := batch.mask; remaining != 0; remaining &= remaining - 1 {
		index := bits.TrailingZeros32(remaining)
		series := batch.chunk.metrics[index]
		result[stringsLowerTrim(series.AzureName)] = queryMetricMatch{series: series, bit: 1 << index}
	}
	return result
}

func queryBatchResourceIndex(resources []resourceInfo) ([]string, map[string]resourceInfo) {
	resourceIDs := make([]string, 0, len(resources))
	resourceByID := make(map[string]resourceInfo, len(resources))
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
		resourceByID[resource.idKey] = resource
	}
	return resourceIDs, resourceByID
}

func queryBatchWindow(batch queryBatch, queryNow time.Time, queryOffsetSeconds int) (string, string, string, string) {
	queryEnd := queryEndForBatch(queryNow, queryOffsetSeconds, batch.TimeGrainEvery)
	start := queryEnd.Add(-batch.TimeGrainEvery).UTC().Format(time.RFC3339)
	end := queryEnd.UTC().Format(time.RFC3339)
	return start, end, batch.TimeGrain, batch.Aggregation
}

func queryEndForBatch(now time.Time, queryOffsetSeconds int, batchTimeGrainEvery time.Duration) time.Time {
	offset := effectiveQueryOffset(queryOffsetSeconds, batchTimeGrainEvery)
	queryEnd := now.Add(-offset)
	if queryEnd.IsZero() {
		return now
	}
	return queryEnd
}

func effectiveQueryOffset(queryOffsetSeconds int, batchTimeGrainEvery time.Duration) time.Duration {
	offset := secondsToDuration(queryOffsetSeconds)
	if batchTimeGrainEvery > offset {
		return batchTimeGrainEvery
	}
	return offset
}

func reduceQueryResponse(metricData []azmetrics.MetricData, profile *profileRuntime, metrics map[string]queryMetricMatch, resourceByID map[string]resourceInfo) queryBatchResult {
	result := queryBatchResult{Samples: make([]metricSample, 0, len(metricData))}
	for _, data := range metricData {
		id := stringsLowerTrim(derefOrZero(data.ResourceID))
		resource, ok := resourceByID[id]
		if !ok {
			continue
		}
		identity := resourceIdentityFor(resource, profile.Name)
		for _, metric := range data.Values {
			if metric.Name == nil {
				continue
			}
			match, ok := metrics[stringsLowerTrim(derefOrZero(metric.Name.Value))]
			if !ok {
				continue
			}
			if code := metricErrorCode(metric); code != "" {
				if result.metricErrors == nil {
					result.metricErrors = make(map[string]uint32)
				}
				result.metricErrors[id] |= match.bit
				if strings.EqualFold(code, "InvalidSamplingType") {
					if result.incompatible == nil {
						result.incompatible = make(map[string]uint32)
					}
					result.incompatible[id] |= match.bit
				}
				continue
			}
			value, found := sumLatestAggregate(metric.TimeSeries, match.series.Aggregation)
			if found {
				result.Samples = append(result.Samples, metricSample{
					Instrument: match.series.Instrument,
					Resource:   identity,
					Value:      value,
				})
			}
		}
	}
	if len(result.metricErrors) != 0 && len(result.Samples) != 0 {
		discardMeasuredErrors(&result, metricData, metrics)
	}
	return result
}

// If the service sends conflicting rows, a valid observation takes precedence.
func discardMeasuredErrors(result *queryBatchResult, data []azmetrics.MetricData, metrics map[string]queryMetricMatch) {
	for _, row := range data {
		id := stringsLowerTrim(derefOrZero(row.ResourceID))
		errorMask := result.metricErrors[id]
		if errorMask == 0 {
			continue
		}
		var measured uint32
		for _, metric := range row.Values {
			if metric.Name == nil || metricErrorCode(metric) != "" {
				continue
			}
			match := metrics[stringsLowerTrim(derefOrZero(metric.Name.Value))]
			if match.bit&errorMask == 0 {
				continue
			}
			if _, found := sumLatestAggregate(metric.TimeSeries, match.series.Aggregation); found {
				measured |= match.bit
			}
		}
		if measured == 0 {
			continue
		}
		if mask := errorMask &^ measured; mask == 0 {
			delete(result.metricErrors, id)
		} else {
			result.metricErrors[id] = mask
		}
		if mask := result.incompatible[id] &^ measured; mask == 0 {
			delete(result.incompatible, id)
		} else {
			result.incompatible[id] = mask
		}
	}
}

// Azure uses BadRequest for both metric incompatibility and request-wide
// validation failures. Only the ambiguous responses get bounded isolation.
func rejectedMetrics(err error) bool {
	var response *azcore.ResponseError
	if !errors.As(err, &response) || response.StatusCode != http.StatusBadRequest {
		return false
	}
	switch stringsLowerTrim(response.ErrorCode) {
	case "invalidapiversionparameter", "missingapiversionparameter", "invalidsubscriptionid",
		"subscriptionnotfound", "invalidtimerange", "invalidstarttime", "invalidendtime":
		return false
	}
	if response.RawResponse == nil || response.RawResponse.Body == nil {
		return true
	}
	var envelope struct {
		Error struct {
			Target  string
			Message string
		}
	}
	if err := runtime.UnmarshalAsJSON(response.RawResponse, &envelope); err != nil {
		return true
	}
	switch stringsLowerTrim(envelope.Error.Target) {
	case "api-version", "subscriptionid", "starttime", "endtime", "timespan":
		return false
	}
	// The Metrics Batch REST examples use an unqualified BadRequest for both
	// malformed time parameters and windows outside the retention period.
	message := stringsLowerTrim(envelope.Error.Message)
	return !strings.HasPrefix(message, "detected invalid time range parameters") &&
		!(strings.HasPrefix(message, "query endtime:") && strings.Contains(message, "out of the max metrics retention period"))
}

func (e *queryExecutor) getMetricsClient(region string) (metricsQueryClient, error) {
	endpoint, err := metricsEndpoint(e.cloudCfg, region)
	if err != nil {
		return nil, err
	}

	e.clientsMu.Lock()
	defer e.clientsMu.Unlock()

	if client, ok := e.clients[endpoint]; ok {
		return client, nil
	}

	client, err := e.newClient(endpoint, e.credential, e.cloudCfg)
	if err != nil {
		return nil, err
	}
	e.clients[endpoint] = client
	return client, nil
}

func metricsEndpoint(cfg azcloud.Configuration, region string) (string, error) {
	svc, ok := cfg.Services[azmetrics.ServiceName]
	if !ok {
		return "", errors.New("Azure cloud configuration does not include azmetrics service")
	}
	audience := stringsTrim(svc.Audience)
	if audience == "" {
		return "", errors.New("Azure cloud configuration has empty azmetrics audience")
	}
	u, err := url.Parse(audience)
	if err != nil {
		return "", err
	}

	return "https://" + normalizeRegion(region) + "." + u.Host, nil
}

func normalizeRegion(region string) string {
	region = stringsLowerTrim(region)
	if region == "" {
		return "global"
	}
	for i := 0; i < len(region); i++ {
		c := region[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return "global"
	}
	return region
}

func sumLatestAggregate(series []azmetrics.TimeSeriesElement, aggregation string) (float64, bool) {
	var (
		total float64
		found bool
	)
	for _, s := range series {
		value, ok := latestAggregateValue(s.Data, aggregation)
		if !ok {
			continue
		}
		total += value
		found = true
	}
	return total, found
}

func latestAggregateValue(values []azmetrics.MetricValue, aggregation string) (float64, bool) {
	var (
		bestTime time.Time
		bestVal  float64
		hasVal   bool
	)
	for _, value := range values {
		current, ok := metricValueForAggregation(value, aggregation)
		if !ok {
			continue
		}
		timeValue := derefOrZero(value.TimeStamp)
		if !hasVal || timeValue.After(bestTime) {
			bestTime = timeValue
			bestVal = current
			hasVal = true
		}
	}
	return bestVal, hasVal
}

func metricValueForAggregation(v azmetrics.MetricValue, aggregation string) (float64, bool) {
	switch aggregation {
	case "average":
		if v.Average == nil {
			return 0, false
		}
		return *v.Average, true
	case "minimum":
		if v.Minimum == nil {
			return 0, false
		}
		return *v.Minimum, true
	case "maximum":
		if v.Maximum == nil {
			return 0, false
		}
		return *v.Maximum, true
	case "total":
		if v.Total == nil {
			return 0, false
		}
		return *v.Total, true
	case "count":
		if v.Count == nil {
			return 0, false
		}
		return *v.Count, true
	default:
		return 0, false
	}
}

func derefOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
