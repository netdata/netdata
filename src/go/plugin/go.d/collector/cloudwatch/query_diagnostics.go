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
	kind                      queryResultIssueKind
	count                     int
}

type queryResultIssueKind string

const (
	queryIssueForbidden       queryResultIssueKind = "Forbidden"
	queryIssueInternalError   queryResultIssueKind = "InternalError"
	queryIssuePartialData     queryResultIssueKind = "PartialData"
	queryIssueMissingResult   queryResultIssueKind = "missing result"
	queryIssueUnknownStatus   queryResultIssueKind = "unknown status"
	queryIssuePaginationLimit queryResultIssueKind = "pagination limit"
)

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
		return a.kind < b.kind
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
	var forbidden, transient []queryResultIssue
	for _, issue := range aggregated {
		if issue.kind == queryIssueForbidden {
			forbidden = append(forbidden, issue)
		} else {
			transient = append(transient, issue)
		}
	}
	c.warnQueryResultIssueGroup(
		logKeyGetMetricDataForbidden, forbidden,
		"CloudWatch GetMetricData returned Forbidden for %d metric result(s): %s; verify each target identity is allowed cloudwatch:GetMetricData",
	)
	c.warnQueryResultIssueGroup(
		logKeyGetMetricDataTransient, transient,
		"CloudWatch GetMetricData left %d metric result(s) unresolved: %s; retained values are replayed and retries use per-query exponential backoff",
	)
}

func (c *Collector) warnQueryResultIssueGroup(limiterKey string, issues []queryResultIssue, format string) {
	if len(issues) == 0 {
		return
	}
	var samples []string
	total := 0
	for _, issue := range issues {
		total += issue.count
	}
	for _, issue := range issues[:min(len(issues), maxFailureLogSamples)] {
		samples = append(samples, fmt.Sprintf(
			"target %q region %q namespace %q period %ds: %s (%d result(s))",
			issue.target, issue.region, issue.namespace, issue.period, issue.kind, issue.count,
		))
	}
	if remaining := len(issues) - len(samples); remaining > 0 {
		samples = append(samples, fmt.Sprintf("and %d more scope(s)", remaining))
	}
	c.Limit(limiterKey, 1, recurringLogEvery).Warningf(format, total, strings.Join(samples, "; "))
}
