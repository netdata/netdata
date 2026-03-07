// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var labelKeys = []string{"resource_uid", "resource_name", "resource_group", "region", "resource_type", "profile"}

func (c *Collector) initInstruments(plan *runtimePlan) error {
	if plan == nil {
		return errors.New("nil runtime plan")
	}

	vec := c.store.Write().SnapshotMeter("").Vec(labelKeys...)
	if plan.Instruments == nil {
		plan.Instruments = make(map[string]metrix.SnapshotGaugeVec)
	}

	for _, p := range plan.Profiles {
		for _, m := range p.Metrics {
			for _, agg := range m.Aggregations {
				name := m.InstrumentByAgg[agg]
				if _, ok := plan.Instruments[name]; ok {
					continue
				}
				plan.Instruments[name] = vec.Gauge(name)
			}
		}
	}

	return nil
}

func (c *Collector) collect(ctx context.Context) error {
	resources, err := c.refreshDiscovery(ctx, false)
	if err != nil {
		return fmt.Errorf("resource discovery: %w", err)
	}
	if len(resources) == 0 {
		return nil
	}

	now := c.now()
	tasks := c.buildQueryTasks(resources, now)

	// Re-observe cached values for series not updated this cycle.
	// This prevents the metrix store from evicting series that belong
	// to non-due time grains (e.g., PT30M/PT1H collected less frequently).
	observedThisCycle := make(map[string]bool, len(c.lastObserved))

	if len(tasks) > 0 {
		queryEnd := now.Add(-secondsToDuration(c.QueryOffset))
		if queryEnd.IsZero() {
			queryEnd = now
		}

		results := c.runTasks(ctx, tasks, queryEnd)

		var (
			allSamples []metricSample
			errCount   int
		)
		for _, res := range results {
			if res.Err != nil {
				errCount++
				c.Warningf("collection task failed: %v", res.Err)
				continue
			}
			allSamples = append(allSamples, res.Samples...)
		}

		if errCount == len(results) {
			return errors.New("all Azure Monitor batch queries failed")
		}

		for _, sample := range allSamples {
			vec, ok := c.runtimePlan.Instruments[sample.Instrument]
			if !ok {
				continue
			}
			values := labelValues(sample.Labels)

			value := sample.Value
			if sample.Accumulate {
				key := sample.Instrument + "\x00" + strings.Join(values, "\x00")
				c.accumulators[key] += value
				value = c.accumulators[key]
			}

			vec.WithLabelValues(values...).Observe(value)

			obsKey := sample.Instrument + "\x00" + strings.Join(values, "\x00")
			c.lastObserved[obsKey] = lastObservation{
				instrument:  sample.Instrument,
				labelValues: append([]string(nil), values...),
				value:       value,
			}
			observedThisCycle[obsKey] = true
		}
	}

	for key, obs := range c.lastObserved {
		if observedThisCycle[key] {
			continue
		}
		vec, ok := c.runtimePlan.Instruments[obs.instrument]
		if !ok {
			continue
		}
		vec.WithLabelValues(obs.labelValues...).Observe(obs.value)
	}

	return nil
}

func (c *Collector) buildQueryTasks(resources []resourceInfo, now time.Time) []queryTask {
	dueByGrain := make(map[string]bool)
	resourcesByType := c.discovery.ByType
	if len(resourcesByType) == 0 {
		resourcesByType = make(map[string][]resourceInfo)
		for _, r := range resources {
			resourcesByType[stringsLowerTrim(r.Type)] = append(resourcesByType[stringsLowerTrim(r.Type)], r)
		}
	}

	var tasks []queryTask

	for _, profile := range c.runtimePlan.Profiles {
		pool := resourcesByType[stringsLowerTrim(profile.ResourceType)]
		if len(pool) == 0 {
			continue
		}

		metricsByGrain := map[string][]*metricRuntime{}
		for _, metric := range profile.Metrics {
			due, ok := dueByGrain[metric.TimeGrain]
			if !ok {
				next, hasNext := c.nextCollectByGrain[metric.TimeGrain]
				due = !hasNext || !now.Before(next)
				dueByGrain[metric.TimeGrain] = due
				if due {
					c.nextCollectByGrain[metric.TimeGrain] = now.Add(metric.TimeGrainEvery)
				}
			}
			if !due {
				continue
			}
			metricsByGrain[metric.TimeGrain] = append(metricsByGrain[metric.TimeGrain], metric)
		}

		for grain, metrics := range metricsByGrain {
			if len(metrics) == 0 {
				continue
			}

			regions := groupResourcesByRegion(pool)
			metricChunks := chunkMetrics(metrics, c.MaxMetricsPerQuery)
			for region, regionResources := range regions {
				resourceChunks := chunkResources(regionResources, c.MaxBatchResources)
				for _, mChunk := range metricChunks {
					names := make([]string, 0, len(mChunk))
					aggs := map[string]struct{}{}
					for _, m := range mChunk {
						names = append(names, m.Name)
						for _, agg := range m.Aggregations {
							aggs[agg] = struct{}{}
						}
					}
					aggList := make([]string, 0, len(aggs))
					for agg := range aggs {
						aggList = append(aggList, agg)
					}
					sort.Strings(aggList)

					for _, rChunk := range resourceChunks {
						tasks = append(tasks, queryTask{
							Profile:        profile,
							MetricSubset:   mChunk,
							MetricNames:    append([]string(nil), names...),
							Aggregations:   append([]string(nil), aggList...),
							TimeGrain:      grain,
							TimeGrainEvery: supportedTimeGrains[grain],
							Region:         region,
							Resources:      append([]resourceInfo(nil), rChunk...),
						})
					}
				}
			}
		}
	}

	return tasks
}

