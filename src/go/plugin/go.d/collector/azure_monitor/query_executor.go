// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type queryExecutor struct {
	maxConcurrency int
	timeout        time.Duration
	cloudCfg       azcloud.Configuration
	credential     azcore.TokenCredential
	newClient      func(endpoint string, cred azcore.TokenCredential, cloud azcloud.Configuration) (metricsQueryClient, error)

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

func (e *queryExecutor) runQueryBatches(ctx context.Context, batches []queryBatch, queryNow time.Time, queryOffsetSeconds int) []queryBatchResult {
	workers := min(max(e.maxConcurrency, 1), len(batches))

	input := make(chan queryBatch)
	output := make(chan queryBatchResult, len(batches))

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for batch := range input {
				samples, err := e.executeQueryBatch(ctx, batch, queryNow, queryOffsetSeconds)
				output <- queryBatchResult{Samples: samples, Err: err}
			}
		})
	}

	for _, batch := range batches {
		input <- batch
	}
	close(input)
	wg.Wait()
	close(output)

	results := make([]queryBatchResult, 0, len(batches))
	for result := range output {
		results = append(results, result)
	}
	return results
}

func (e *queryExecutor) executeQueryBatch(ctx context.Context, batch queryBatch, queryNow time.Time, queryOffsetSeconds int) ([]metricSample, error) {
	client, err := e.getMetricsClient(batch.Region)
	if err != nil {
		return nil, err
	}

	resourceIDs, resourceByID := queryBatchResourceIndex(batch.Resources)
	startTime, endTime, interval, aggregation := queryBatchWindow(batch, queryNow, queryOffsetSeconds)

	reqCtx, cancel := withOptionalTimeout(ctx, e.timeout)
	defer cancel()

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
		return nil, err
	}

	return samplesFromQueryResponse(resp.Values, batch.Profile.Name, queryBatchMetricIndex(batch.Metrics), resourceByID), nil
}

func queryBatchMetricIndex(metrics []*metricRuntime) map[string]*metricRuntime {
	result := make(map[string]*metricRuntime, len(metrics))
	for _, metric := range metrics {
		result[stringsLowerTrim(metric.AzureName)] = metric
	}
	return result
}

func queryBatchResourceIndex(resources []resourceInfo) ([]string, map[string]resourceInfo) {
	resourceIDs := make([]string, 0, len(resources))
	resourceByID := make(map[string]resourceInfo, len(resources))
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
		resourceByID[stringsLowerTrim(resource.ID)] = resource
	}
	return resourceIDs, resourceByID
}

func queryBatchWindow(batch queryBatch, queryNow time.Time, queryOffsetSeconds int) (string, string, string, string) {
	queryEnd := queryEndForBatch(queryNow, queryOffsetSeconds, batch.TimeGrainEvery)
	start := queryEnd.Add(-batch.TimeGrainEvery).UTC().Format(time.RFC3339)
	end := queryEnd.UTC().Format(time.RFC3339)
	return start, end, batch.TimeGrain, strings.Join(batch.Aggregations, ",")
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

func samplesFromQueryResponse(metricData []azmetrics.MetricData, profileName string, metricToRuntime map[string]*metricRuntime, resourceByID map[string]resourceInfo) []metricSample {
	samples := make([]metricSample, 0, len(metricData))
	for _, data := range metricData {
		resource, ok := resourceByID[stringsLowerTrim(derefOrZero(data.ResourceID))]
		if !ok {
			continue
		}
		samples = append(samples, samplesFromMetricValues(data.Values, resourceLabels(resource, profileName), metricToRuntime)...)
	}
	return samples
}

func samplesFromMetricValues(metrics []azmetrics.Metric, labels metrix.Labels, metricToRuntime map[string]*metricRuntime) []metricSample {
	samples := make([]metricSample, 0, len(metrics))
	for _, metric := range metrics {
		runtimeMetric, ok := metricToRuntime[stringsLowerTrim(derefOrZero(metric.Name.Value))]
		if !ok {
			continue
		}
		for _, series := range runtimeMetric.Series {
			value, found := sumLatestAggregate(metric.TimeSeries, series.Aggregation)
			if !found {
				continue
			}
			samples = append(samples, metricSample{
				Instrument: series.Instrument,
				Kind:       series.Kind,
				Labels:     labels,
				Value:      value,
			})
		}
	}
	return samples
}

func resourceLabels(resource resourceInfo, profileName string) metrix.Labels {
	return metrix.Labels{
		"resource_uid":    resource.UID,
		"subscription_id": resource.SubscriptionID,
		"resource_name":   resource.Name,
		"resource_group":  resource.ResourceGroup,
		"region":          resource.Region,
		"resource_type":   resource.Type,
		"profile":         profileName,
	}
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
