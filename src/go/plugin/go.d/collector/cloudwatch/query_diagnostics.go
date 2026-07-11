// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type queryResultIssue struct {
	target, region, namespace string
	period                    int
	status                    cwtypes.StatusCode
	count                     int
}

func queryNamespace(query cwtypes.MetricDataQuery) string {
	if query.MetricStat == nil || query.MetricStat.Metric == nil {
		return "unknown"
	}
	if namespace := aws.ToString(query.MetricStat.Metric.Namespace); namespace != "" {
		return namespace
	}
	return "unknown"
}

func queryResultIssues(counts map[queryResultIssue]int) []queryResultIssue {
	issues := make([]queryResultIssue, 0, len(counts))
	for issue, count := range counts {
		issue.count = count
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.target != b.target {
			return a.target < b.target
		}
		if a.region != b.region {
			return a.region < b.region
		}
		if a.namespace != b.namespace {
			return a.namespace < b.namespace
		}
		if a.period != b.period {
			return a.period < b.period
		}
		return a.status < b.status
	})
	return issues
}

func (c *Collector) warnQueryResultIssues(issues []queryResultIssue) {
	if len(issues) == 0 {
		return
	}
	counts := make(map[queryResultIssue]int)
	for _, issue := range issues {
		key := issue
		key.count = 0
		counts[key] += issue.count
	}
	aggregated := queryResultIssues(counts)
	var samples []string
	total := 0
	for _, issue := range aggregated {
		total += issue.count
	}
	for _, issue := range aggregated[:min(len(aggregated), maxFailureLogSamples)] {
		samples = append(samples, fmt.Sprintf(
			"target %q region %q namespace %q period %ds: %s (%d result(s))",
			issue.target, issue.region, issue.namespace, issue.period, issue.status, issue.count,
		))
	}
	if remaining := len(aggregated) - len(samples); remaining > 0 {
		samples = append(samples, fmt.Sprintf("and %d more scope(s)", remaining))
	}
	c.Limit(logKeyGetMetricDataForbidden, 1, recurringLogEvery).Warningf(
		"CloudWatch GetMetricData returned Forbidden for %d metric result(s): %s; verify each target identity is allowed cloudwatch:GetMetricData",
		total, strings.Join(samples, "; "),
	)
}