func (c *Collector) runTasks(ctx context.Context, tasks []queryTask, queryEnd time.Time) []taskResult {
	workers := c.MaxConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	input := make(chan queryTask)
	output := make(chan taskResult, len(tasks))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			for task := range input {
				samples, err := c.executeTask(ctx, task, queryEnd)
				output <- taskResult{Samples: samples, Err: err}
			}
		})
	}

	for _, task := range tasks {
		input <- task
	}
	close(input)
	wg.Wait()
	close(output)

	results := make([]taskResult, 0, len(tasks))
	for r := range output {
		results = append(results, r)
	}
	return results
}

func (c *Collector) executeTask(ctx context.Context, task queryTask, queryEnd time.Time) ([]metricSample, error) {
	client, err := c.getMetricsClient(task.Region)
	if err != nil {
		return nil, err
	}

	metricToRuntime := make(map[string]*metricRuntime, len(task.MetricSubset))
	for _, m := range task.MetricSubset {
		metricToRuntime[stringsLowerTrim(m.Name)] = m
	}

	resourceIDs := make([]string, 0, len(task.Resources))
	resourceByID := make(map[string]resourceInfo, len(task.Resources))
	for _, r := range task.Resources {
		resourceIDs = append(resourceIDs, r.ID)
		resourceByID[stringsLowerTrim(r.ID)] = r
	}

	start := queryEnd.Add(-task.TimeGrainEvery).UTC().Format(time.RFC3339)
	end := queryEnd.UTC().Format(time.RFC3339)
	interval := task.TimeGrain
	agg := strings.Join(task.Aggregations, ",")

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.QueryResources(
		reqCtx,
		c.SubscriptionID,
		task.Profile.MetricNamespace,
		task.MetricNames,
		azmetrics.ResourceIDList{ResourceIDs: resourceIDs},
		&azmetrics.QueryResourcesOptions{
			StartTime:   &start,
			EndTime:     &end,
			Interval:    &interval,
			Aggregation: &agg,
		},
	)
	if err != nil {
		return nil, err
	}

	samples := make([]metricSample, 0, len(task.Resources)*len(task.MetricSubset))
	for _, metricData := range resp.Values {
		resourceID := stringsLowerTrim(ptrToString(metricData.ResourceID))
		resource, ok := resourceByID[resourceID]
		if !ok {
			continue
		}

		labels := metrix.Labels{
			"resource_uid":   resource.UID,
			"resource_name":  resource.Name,
			"resource_group": resource.ResourceGroup,
			"region":         resource.Region,
			"resource_type":  resource.Type,
			"profile":        task.Profile.Name,
		}

		for _, metric := range metricData.Values {
			metricName := stringsLowerTrim(ptrToString(metric.Name.Value))
			runtimeMetric, ok := metricToRuntime[metricName]
			if !ok {
				continue
			}
			for _, aggName := range runtimeMetric.Aggregations {
				value, found := sumLatestAggregate(metric.TimeSeries, aggName)
				if !found {
					continue
				}
				samples = append(samples, metricSample{
					Instrument: runtimeMetric.InstrumentByAgg[aggName],
					Labels:     labels,
					Value:      value,
					Accumulate: runtimeMetric.AccumulateAgg[aggName],
				})
			}
		}
	}

	return samples, nil
}

func (c *Collector) getMetricsClient(region string) (metricsQueryClient, error) {
	endpoint, err := metricsEndpoint(c.cloudCfg, region)
	if err != nil {
		return nil, err
	}

	c.metricsClientsMu.Lock()
	defer c.metricsClientsMu.Unlock()

	if client, ok := c.metricsClients[endpoint]; ok {
		return client, nil
	}

	client, err := c.newMetricsClient(endpoint, c.credential, c.cloudCfg)
	if err != nil {
		return nil, err
	}
	c.metricsClients[endpoint] = client
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

	r := normalizeRegion(region)
	return "https://" + r + "." + u.Host, nil
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
	for _, v := range values {
		current, ok := metricValueForAggregation(v, aggregation)
		if !ok {
			continue
		}
		timeValue := ptrTime(v.TimeStamp)
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

func ptrToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptrTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}

func labelValues(labels metrix.Labels) []string {
	return []string{
		labels["resource_uid"],
		labels["resource_name"],
		labels["resource_group"],
		labels["region"],
		labels["resource_type"],
		labels["profile"],
	}
}

func groupResourcesByRegion(resources []resourceInfo) map[string][]resourceInfo {
	result := make(map[string][]resourceInfo)
	for _, r := range resources {
		region := normalizeRegion(r.Region)
		result[region] = append(result[region], r)
	}
	return result
}

func chunkResources(resources []resourceInfo, chunkSize int) [][]resourceInfo {
	if chunkSize <= 0 || len(resources) <= chunkSize {
		return [][]resourceInfo{resources}
	}
	chunks := make([][]resourceInfo, 0, (len(resources)+chunkSize-1)/chunkSize)
	for i := 0; i < len(resources); i += chunkSize {
		end := i + chunkSize
		if end > len(resources) {
			end = len(resources)
		}
		chunks = append(chunks, resources[i:end])
	}
	return chunks
}

func chunkMetrics(metrics []*metricRuntime, chunkSize int) [][]*metricRuntime {
	if chunkSize <= 0 || len(metrics) <= chunkSize {
		return [][]*metricRuntime{metrics}
	}
	chunks := make([][]*metricRuntime, 0, (len(metrics)+chunkSize-1)/chunkSize)
	for i := 0; i < len(metrics); i += chunkSize {
		end := i + chunkSize
		if end > len(metrics) {
			end = len(metrics)
		}
		chunks = append(chunks, metrics[i:end])
	}
	return chunks
}

func secondsToDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
