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

func recordServiceHold[K comparable](holds *map[K]serviceHold, key K, until, receivedAt, admittedAt time.Time) {
	if !until.IsZero() && !receivedAt.Before(until) {
		return
	}
	if *holds == nil {
		*holds = make(map[K]serviceHold)
	}
	hold := (*holds)[key]
	if until.IsZero() {
		// A request already in flight when a hold began cannot advance its backoff.
		if admittedAt.Before(hold.until) {
			return
		}
		hold.backoff = min(maxServiceBackoff, max(initialServiceBackoff, 2*hold.backoff))
		until = receivedAt.Add(hold.backoff)
	}
	if until.After(hold.until) {
		hold.until = until
	}
	(*holds)[key] = hold
}

func clearServiceBackoff[K comparable](holds map[K]serviceHold, key K, admittedAt time.Time) {
	if hold := holds[key]; hold.backoff != 0 && !admittedAt.Before(hold.until) {
		// Keep the expired deadline until the next poll so responses from older
		// in-flight attempts cannot recreate the streak after this success.
		hold.backoff = 0
		holds[key] = hold
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
		row, _ := h.resourceMetricHolds(batch, resource, now)
		held |= row
		batch.mask &^= row
		if batch.mask == 0 {
			break
		}
	}
	return held
}

// Return the held names and the first time any of them becomes eligible.
func (h *serviceHolds) resourceMetricHolds(batch queryBatch, resource resourceInfo, now time.Time) (uint32, time.Time) {
	key := metricHoldKey(batch, resource)
	var held uint32
	var firstExpiry time.Time
	for remaining := batch.mask; remaining != 0; remaining &= remaining - 1 {
		index := bits.TrailingZeros32(remaining)
		key.name = stringsLowerTrim(batch.chunk.metrics[index].AzureName)
		if until := h.metrics[key].until; now.Before(until) {
			held |= 1 << index
			firstExpiry = earliestDeadline(firstExpiry, until)
		}
	}
	return held, firstExpiry
}

func (h *serviceHolds) recordMetrics(batch queryBatch, receipt throttleReceipt, admittedAt time.Time) {
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
				recordServiceHold(&h.metrics, key, until, receipt.at, admittedAt)
			} else {
				clearServiceBackoff(h.metrics, key, admittedAt)
			}
		}
	}
}

func (h *serviceHolds) armHeld(wire definitionKey, subscription string, now time.Time) bool {
	return now.Before(h.arm[wire.canonical()].until) ||
		now.Before(h.arm[definitionKey{resourceID: stringsLowerTrim(subscription)}].until) ||
		now.Before(h.arm[definitionKey{}].until)
}

func (h *serviceHolds) recordARM(wire definitionKey, subscription string, receipt throttleReceipt, admittedAt time.Time) {
	resource := wire.canonical()
	sub := definitionKey{resourceID: stringsLowerTrim(subscription)}
	tenant := definitionKey{}
	if receipt.status == 0 {
		clearServiceBackoff(h.arm, resource, admittedAt)
		clearServiceBackoff(h.arm, sub, admittedAt)
		clearServiceBackoff(h.arm, tenant, admittedAt)
		return
	}

	until := retryAfterTime(receipt.headers, receipt.at)
	recordServiceHold(&h.arm, resource, until, receipt.at, admittedAt)
	// These reads share the Microsoft.Insights metricDefinitions operation.
	// ARM's resource-type quota headers can therefore apply to its other reads.
	if receipt.headers.Get("x-ms-ratelimit-remaining-subscription-reads") == "0" ||
		receipt.headers.Get("x-ms-ratelimit-remaining-subscription-resource-requests") == "0" {
		recordServiceHold(&h.arm, sub, until, receipt.at, admittedAt)
	}
	if receipt.headers.Get("x-ms-ratelimit-remaining-tenant-reads") == "0" ||
		receipt.headers.Get("x-ms-ratelimit-remaining-tenant-resource-requests") == "0" {
		recordServiceHold(&h.arm, tenant, until, receipt.at, admittedAt)
	}
}

func throttleStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == metricsBackendThrottled
}

type throttleReceipt struct {
	status  int
	headers http.Header
	at      time.Time
}

// A zero status means the error, if any, is not a throttle.
func throttleFromError(err error, at time.Time) throttleReceipt {
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
