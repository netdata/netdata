// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"sync"
	"time"
)

const (
	maxListMetricsOperationsPerRefresh       = 100
	maxScannedMetricsPerRefresh              = 50000
	maxDiscoveryMatcherEvaluationsPerRefresh = 1000000
	maxCandidateInstancesPerRefresh          = 20000
	maxRetainedCandidateBytesPerRefresh      = 64 << 20

	// Conservative representation weights use 64-bit header sizes on every
	// architecture and include map/slice entry overhead beyond the packed value.
	retainedCandidateBaseBytes         = 128
	retainedCandidatePerDimensionBytes = 16
)

type discoveryBudget struct {
	mu sync.Mutex

	cancel context.CancelCauseFunc
	err    error

	remainingOperations    int
	scannedMetrics         int
	matcherEvaluations     int
	candidateInstances     int
	retainedCandidateBytes int
}

func newDiscoveryBudget(groupCount int, cancel context.CancelCauseFunc) (*discoveryBudget, error) {
	if groupCount > maxListMetricsOperationsPerRefresh {
		return nil, safeCollectorErrorf(
			"CloudWatch discovery defines %d groups; the %d-operation refresh budget cannot admit every group",
			groupCount, maxListMetricsOperationsPerRefresh,
		)
	}
	return &discoveryBudget{
		cancel:              cancel,
		remainingOperations: maxListMetricsOperationsPerRefresh,
	}, nil
}

func (b *discoveryBudget) reserveListMetricsOperation() error {
	b.mu.Lock()
	if b.err != nil {
		err := b.err
		b.mu.Unlock()
		return err
	}
	if b.remainingOperations == 0 {
		err := safeCollectorErrorf("CloudWatch discovery requires more than %d ListMetrics SDK operations in one refresh", maxListMetricsOperationsPerRefresh)
		b.err = err
		b.mu.Unlock()
		b.cancel(err)
		return err
	}
	b.remainingOperations--
	b.mu.Unlock()
	return nil
}

func (b *discoveryBudget) reserveScannedMetrics(count int) error {
	return b.reserveWork(count, maxScannedMetricsPerRefresh, &b.scannedMetrics,
		"CloudWatch discovery scans more than %d metrics in one refresh")
}

func (b *discoveryBudget) reserveMatcherEvaluations(count int) error {
	return b.reserveWork(count, maxDiscoveryMatcherEvaluationsPerRefresh, &b.matcherEvaluations,
		"CloudWatch discovery requires more than %d residual profile matches in one refresh")
}

func (b *discoveryBudget) reserveWork(count, limit int, used *int, format string) error {
	b.mu.Lock()
	if b.err != nil {
		err := b.err
		b.mu.Unlock()
		return err
	}
	if count < 0 || *used > limit-count {
		err := safeCollectorErrorf(format, limit)
		b.err = err
		b.mu.Unlock()
		b.cancel(err)
		return err
	}
	*used += count
	b.mu.Unlock()
	return nil
}

func (b *discoveryBudget) reserveCandidate(retainedBytes int) error {
	b.mu.Lock()
	if b.err != nil {
		err := b.err
		b.mu.Unlock()
		return err
	}
	switch {
	case b.candidateInstances == maxCandidateInstancesPerRefresh:
		err := safeCollectorErrorf("CloudWatch discovery retains more than %d candidate instances in one refresh", maxCandidateInstancesPerRefresh)
		b.err = err
		b.mu.Unlock()
		b.cancel(err)
		return err
	case retainedBytes < 0 || b.retainedCandidateBytes > maxRetainedCandidateBytesPerRefresh-retainedBytes:
		err := safeCollectorErrorf("CloudWatch discovery retains more than %d MiB of candidate instance data in one refresh", maxRetainedCandidateBytesPerRefresh>>20)
		b.err = err
		b.mu.Unlock()
		b.cancel(err)
		return err
	default:
		b.candidateInstances++
		b.retainedCandidateBytes += retainedBytes
		b.mu.Unlock()
		return nil
	}
}

func (b *discoveryBudget) failure() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func retainedCandidateBytes(packedValues string, dimensions int) int {
	return retainedCandidatePayloadBytes(len(packedValues), dimensions)
}

func retainedCandidatePayloadBytes(packedBytes, dimensions int) int {
	return packedBytes + retainedCandidateBaseBytes + dimensions*retainedCandidatePerDimensionBytes
}

func retainedDiscoveredInstanceBytes(instance discoveredInstance) int {
	packedBytes := max(0, len(instance.DimensionValues)-1) * len(instanceKeySep)
	for _, value := range instance.DimensionValues {
		packedBytes += len(value)
	}
	return retainedCandidatePayloadBytes(packedBytes, len(instance.DimensionValues))
}

type discoveryLaneKey struct {
	target string
	region string
}

type discoveryLane struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
}

type discoveryController struct {
	parent context.Context
	stage  context.Context
	cancel context.CancelCauseFunc
	stop   context.CancelFunc
	budget *discoveryBudget
	lanes  map[discoveryLaneKey]discoveryLane
}

func newDiscoveryController(parent context.Context, groups []discoveryGroup, timeout time.Duration) (*discoveryController, error) {
	deadlineCtx, stop := context.WithTimeoutCause(parent, timeout, safeCollectorError("CloudWatch discovery stage timed out"))
	stageCtx, cancel := context.WithCancelCause(deadlineCtx)
	budget, err := newDiscoveryBudget(len(groups), cancel)
	if err != nil {
		cancel(err)
		stop()
		return nil, err
	}
	c := &discoveryController{
		parent: parent, stage: stageCtx, cancel: cancel, stop: stop, budget: budget,
		lanes: make(map[discoveryLaneKey]discoveryLane),
	}
	for _, group := range groups {
		key := discoveryLaneKey{target: group.Target, region: group.Region}
		if _, ok := c.lanes[key]; ok {
			continue
		}
		ctx, laneCancel := context.WithCancelCause(stageCtx)
		c.lanes[key] = discoveryLane{ctx: ctx, cancel: laneCancel}
	}
	return c, nil
}

func (c *discoveryController) close() {
	c.cancel(context.Canceled)
	c.stop()
}

func (c *discoveryController) lane(group discoveryGroup) discoveryLane {
	return c.lanes[discoveryLaneKey{target: group.Target, region: group.Region}]
}

func (c *discoveryController) denyLane(group discoveryGroup, err error) {
	c.lane(group).cancel(err)
}

func (c *discoveryController) failure() error {
	if err := c.parent.Err(); err != nil {
		return err
	}
	if err := c.budget.failure(); err != nil {
		return err
	}
	if err := c.stage.Err(); err != nil {
		return context.Cause(c.stage)
	}
	return nil
}
