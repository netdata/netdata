// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type responseQueryState struct {
	query        plannedQuery
	value        float64
	datapointAt  time.Time
	hasCandidate bool
	complete     bool
	forbidden    bool
}

func runGetMetricData(ctx context.Context, batch queryBatch) (map[string]queryOutcome, []queryResultIssue, error) {
	requestQueries := make([]cwtypes.MetricDataQuery, len(batch.queries))
	byID := make(map[string]*responseQueryState, len(batch.queries))
	for i, query := range batch.queries {
		id := fmt.Sprintf("q%d", i)
		requestQueries[i] = query.query
		requestQueries[i].Id = aws.String(id)
		byID[id] = &responseQueryState{query: query}
	}

	issueCounts := make(map[queryResultIssue]int)
	var nextToken *string
	for page := 0; page < maxGetMetricDataPages; page++ {
		out, err := batch.client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
			MetricDataQueries: requestQueries,
			StartTime:         aws.Time(batch.start),
			EndTime:           aws.Time(batch.end),
			ScanBy:            cwtypes.ScanByTimestampDescending,
			NextToken:         nextToken,
			MaxDatapoints:     aws.Int32(int32(len(batch.queries) * batch.key.policy.bucketCount())),
		})
		if err != nil {
			return completedOutcomes(byID, batch), queryResultIssues(issueCounts), err
		}

		for _, result := range out.MetricDataResults {
			state := byID[aws.ToString(result.Id)]
			if state == nil || state.complete || state.forbidden {
				continue
			}
			accumulateCandidate(state, result.Values, result.Timestamps, batch.start, batch.end)
			switch result.StatusCode {
			case cwtypes.StatusCodeComplete:
				state.complete = true
			case cwtypes.StatusCodeForbidden:
				state.forbidden = true
				issueCounts[queryResultIssue{
					target: state.query.target, region: state.query.region, namespace: queryNamespace(state.query.query),
					period: int(state.query.policy.period / time.Second), status: result.StatusCode,
				}]++
			}
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}
	return completedOutcomes(byID, batch), queryResultIssues(issueCounts), nil
}

func accumulateCandidate(state *responseQueryState, values []float64, timestamps []time.Time, start, end time.Time) {
	for i := 0; i < min(len(values), len(timestamps)); i++ {
		value, timestamp := values[i], timestamps[i]
		if math.IsNaN(value) || math.IsInf(value, 0) || timestamp.Before(start) || timestamp.Add(state.query.policy.period).After(end) {
			continue
		}
		if !state.hasCandidate || !timestamp.Before(state.datapointAt) {
			state.value = value
			state.datapointAt = timestamp
			state.hasCandidate = true
		}
	}
}

func completedOutcomes(states map[string]*responseQueryState, batch queryBatch) map[string]queryOutcome {
	outcomes := make(map[string]queryOutcome)
	for _, state := range states {
		kind := queryOutcomeTransient
		switch {
		case state.forbidden:
			kind = queryOutcomeForbidden
		case state.complete:
			kind = queryOutcomeComplete
		default:
			continue
		}
		outcomes[state.query.key] = queryOutcome{
			kind: kind, windowStart: batch.start, windowEnd: batch.end,
			datapointAt: state.datapointAt, value: state.value, hasDatapoint: state.hasCandidate,
		}
	}
	return outcomes
}
