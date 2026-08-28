// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"time"
)

type stageID string
type stageState string

const (
	stageSetup   stageID = "setup"
	stagePut     stageID = "put"
	stageGet     stageID = "get"
	stageList    stageID = "list"
	stageDelete  stageID = "delete"
	stageCleanup stageID = "cleanup"

	stateOK      stageState = "ok"
	stateWaiting stageState = "waiting"
	stateFailed  stageState = "failed"
	stateSkipped stageState = "skipped"

	reasonOK                   = "ok"
	reasonNotRun               = "not_run"
	reasonInternal             = "internal"
	reasonTimeout              = "timeout"
	reasonRequestFailed        = "request_failed"
	reasonPayloadMismatch      = "payload_mismatch"
	reasonNotVisible           = "not_visible"
	reasonStillPresent         = "still_present"
	reasonQuarantinePending    = "quarantine_pending"
	reasonOrphanCleanupPending = "orphan_cleanup_pending"
)

var stageOrder = []stageID{stageSetup, stagePut, stageGet, stageList, stageDelete, stageCleanup}

type stageResult struct {
	state      stageState
	reason     string
	duration   time.Duration
	attempts   int
	retries    int
	operations int
}

type stageResults map[stageID]*stageResult

type stageCounters struct {
	operations int64
	attempts   int64
	retries    int64
	failures   int64
}

type stageStats map[stageID]*stageCounters

func newStageResults() stageResults {
	results := make(stageResults, len(stageOrder))
	for _, stage := range stageOrder {
		results[stage] = &stageResult{state: stateSkipped, reason: reasonNotRun}
	}
	return results
}

func newStageStats() stageStats {
	stats := make(stageStats, len(stageOrder))
	for _, stage := range stageOrder {
		stats[stage] = &stageCounters{}
	}
	return stats
}

func (r *stageResult) succeed() {
	r.state = stateOK
	r.reason = reasonOK
}

func (r *stageResult) fail(reason string) {
	r.state = stateFailed
	r.reason = reason
}

func (r *stageResult) addOperation(attempts int) {
	r.addOperations(1, attempts)
}

func (r *stageResult) addOperations(operations, attempts int) {
	if operations <= 0 || attempts <= 0 {
		return
	}
	r.operations += operations
	r.attempts += attempts
	if attempts > operations {
		r.retries += attempts - operations
	}
}

func (s *stageCounters) add(result *stageResult) {
	s.operations += int64(result.operations)
	s.attempts += int64(result.attempts)
	s.retries += int64(result.retries)
	if result.state == stateFailed {
		s.failures++
	}
}

