// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/logger"
)

type batchDiagnostics struct {
	withSamples, empty, missing, unmatchedRows, metricErrors int
	emptyExamples, missingExamples, errorExamples            []string
}

// Summarize what this request produced before cached observations are replayed.
func summarizeBatchResponse(requested []resourceInfo, byID map[string]resourceInfo, response []azmetrics.MetricData, samples []metricSample) *batchDiagnostics {
	var d batchDiagnostics
	hasSamples := make(map[string]bool, len(requested))
	for _, row := range response {
		resource, ok := byID[stringsLowerTrim(derefOrZero(row.ResourceID))]
		if ok {
			hasSamples[resource.UID] = false
		} else {
			d.unmatchedRows++
		}
		for _, metric := range row.Values {
			d.observeMetricError(resource, ok, metric)
		}
	}
	for _, sample := range samples {
		hasSamples[sample.Resource.UID] = true
	}
	for _, resource := range requested {
		has, present := hasSamples[resource.UID]
		switch {
		case has:
			d.withSamples++
		case present:
			d.empty++
			if len(d.emptyExamples) < diagnosticExampleLimit {
				d.emptyExamples = append(d.emptyExamples, resource.diagnosticName())
			}
		default:
			d.missing++
			if len(d.missingExamples) < diagnosticExampleLimit {
				d.missingExamples = append(d.missingExamples, resource.diagnosticName())
			}
		}
	}
	return &d
}

func (d *batchDiagnostics) observeMetricError(resource resourceInfo, matched bool, metric azmetrics.Metric) {
	code := derefOrZero(metric.ErrorCode)
	if strings.EqualFold(code, "Success") {
		return
	}
	if code == "" {
		if stringsTrim(derefOrZero(metric.ErrorMessage)) == "" {
			return
		}
		code = "<unspecified>"
	}
	d.metricErrors++
	if len(d.errorExamples) >= diagnosticExampleLimit {
		return
	}
	label := "<unmatched>"
	if matched {
		label = resource.diagnosticName()
	}
	name := "<unknown>"
	if metric.Name != nil && metric.Name.Value != nil {
		name = *metric.Name.Value
	}
	d.errorExamples = append(d.errorExamples, fmt.Sprintf("%s metric=%.80q code=%.80q", label, name, code))
}

func (b *queryBatch) debugRequest(now time.Time, offset int) string {
	start, end, _, aggregation := queryBatchWindow(*b, now, offset)
	// A request contains at most 20 metric names. Quote and bound each name.
	return fmt.Sprintf("start=%s end=%s metrics=%.80q aggregation=%s", start, end, b.MetricNames, aggregation)
}

func (c *Collector) logBatchFailure(batch *queryBatch, err error, queryNow time.Time) {
	class, detail := batchFailureReason(err)
	const format = "collection batch failed: profile=%s region=%s subscription=%.80q grain=%s resources=%d class=%s %s"
	if logger.Level.Enabled(slog.LevelDebug) {
		c.Debugf(format+" %s", batch.Profile.Name, batch.Region, batch.SubscriptionID, batch.TimeGrain, len(batch.Resources),
			class, detail, batch.debugRequest(queryNow, c.QueryOffset))
	}
	c.Limit("metrics:batch:"+batch.Profile.Name+":"+class, 1, time.Hour).Warningf(format,
		batch.Profile.Name, batch.Region, batch.SubscriptionID, batch.TimeGrain, len(batch.Resources), class, detail)
}

// Keep limiter keys independent of remote error strings. Azure response errors
// can include the entire response body when formatted with %v.
func batchFailureReason(err error) (class, detail string) {
	var response *azcore.ResponseError
	var auth *azidentity.AuthenticationFailedError
	var dns *net.DNSError
	var network net.Error
	switch {
	case errors.As(err, &response):
		return fmt.Sprintf("http_%d", response.StatusCode), fmt.Sprintf("azure_code=%.80q", response.ErrorCode)
	case errors.Is(err, context.Canceled):
		return "canceled", "request canceled"
	case errors.As(err, &dns):
		return "dns", "endpoint DNS lookup failed"
	case errors.As(err, &auth):
		return "authentication", "Azure credential authentication failed"
	case errors.As(err, &network):
		if network.Timeout() {
			return "timeout", "request timed out"
		}
		return "network", "network request failed"
	default:
		return "client", fmt.Sprintf("error_type=%T", err)
	}
}
