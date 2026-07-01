// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/sourcegraph/conc/pool"
)

// chunkJob is one GetMetricData call: a chunk of a group's queries together with
// the region client and time window they share.
type chunkJob struct {
	key    queryGroupKey
	client cloudwatchClient
	chunk  []cwtypes.MetricDataQuery
	start  time.Time
	end    time.Time
}

// chunkResult is the outcome of one chunk job.
type chunkResult struct {
	key     queryGroupKey
	samples []querySample
	err     error
}

// withTimeout bounds one CloudWatch operation (a paginated ListMetrics sequence
// or a GetMetricData chunk) by the configured timeout. A non-positive timeout
// leaves the context unbounded. Callers must always defer the returned cancel.
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d)
}

// executeQueries runs the planned queries grouped by (region, period) — each
// group shares a CloudWatch client and a time window — chunked to the
// GetMetricData ≤500-query limit, and runs the chunks concurrently bounded by
// apiConcurrency, each bounded by the configured timeout. It returns the
// collected samples and the set of (region, period) groups that did NOT fully
// succeed (a region client failed, or a chunk errored), so the caller advances
// the query schedule only for groups that succeeded. An all-failed pass returns
// an error.
func (c *Collector) executeQueries(ctx context.Context, plan []plannedQuery, now time.Time) ([]querySample, map[queryGroupKey]bool, error) {
	if len(plan) == 0 {
		return nil, nil, nil
	}

	byID, groups := indexPlan(plan)
	regionClients, regionErrs := c.resolveRegionClients(ctx, groups)
	c.Debugf("CloudWatch query: %d planned quer(y/ies) in %d (region,period) group(s); %d region client(s) ready, %d failed",
		len(plan), len(groups), len(regionClients), len(regionErrs))

	jobs, failedGroups := c.buildChunkJobs(groups, regionClients, regionErrs, now, metricsPerQuery)
	if len(jobs) == 0 {
		if len(regionErrs) > 0 {
			return nil, nil, fmt.Errorf("CloudWatch query: no usable region clients (%d region(s) failed)", len(regionErrs))
		}
		return nil, nil, nil
	}

	results := c.runChunkJobs(ctx, jobs, byID)
	samples, allFailed := c.collectChunkResults(results, failedGroups)
	if allFailed {
		return nil, nil, errors.New("all CloudWatch GetMetricData calls failed")
	}
	return samples, failedGroups, nil
}

// indexPlan indexes the plan by query Id and groups the queries by
// (region, period) for batched execution.
func indexPlan(plan []plannedQuery) (map[string]plannedQuery, map[queryGroupKey][]cwtypes.MetricDataQuery) {
	byID := make(map[string]plannedQuery, len(plan))
	groups := make(map[queryGroupKey][]cwtypes.MetricDataQuery)
	for _, pq := range plan {
		byID[pq.id] = pq
		key := pq.groupKey()
		groups[key] = append(groups[key], pq.query)
	}
	return byID, groups
}

// resolveRegionClients builds one client per region once per pass, recording
// per-region build errors so a failed region is attempted only once this pass.
// forRegion is concurrency-safe; resolving up front keeps client building off
// the fan-out below.
func (c *Collector) resolveRegionClients(ctx context.Context, groups map[queryGroupKey][]cwtypes.MetricDataQuery) (map[string]cloudwatchClient, map[string]error) {
	regionClients := make(map[string]cloudwatchClient)
	regionErrs := make(map[string]error)
	for key := range groups {
		if _, ok := regionClients[key.region]; ok {
			continue
		}
		if _, bad := regionErrs[key.region]; bad {
			continue
		}
		client, err := c.clients.forRegion(ctx, key.region)
		if err != nil {
			regionErrs[key.region] = err
			continue
		}
		regionClients[key.region] = client
	}
	return regionClients, regionErrs
}

// buildChunkJobs splits each group's queries into GetMetricData-sized chunks
// paired with the group's region client and time window. A group whose region
// client failed is marked failed and skipped.
func (c *Collector) buildChunkJobs(groups map[queryGroupKey][]cwtypes.MetricDataQuery, regionClients map[string]cloudwatchClient, regionErrs map[string]error, now time.Time, chunkSize int) ([]chunkJob, map[queryGroupKey]bool) {
	failedGroups := make(map[queryGroupKey]bool)
	var jobs []chunkJob
	for key, queries := range groups {
		client, ok := regionClients[key.region]
		if !ok {
			failedGroups[key] = true
			c.Limit(logKeyQueryClientFailed+":"+key.region, 1, recurringLogEvery).
				Warningf("CloudWatch query: build client for region %q: %v", key.region, regionErrs[key.region])
			continue
		}
		start, end := queryWindow(now, key.period, c.QueryOffset)
		for chunk := range slices.Chunk(queries, chunkSize) {
			jobs = append(jobs, chunkJob{key: key, client: client, chunk: chunk, start: start, end: end})
		}
	}
	return jobs, failedGroups
}

