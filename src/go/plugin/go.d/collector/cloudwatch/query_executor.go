// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/sourcegraph/conc/pool"
)

type queryBatch struct {
	key     queryBatchKey
	client  cloudwatchClient
	queries []plannedQuery
	start   time.Time
	end     time.Time
}

type queryBatchResult struct {
	batch       queryBatch
	outcomes    map[string]queryOutcome
	issues      []queryResultIssue
	completedAt time.Time
	err         error
}

type queryExecution struct {
	outcomes  map[string]queryOutcome
	terminal  int
	transient int
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

func (c *Collector) executeQueries(ctx context.Context, due []plannedQuery, now time.Time) queryExecution {
	execution := queryExecution{outcomes: make(map[string]queryOutcome, len(due))}
	if len(due) == 0 {
		return execution
	}

	clients, clientErrs := c.resolveQueryClients(ctx, due)
	clientResolutionAt := c.now()
	var failures []operationFailure
	for key, err := range clientErrs {
		failures = append(failures, operationFailure{Target: key.target, Region: key.region, Err: err})
	}
	c.warnOperationFailures(logKeyQueryClientFailed, "query client creation", "", failures)
	for _, query := range due {
		if _, failed := clientErrs[clientKey{target: query.target, region: query.region}]; !failed {
			continue
		}
		start, end := queryWindow(now, query.policy)
		execution.outcomes[query.key] = queryOutcome{
			kind: queryOutcomeTransient, windowStart: start, windowEnd: end, completedAt: clientResolutionAt,
		}
	}

	batches := buildQueryBatches(due, clients, now)
	results := c.runQueryBatches(ctx, batches)
	var issues []queryResultIssue
	for _, result := range results {
		issues = append(issues, result.issues...)
		if result.err != nil {
			failures = append(failures, operationFailure{
				Target: result.batch.key.target, Region: result.batch.key.region,
				Scope: fmt.Sprintf("period %s", result.batch.key.policy.period), Err: result.err,
			})
		}
		for key, outcome := range result.outcomes {
			outcome.completedAt = result.completedAt
			execution.outcomes[key] = outcome
		}
	}
	c.warnOperationFailures(logKeyGetMetricDataFailed, "GetMetricData", "", failures[len(clientErrs):])
	c.warnQueryResultIssues(issues)
	for _, outcome := range execution.outcomes {
		if outcome.kind == queryOutcomeTransient {
			execution.transient++
		} else {
			execution.terminal++
		}
	}
	return execution
}

func (c *Collector) resolveQueryClients(ctx context.Context, queries []plannedQuery) (map[clientKey]cloudwatchClient, map[clientKey]error) {
	clients := make(map[clientKey]cloudwatchClient)
	errs := make(map[clientKey]error)
	for _, query := range queries {
		ck := clientKey{target: query.target, region: query.region}
		if _, ok := clients[ck]; ok {
			continue
		}
		if _, bad := errs[ck]; bad {
			continue
		}
		client, err := c.clients.forTargetRegion(ctx, query.target, query.region)
		if err != nil {
			errs[ck] = err
			continue
		}
		clients[ck] = client
	}
	return clients, errs
}

func buildQueryBatches(queries []plannedQuery, clients map[clientKey]cloudwatchClient, now time.Time) []queryBatch {
	type group struct {
		key     queryBatchKey
		queries []plannedQuery
	}
	index := make(map[queryBatchKey]int)
	var groups []group
	for _, query := range queries {
		key := query.batchKey()
		if _, ok := clients[clientKey{target: key.target, region: key.region}]; !ok {
			continue
		}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, group{key: key})
		}
		groups[i].queries = append(groups[i].queries, query)
	}

	var batches []queryBatch
	for _, group := range groups {
		client := clients[clientKey{target: group.key.target, region: group.key.region}]
		start, end := queryWindow(now, group.key.policy)
		for _, chunk := range packQueryGroup(group.queries, queryBatchWidth(group.key.policy)) {
			batches = append(batches, queryBatch{key: group.key, client: client, queries: chunk, start: start, end: end})
		}
	}
	return batches
}

func (c *Collector) runQueryBatches(ctx context.Context, batches []queryBatch) []queryBatchResult {
	p := pool.NewWithResults[queryBatchResult]().WithMaxGoroutines(apiConcurrency)
	for _, batch := range batches {
		p.Go(func() queryBatchResult {
			cctx, cancel := withTimeout(ctx, c.Timeout.Duration())
			defer cancel()
			outcomes, issues, err := runGetMetricData(cctx, batch)
			return queryBatchResult{batch: batch, outcomes: outcomes, issues: issues, completedAt: c.now(), err: err}
		})
	}
	return p.Wait()
}
