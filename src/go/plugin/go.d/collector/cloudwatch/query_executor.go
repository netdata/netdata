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
	noData  map[string]bool // query ids with a usable result but no datapoint (zero-fill eligible)
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

// executeQueries runs the planned queries grouped by (account, region, period) — each
// group shares a CloudWatch client and a time window — chunked to the
// GetMetricData ≤500-query limit, and runs the chunks concurrently bounded by
// apiConcurrency, each bounded by the configured timeout. It returns the
// collected samples and the set of (account, region, period) groups that did NOT fully
// succeed (a region client failed, or a chunk errored), so the caller advances
// the query schedule only for groups that succeeded. An all-failed pass returns
// an error.
func (c *Collector) executeQueries(ctx context.Context, plan []plannedQuery, now time.Time) ([]querySample, map[string]bool, map[queryGroupKey]bool, error) {
	if len(plan) == 0 {
		return nil, nil, nil, nil
	}

	byID, groups := indexPlan(plan)
	groupClients, groupErrs := c.resolveGroupClients(ctx, groups)
	c.Debugf("CloudWatch query: %d planned quer(y/ies) in %d (account,region,period) group(s); %d client(s) ready, %d failed",
		len(plan), len(groups), len(groupClients), len(groupErrs))

	jobs, failedGroups := c.buildChunkJobs(groups, groupClients, groupErrs, now, metricsPerQuery)
	if len(jobs) == 0 {
		if len(groupErrs) > 0 {
			return nil, nil, nil, fmt.Errorf("CloudWatch query: no usable clients (%d account/region pair(s) failed)", len(groupErrs))
		}
		return nil, nil, nil, nil
	}

	results := c.runChunkJobs(ctx, jobs, byID)
	samples, noData, allFailed := c.collectChunkResults(results, failedGroups)
	if allFailed {
		return nil, nil, nil, errors.New("all CloudWatch GetMetricData calls failed")
	}
	return samples, noData, failedGroups, nil
}

// indexPlan indexes the plan by query Id and groups the queries by
// (account, region, period) for batched execution.
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

// resolveGroupClients builds one client per (account, region) once per pass,
// recording per-pair build errors so a failed pair is attempted only once this pass.
// forAccountRegion is concurrency-safe; resolving up front keeps client building off
// the fan-out below.
func (c *Collector) resolveGroupClients(ctx context.Context, groups map[queryGroupKey][]cwtypes.MetricDataQuery) (map[clientKey]cloudwatchClient, map[clientKey]error) {
	clients := make(map[clientKey]cloudwatchClient)
	errs := make(map[clientKey]error)
	for key := range groups {
		ck := clientKey{account: key.account, region: key.region}
		if _, ok := clients[ck]; ok {
			continue
		}
		if _, bad := errs[ck]; bad {
			continue
		}
		client, err := c.clients.forAccountRegion(ctx, key.account, key.region)
		if err != nil {
			errs[ck] = err
			continue
		}
		clients[ck] = client
	}
	return clients, errs
}

// buildChunkJobs splits each group's queries into GetMetricData-sized chunks
// paired with the group's region client and time window. A group whose region
// client failed is marked failed and skipped.
func (c *Collector) buildChunkJobs(groups map[queryGroupKey][]cwtypes.MetricDataQuery, groupClients map[clientKey]cloudwatchClient, groupErrs map[clientKey]error, now time.Time, chunkSize int) ([]chunkJob, map[queryGroupKey]bool) {
	failedGroups := make(map[queryGroupKey]bool)
	var jobs []chunkJob
	for key, queries := range groups {
		ck := clientKey{account: key.account, region: key.region}
		client, ok := groupClients[ck]
		if !ok {
			failedGroups[key] = true
			c.Limit(logKeyQueryClientFailed+":"+key.account+"/"+key.region, 1, recurringLogEvery).
				Warningf("CloudWatch query: build client for account %q region %q: %v", key.account, key.region, groupErrs[ck])
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
			samples, noData, err := c.runGetMetricData(cctx, j.client, j.chunk, j.start, j.end, byID)
			return chunkResult{key: j.key, samples: samples, noData: noData, err: err}
		})
	}
	return p.Wait()
}

