// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"errors"
	"math/bits"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

const (
	metricsBackendThrottled = 529
	initialServiceBackoff   = time.Minute
	maxServiceBackoff       = 5 * time.Minute
	serviceBackoffRetention = 2 * time.Hour
)

type heldMetric struct {
	resource                         definitionKey
	region, name, grain, aggregation string
}

// The coordinator owns holds. Discovery changes and ordinary/supplemental
// admission decisions do not affect their lifetime.
type serviceHolds struct {
	metrics map[heldMetric]serviceHold
	arm     map[definitionKey]serviceHold
}

type serviceHold struct {
	until   time.Time
	backoff time.Duration
}

func (h *serviceHolds) expire(now time.Time) {
	expireServiceHolds(h.metrics, now)
	expireServiceHolds(h.arm, now)
}

func expireServiceHolds[K comparable](holds map[K]serviceHold, now time.Time) {
	for k, hold := range holds {
		retainUntil := hold.until
		if hold.backoff != 0 {
			retainUntil = retainUntil.Add(serviceBackoffRetention)
		}
		if !now.Before(retainUntil) {
			delete(holds, k)
		}
	}
}

func recordServiceHold[K comparable](holds *map[K]serviceHold, key K, until, at time.Time) {
	if !until.IsZero() && !at.Before(until) {
		return
	}
	if *holds == nil {
		*holds = make(map[K]serviceHold)
	}
	hold := (*holds)[key]
	if until.IsZero() {
		// Multiple requests may have been in flight before this scope was held.
		// Advance backoff only on a subsequent eligible attempt.
		if at.Before(hold.until) {
			return
		}
		hold.backoff = min(maxServiceBackoff, max(initialServiceBackoff, 2*hold.backoff))
		until = at.Add(hold.backoff)
	}
	if until.After(hold.until) {
		hold.until = until
	}
	(*holds)[key] = hold
}

func clearServiceBackoff[K comparable](holds map[K]serviceHold, key K, at time.Time) {
	if hold, exists := holds[key]; exists && !at.Before(hold.until) {
		delete(holds, key)
	}
}

func metricHoldKey(batch queryBatch, resource resourceInfo) heldMetric {
	return heldMetric{
		resource: resource.definitionKey(batch.Profile),
		region:   batch.Region, grain: batch.TimeGrain, aggregation: batch.Aggregation,
	}
}

func (h *serviceHolds) hasMetrics() bool {
	return len(h.metrics) != 0
}

func (h *serviceHolds) metricMask(batch queryBatch, now time.Time) uint32 {
	if !h.hasMetrics() {
		return 0
	}
	var held uint32
	for _, resource := range batch.Resources {
		key := metricHoldKey(batch, resource)
		for remaining := batch.mask &^ held; remaining != 0; remaining &= remaining - 1 {
			index := bits.TrailingZeros32(remaining)
			key.name = stringsLowerTrim(batch.chunk.metrics[index].AzureName)
			if now.Before(h.metrics[key].until) {
				held |= 1 << index
			}
		}
	}
	return held
}

func (h *serviceHolds) recordMetrics(batch queryBatch, receipt throttleReceipt) {
	if receipt.status == 0 && !h.hasMetrics() {
		return
	}
	until := retryAfterTime(receipt.headers, receipt.at)
	// Batch Metrics has no documented broader quota header. An ARM remaining
	// quota header cannot identify the scope of a batch Metrics throttle.
	for _, resource := range batch.Resources {
		key := metricHoldKey(batch, resource)
		for remaining := batch.mask; remaining != 0; remaining &= remaining - 1 {
			key.name = stringsLowerTrim(batch.chunk.metrics[bits.TrailingZeros32(remaining)].AzureName)
			if receipt.status != 0 {
				recordServiceHold(&h.metrics, key, until, receipt.at)
			} else {
				clearServiceBackoff(h.metrics, key, receipt.at)
			}
		}
	}
}

func (h *serviceHolds) armHeld(wire definitionKey, subscription string, now time.Time) bool {
	return now.Before(h.arm[wire.canonical()].until) ||
		now.Before(h.arm[definitionKey{resourceID: stringsLowerTrim(subscription)}].until) ||
		now.Before(h.arm[definitionKey{}].until)
}

func (h *serviceHolds) recordARM(wire definitionKey, subscription string, receipt throttleReceipt) {
	resource := wire.canonical()
	sub := definitionKey{resourceID: stringsLowerTrim(subscription)}
	tenant := definitionKey{}
	if receipt.status == 0 {
		clearServiceBackoff(h.arm, resource, receipt.at)
		clearServiceBackoff(h.arm, sub, receipt.at)
		clearServiceBackoff(h.arm, tenant, receipt.at)
		return
	}

	until := retryAfterTime(receipt.headers, receipt.at)
	recordServiceHold(&h.arm, resource, until, receipt.at)
	// These reads share the Microsoft.Insights metricDefinitions operation.
	// ARM's resource-type quota headers can therefore apply to its other reads.
	if receipt.headers.Get("x-ms-ratelimit-remaining-subscription-reads") == "0" ||
		receipt.headers.Get("x-ms-ratelimit-remaining-subscription-resource-requests") == "0" {
		recordServiceHold(&h.arm, sub, until, receipt.at)
	}
	if receipt.headers.Get("x-ms-ratelimit-remaining-tenant-reads") == "0" ||
		receipt.headers.Get("x-ms-ratelimit-remaining-tenant-resource-requests") == "0" {
		recordServiceHold(&h.arm, tenant, until, receipt.at)
	}
}

func throttleStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == metricsBackendThrottled
}

func throttleFromError(err error, at time.Time) throttleReceipt {
	var captured *receivedThrottleError
	if errors.As(err, &captured) {
		return captured.receipt
	}
	var response *azcore.ResponseError
	if !errors.As(err, &response) || !throttleStatus(response.StatusCode) {
		return throttleReceipt{at: at}
	}
	receipt := throttleReceipt{status: response.StatusCode, at: at}
	if response.RawResponse == nil {
		return receipt
	}
	receipt.headers = response.RawResponse.Header
	return receipt
}

func retryAfterTime(headers http.Header, at time.Time) time.Time {
	for _, name := range []string{"retry-after-ms", "x-ms-retry-after-ms", "Retry-After"} {
		value := strings.TrimSpace(headers.Get(name))
		if value == "" {
			continue
		}
		unit := time.Millisecond
		if name == "Retry-After" {
			if date, err := http.ParseTime(value); err == nil {
				return date
			}
			unit = time.Second
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= 0 {
			return at.Add(time.Duration(min(n, int64((1<<63-1)/unit))) * unit)
		}
	}
	return time.Time{}
}