func readRandomBytes(size int) ([]byte, error) {
	payload := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Collector) collect(ctx context.Context, results stageResults) {
	cycleStartedAt := c.now()
	if c.stateStore == nil {
		results[stageSetup].fail(reasonInternal)
		return
	}
	if c.pendingOwnershipState == nil {
		state, stateErr := c.stateStore.load()
		if stateErr != nil {
			results[stageSetup].fail(reasonInternal)
			return
		}
		c.pendingOwnershipState = state
	}
	reconciliationResumed := false
	if c.pendingOwnershipState != nil {
		if err := c.touchOwnership(); err != nil {
			results[stageSetup].fail(reasonInternal)
			return
		}
		if c.pendingOwnershipState.ReconciliationPending {
			if !c.resumeSingleReconciliation(ctx, results) {
				return
			}
			reconciliationResumed = true
			if c.pendingOwnershipState != nil && !c.cleanupOwnedSingleState(ctx, results) {
				return
			}
		} else if !c.cleanupOwnedSingleState(ctx, results) {
			return
		}
	}
	if c.pendingOwnershipState == nil && !reconciliationResumed {
		if !c.prepareCollection(ctx, results) {
			return
		}
	}
	if !c.verifySingleBucketUnversioned(ctx, results[stageSetup]) {
		return
	}

	payload, key, ok := c.setupProbe(ctx, results)
	if !ok {
		return
	}
	payloadHash := sha256.Sum256(payload)

	state := &ownershipState{
		Phase:              string(multisiteSourcePut),
		SourceKey:          key,
		CreatedAt:          cycleStartedAt,
		SourcePutAttempted: true,
	}
	if err := c.reserveOwnershipPublication(); err != nil {
		results[stageSetup].fail(publicationFailureReason(err))
		return
	}
	saveErr := c.saveOwnership(state)
	c.releaseOwnershipHandoff()
	if saveErr != nil {
		results[stageSetup].fail(reasonInternal)
		return
	}
	c.pendingOwnershipState = state

	if !c.putProbe(ctx, key, payload, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	if !c.getProbe(ctx, key, payload, payloadHash, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	if !c.listProbe(ctx, key, results) {
		c.deleteAndVerify(ctx, key, results)
		return
	}
	c.deleteAndVerify(ctx, key, results)
}

func (c *Collector) prepareCollection(ctx context.Context, results stageResults) bool {
	// As with multisite, a keyless blocker is durable before the first LIST. A
	// request failure or crash cannot expose the old owner namespace to a route
	// replacement before reconciliation proves it safe.
	if !c.rememberSingleReconciliationBlocker(results) {
		return false
	}

	probeKeys, ok := c.listSingleProbeKeys(ctx, results)
	if !ok {
		return false
	}
	if !c.publishSingleReconciliation(c.pendingOwnershipState, probeKeys, results) {
		return false
	}
	if c.pendingOwnershipState == nil {
		results[stageSetup].succeed()
		return true
	}

	if !c.cleanupOwnedKeyBatch(ctx, results, cleanupBatchSize) {
		return false
	}
	results[stageCleanup].succeed()
	c.finishPendingSetup(results)
	return false
}

func (c *Collector) resumeSingleReconciliation(ctx context.Context, results stageResults) bool {
	probeKeys, ok := c.listSingleProbeKeys(ctx, results)
	if !ok {
		return false
	}
	return c.publishSingleReconciliation(c.pendingOwnershipState, probeKeys, results)
}

func (c *Collector) listSingleProbeKeys(ctx context.Context, results stageResults) ([]string, bool) {
	probePrefix := c.stateStore.sourceProbePrefix
	start := c.now()
	keys, truncated, attempts, err := c.client.ListObjects(ctx, c.Bucket, probePrefix, maxOwnedKeys+1)
	elapsed := c.now().Sub(start)
	setup := results[stageSetup]
	setup.duration += elapsed
	setup.addOperation(attempts)
	if err != nil {
		setup.fail(errorReason(err))
		return nil, false
	}

	probeKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if isProbeKey(probePrefix, key) {
			probeKeys = append(probeKeys, key)
		}
	}
	if truncated || len(probeKeys) > maxOwnedKeys {
		setup.fail(reasonInternal)
		return nil, false
	}
	return probeKeys, true
}

func (c *Collector) rememberSingleReconciliationBlocker(results stageResults) bool {
	if err := c.reserveOwnershipPublication(); err != nil {
		results[stageSetup].fail(publicationFailureReason(err))
		return false
	}
	defer c.releaseOwnershipHandoff()

	state := &ownershipState{
		Phase: string(multisiteCleanup), CreatedAt: c.now(), ReconciliationPending: true,
	}
	if err := c.saveOwnership(state); err != nil {
		results[stageSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) publishSingleReconciliation(
	existing *ownershipState, keys []string, results stageResults,
) bool {
	if err := c.reserveOwnershipPublication(); err != nil {
		results[stageSetup].fail(publicationFailureReason(err))
		return false
	}
	defer c.releaseOwnershipHandoff()

	state := &ownershipState{
		Phase: string(multisiteCleanup), CreatedAt: c.now(), TerminalReason: reasonOrphanCleanupPending,
	}
	if existing != nil {
		state = existing.clone()
		state.ReconciliationPending = false
		state.RetiredAt = nil
		state.TerminalReason = reasonOrphanCleanupPending
	}
	exactKeysFit := true
	for _, key := range keys {
		owned := ownedKey{Scope: ownershipSingle, Key: key}
		if state.ownsKey(owned) {
			continue
		}
		if len(state.PendingKeys) >= maxOwnedKeys {
			exactKeysFit = false
			break
		}
		state.PendingKeys = append(state.PendingKeys, owned)
	}
	if !exactKeysFit {
		results[stageSetup].fail(reasonInternal)
		return false
	}

	if len(state.PendingKeys) == 0 {
		if existing == nil {
			return true
		}
		if err := c.stateStore.clear(); err != nil {
			results[stageSetup].fail(reasonInternal)
			return false
		}
		c.pendingOwnershipState = nil
		results[stageSetup].succeed()
		return true
	}

	if err := c.saveOwnership(state); err != nil {
		results[stageSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) cleanupOwnedKeyBatch(ctx context.Context, results stageResults, limit int) bool {
	original := c.pendingOwnershipState
	cleanup := results[stageCleanup]
	if original == nil || len(original.PendingKeys) == 0 {
		return true
	}

	count := min(limit, len(original.PendingKeys))
	for range count {
		// Each durable transition starts from the last persisted key set. Never
		// mutate the live clone across multiple keys before another save/clear.
		state := c.pendingOwnershipState.clone()
		owned := state.PendingKeys[0]
		if !c.verifySingleBucketUnversioned(ctx, results[stageCleanup]) {
			c.finishPendingSetup(results)
			return false
		}
		start := c.now()
		attempts, err := c.client.DeleteObject(ctx, c.Bucket, owned.Key)
		elapsed := c.now().Sub(start)
		cleanup.duration += elapsed
		cleanup.addOperation(attempts)
		if err != nil {
			cleanup.fail(errorReason(err))
			c.finishPendingSetup(results)
			return false
		}

		start = c.now()
		exists, report, err := c.client.ObjectExists(ctx, c.Bucket, owned.Key)
		elapsed = c.now().Sub(start)
		cleanup.duration += elapsed
		cleanup.addOperations(report.operations, report.attempts)
		if err != nil {
			cleanup.fail(errorReason(err))
			c.finishPendingSetup(results)
			return false
		}
		if exists {
			cleanup.fail(reasonStillPresent)
			c.finishPendingSetup(results)
			return false
		}
		state.removeOwnedKey(owned)
		if state.hasActiveObject() || len(state.PendingKeys) > 0 {
			if err := c.saveOwnership(state); err != nil {
				cleanup.fail(reasonInternal)
				c.finishPendingSetup(results)
				return false
			}
			c.pendingOwnershipState = state
			continue
		}
		if err := c.stateStore.clear(); err != nil {
			cleanup.fail(reasonInternal)
			c.finishPendingSetup(results)
			return false
		}
		c.pendingOwnershipState = nil
		break
	}
	cleanup.succeed()
	return true
}

func (c *Collector) cleanupOwnedSingleState(ctx context.Context, results stageResults) bool {
	original := c.pendingOwnershipState
	if original == nil {
		return true
	}
	if original.ReconciliationPending {
		results[stageCleanup].state = stateWaiting
		results[stageCleanup].reason = reasonReconciliationPending
		return false
	}
	state := original.clone()

	if state.SourceKey != "" {
		if !c.cleanupStage(ctx, state.SourceKey, results) {
			c.finishPendingSetup(results)
			return false
		}

		// The journal may be the only witness of a DELETE that completed just
		// before a restart. Retain its exact key through one quarantine interval so
		// a delayed PUT commit cannot bypass the owner-prefix recheck.
		owned := ownedKey{Scope: ownershipSingle, Key: state.SourceKey}
		if !state.ownsKey(owned) {
			if len(state.PendingKeys) >= maxOwnedKeys {
				results[stageCleanup].fail(reasonInternal)
				return false
			}
			state.PendingKeys = append(state.PendingKeys, owned)
		}
		if state.QuarantinedAt == nil {
			quarantinedAt := state.CreatedAt
			state.QuarantinedAt = &quarantinedAt
		}
		state.SourceKey = ""
		state.PayloadDigest = ""
		if err := c.saveOwnership(state); err != nil {
			results[stageCleanup].fail(reasonInternal)
			return false
		}
		c.pendingOwnershipState = state
		c.finishPendingSetup(results)
		return false
	}

	if len(state.PendingKeys) > 0 {
		if state.QuarantinedAt != nil {
			if c.now().Sub(*state.QuarantinedAt) < time.Duration(c.UpdateEvery)*time.Second {
				setup := results[stageSetup]
				setup.state = stateSkipped
				setup.reason = reasonQuarantinePending
				return false
			}
			if !c.refreshSinglePendingKeys(ctx, results) {
				return false
			}
			if c.pendingOwnershipState == nil {
				return true
			}
			c.finishPendingSetup(results)
		}
		if !c.cleanupOwnedKeyBatch(ctx, results, cleanupBatchSize) {
			return false
		}
	}

	if c.pendingOwnershipState == nil {
		return false
	}
	c.finishPendingSetup(results)
	return false
}

func (c *Collector) refreshSinglePendingKeys(ctx context.Context, results stageResults) bool {
	original := c.pendingOwnershipState
	if original == nil {
		return true
	}
	start := c.now()
	keys, truncated, attempts, err := c.client.ListObjects(ctx, c.Bucket, c.stateStore.sourceProbePrefix, maxOwnedKeys+1)
	elapsed := c.now().Sub(start)
	setup := results[stageSetup]
	setup.duration += elapsed
	setup.addOperation(attempts)
	if err != nil {
		setup.fail(errorReason(err))
		return false
	}

	if truncated {
		setup.fail(reasonInternal)
		return false
	}

	state := original.clone()
	state.PendingKeys = nil
	for _, key := range keys {
		if isProbeKey(c.stateStore.sourceProbePrefix, key) {
			state.PendingKeys = append(state.PendingKeys, ownedKey{Scope: ownershipSingle, Key: key})
		}
	}
	if len(state.PendingKeys) == 0 && !truncated {
		if err := c.stateStore.clear(); err != nil {
			results[stageCleanup].fail(reasonInternal)
			return false
		}
		c.pendingOwnershipState = nil
		return true
	}
	if len(state.PendingKeys) > maxOwnedKeys {
		results[stageSetup].fail(reasonInternal)
		return false
	}
	if err := c.saveOwnership(state); err != nil {
		results[stageSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) verifySingleBucketUnversioned(ctx context.Context, result *stageResult) bool {
	start := c.now()
	status, attempts, err := c.client.GetBucketVersioning(ctx, c.Bucket)
	elapsed := c.now().Sub(start)
	result.duration += elapsed
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if status != "" {
		result.fail(reasonBucketVersioned)
		return false
	}
	return true
}

func (c *Collector) finishPendingSetup(results stageResults) {
	setup := results[stageSetup]
	setup.state = stateFailed
	setup.reason = reasonOrphanCleanupPending
}

func (c *Collector) setupProbe(_ context.Context, results stageResults) ([]byte, string, bool) {
	start := c.now()
	payload, payloadErr := c.randomRead(probePayloadBytes)
	var key string
	var keyErr error
	if payloadErr == nil {
		key, keyErr = c.newProbeKey()
	}
	results[stageSetup].duration += c.now().Sub(start)

	if payloadErr != nil || keyErr != nil {
		results[stageSetup].fail(reasonInternal)
		return nil, "", false
	}
	results[stageSetup].succeed()
	return payload, key, true
}

func (c *Collector) newProbeKey() (string, error) {
	suffix, err := c.randomRead(8)
	if err != nil {
		return "", err
	}
	ownerTag := c.stateStore.ownerTag
	name := fmt.Sprintf("probe-%d-%s-%s.bin", c.now().UnixNano(), hex.EncodeToString(suffix), ownerTag)
	return c.stateStore.sourceProbePrefix + name, nil
}

func isProbeKey(prefix, key string) bool {
	base, valid := strings.CutPrefix(key, prefix)
	return valid && multisiteProbeKeyRE.MatchString(base)
}

func (c *Collector) putProbe(ctx context.Context, key string, payload []byte, results stageResults) bool {
	start := c.now()
	attempts, err := c.client.PutObject(ctx, c.Bucket, key, payload)
	result := results[stagePut]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) getProbe(ctx context.Context, key string, payload []byte, payloadHash [sha256.Size]byte, results stageResults) bool {
	start := c.now()
	body, attempts, err := c.client.GetObject(ctx, c.Bucket, key, int64(len(payload)+1))
	result := results[stageGet]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}

	bodyHash := sha256.Sum256(body)
	if !bytes.Equal(payloadHash[:], bodyHash[:]) {
		result.fail(reasonPayloadMismatch)
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) listProbe(ctx context.Context, key string, results stageResults) bool {
	start := c.now()
	keys, _, attempts, err := c.client.ListObjects(ctx, c.Bucket, c.stateStore.sourceProbePrefix, 100)
	result := results[stageList]
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if slices.Contains(keys, key) {
		result.succeed()
		return true
	}
	result.fail(reasonNotVisible)
	return false
}

func (c *Collector) deleteAndVerify(ctx context.Context, key string, results stageResults) {
	result := results[stageDelete]
	if !c.verifySingleBucketUnversioned(ctx, result) {
		return
	}

	start := c.now()
	attempts, err := c.client.DeleteObject(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
	} else {
		result.succeed()
	}

	if !c.verifyObjectGone(ctx, key, results) {
		return
	}
	if state := c.pendingOwnershipState; state != nil && state.SourceKey == key {
		updated := state.clone()
		updated.SourceKey = ""
		updated.PayloadDigest = ""
		owned := ownedKey{Scope: ownershipSingle, Key: key}
		if !updated.ownsKey(owned) {
			updated.PendingKeys = append(updated.PendingKeys, owned)
		}
		updated.QuarantinedAt = &updated.CreatedAt
		if err := c.saveOwnership(updated); err != nil {
			results[stageCleanup].fail(reasonInternal)
			return
		}
		c.pendingOwnershipState = updated
	}
}

func (c *Collector) verifyObjectGone(ctx context.Context, key string, results stageResults) bool {
	result := results[stageCleanup]
	start := c.now()
	exists, report, err := c.client.ObjectExists(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperations(report.operations, report.attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if exists {
		// A still-present object is retried on a later collection cycle, keeping
		// the destructive operation budget bounded.
		result.fail(reasonStillPresent)
		return false
	}
	result.succeed()
	return true
}

func (c *Collector) cleanupStage(ctx context.Context, key string, results stageResults) bool {
	result := results[stageCleanup]

	start := c.now()
	exists, report, err := c.client.ObjectExists(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperations(report.operations, report.attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if !exists {
		result.succeed()
		return true
	}

	start = c.now()
	status, versionAttempts, versionErr := c.client.GetBucketVersioning(ctx, c.Bucket)
	result.duration += c.now().Sub(start)
	result.addOperation(versionAttempts)
	if versionErr != nil {
		result.fail(errorReason(versionErr))
		return false
	}
	if status != "" {
		result.fail(reasonBucketVersioned)
		return false
	}

	start = c.now()
	attempts, err := c.client.DeleteObject(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}

	start = c.now()
	exists, report, err = c.client.ObjectExists(ctx, c.Bucket, key)
	result.duration += c.now().Sub(start)
	result.addOperations(report.operations, report.attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if exists {
		// Retry on the next collection cycle. A same-cycle retry would double the
		// destructive budget and cannot distinguish slow DELETE propagation yet.
		result.fail(reasonStillPresent)
		return false
	}
	result.succeed()
	return true
}

func errorReason(err error) string {
	if err == nil {
		return reasonOK
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return reasonTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return reasonTimeout
	}
	return reasonRequestFailed
}