// collectChunkResults aggregates successful samples and marks the groups of
// failed jobs. It reports allFailed when every job errored.
func (c *Collector) collectChunkResults(results []chunkResult, failedGroups map[queryGroupKey]bool) (samples []querySample, noData map[string]bool, allFailed bool) {
	noData = make(map[string]bool)
	failures := 0
	for _, r := range results {
		if r.err != nil {
			failures++
			failedGroups[r.key] = true
			c.Limit(logKeyGetMetricDataFailed+":"+r.key.account+"/"+r.key.region, 1, recurringLogEvery).
				Warningf("CloudWatch GetMetricData (account %q, region %q, period %ds): %v", r.key.account, r.key.region, r.key.period, r.err)
			continue
		}
		samples = append(samples, r.samples...)
		for id := range r.noData {
			noData[id] = true
		}
	}
	return samples, noData, failures == len(results)
}

// runGetMetricData issues GetMetricData for one chunk, following NextToken to
// completion, and maps each result back to its series. With ScanBy descending
// the first value seen per Id is the newest. It returns the samples plus noData:
// the query ids that got a USABLE result (Complete/PartialData) with no
// datapoint. A per-result InternalError/Forbidden (or a missing result) is NOT
// in noData, so observe gaps that series rather than zero-filling it — an
// error must not read as a real 0 for nil_as_zero metrics.
func (c *Collector) runGetMetricData(ctx context.Context, client cloudwatchClient, chunk []cwtypes.MetricDataQuery, start, end time.Time, byID map[string]plannedQuery) ([]querySample, map[string]bool, error) {
	valueByID := make(map[string]float64, len(chunk))
	usableByID := make(map[string]bool, len(chunk)) // saw a usable (non-error) result for this id
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
			return nil, nil, err
		}

		for _, r := range out.MetricDataResults {
			if r.Id == nil {
				continue
			}
			// A per-result error is NOT "no data": leave the id unusable so its
			// series gaps (not a false 0). InternalError is transient; Forbidden
			// means the IAM identity cannot read this metric, so surface it
			// (rate-limited). PartialData is normal pagination (handled by the
			// NextToken loop), so it stays usable.
			switch r.StatusCode {
			case cwtypes.StatusCodeInternalError:
				c.Debugf("CloudWatch GetMetricData result %q: InternalError (transient); skipping this period", aws.ToString(r.Id))
				continue
			case cwtypes.StatusCodeForbidden:
				c.Limit(logKeyGetMetricDataForbidden, 1, recurringLogEvery).
					Warningf("CloudWatch GetMetricData: access denied for one or more metrics (result Forbidden); verify the IAM identity is allowed cloudwatch:GetMetricData")
				continue
			}
			usableByID[*r.Id] = true
			if _, ok := valueByID[*r.Id]; ok || len(r.Values) == 0 {
				continue
			}
			if v := r.Values[0]; !math.IsNaN(v) && !math.IsInf(v, 0) {
				valueByID[*r.Id] = v // skip non-finite datapoints (treated as no datapoint)
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	samples := make([]querySample, 0, len(chunk))
	noData := make(map[string]bool)
	for _, q := range chunk {
		id := aws.ToString(q.Id)
		if value, ok := valueByID[id]; ok {
			pq := byID[id]
			samples = append(samples, querySample{seriesName: pq.seriesName, labels: pq.labels, value: value, account: pq.account, region: pq.region, period: pq.period})
			continue
		}
		if usableByID[id] {
			noData[id] = true // usable result, no datapoint -> eligible for zero-fill
		}
		// else: errored or absent id -> not eligible; observe gaps the series
	}
	return samples, noData, nil
}