// runChunkJobs executes the chunk jobs concurrently, bounded by apiConcurrency,
// each call bounded by the configured timeout. It returns one result per job.
func (c *Collector) runChunkJobs(ctx context.Context, jobs []chunkJob, byID map[string]plannedQuery) []chunkResult {
	p := pool.NewWithResults[chunkResult]().WithMaxGoroutines(apiConcurrency)
	for _, j := range jobs {
		p.Go(func() chunkResult {
			cctx, cancel := withTimeout(ctx, c.Timeout.Duration())
			defer cancel()
			samples, err := c.runGetMetricData(cctx, j.client, j.chunk, j.start, j.end, byID)
			return chunkResult{key: j.key, samples: samples, err: err}
		})
	}
	return p.Wait()
}

// collectChunkResults aggregates successful samples and marks the groups of
// failed jobs. It reports allFailed when every job errored.
func (c *Collector) collectChunkResults(results []chunkResult, failedGroups map[queryGroupKey]bool) (samples []querySample, allFailed bool) {
	failures := 0
	for _, r := range results {
		if r.err != nil {
			failures++
			failedGroups[r.key] = true
			c.Limit(logKeyGetMetricDataFailed+":"+r.key.region, 1, recurringLogEvery).
				Warningf("CloudWatch GetMetricData (region %q, period %ds): %v", r.key.region, r.key.period, r.err)
			continue
		}
		samples = append(samples, r.samples...)
	}
	return samples, failures == len(results)
}

// runGetMetricData issues GetMetricData for one chunk, following NextToken to
// completion, and maps each result back to its series. With ScanBy descending
// the first value seen per Id is the newest. Queries that return no datapoint
// are emitted as a gap.
func (c *Collector) runGetMetricData(ctx context.Context, client cloudwatchClient, chunk []cwtypes.MetricDataQuery, start, end time.Time, byID map[string]plannedQuery) ([]querySample, error) {
	valueByID := make(map[string]float64, len(chunk))
	var nextToken *string

	for {
		out, err := client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
			MetricDataQueries: chunk,
			StartTime:         aws.Time(start),
			EndTime:           aws.Time(end),
			ScanBy:            cwtypes.ScanByTimestampDescending,
			NextToken:         nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, r := range out.MetricDataResults {
			if r.Id == nil {
				continue
			}
			// Per-result statuses: skip caching an unreliable value, so the series
			// gaps this period and is retried when its group is next due.
			// InternalError is transient; Forbidden means the IAM identity cannot
			// read this metric, so surface it (rate-limited) rather than hiding a
			// permissions gap behind a debug line.
			switch r.StatusCode {
			case cwtypes.StatusCodeInternalError:
				c.Debugf("CloudWatch GetMetricData result %q: InternalError (transient); skipping this period", aws.ToString(r.Id))
				continue
			case cwtypes.StatusCodeForbidden:
				c.Limit(logKeyGetMetricDataForbidden, 1, recurringLogEvery).
					Warningf("CloudWatch GetMetricData: access denied for one or more metrics (result Forbidden); verify the IAM identity is allowed cloudwatch:GetMetricData")
				continue
			}
			if _, ok := valueByID[*r.Id]; ok || len(r.Values) == 0 {
				continue
			}
			if v := r.Values[0]; !math.IsNaN(v) && !math.IsInf(v, 0) {
				valueByID[*r.Id] = v // skip non-finite datapoints (treated as a gap)
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	samples := make([]querySample, 0, len(chunk))
	for _, q := range chunk {
		pq := byID[aws.ToString(q.Id)]
		value, ok := valueByID[aws.ToString(q.Id)]
		if !ok {
			continue // missing datapoint -> gap
		}
		samples = append(samples, querySample{seriesName: pq.seriesName, labels: pq.labels, value: value, region: pq.region, period: pq.period})
	}
	return samples, nil
}
