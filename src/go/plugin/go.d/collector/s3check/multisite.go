// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"time"
)

const (
	reasonDeleteVerificationDisabled = "delete_verification_disabled"
	reasonRecovered                  = "recovered"
	reasonRestartAbandoned           = "restart_abandoned"
	reasonReplicationTimeout         = "replication_timeout"
	reasonDeleteTimeout              = "delete_timeout"
	reasonShutdownCleanup            = "shutdown_cleanup"
	reasonReconciliationPending      = "reconciliation_pending"
	reasonBucketVersioned            = "bucket_versioned"
)

func (c *Collector) collectMultisite(ctx context.Context, cycle *multisiteCycle) {
	if c.stateStore == nil {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return
	}

	c.releaseOwnershipHandoff()

	if c.pendingOwnershipState != nil {
		if err := c.touchOwnership(); err != nil {
			cycle.phases[multisiteSetup].fail(reasonInternal)
			return
		}
	}

	if c.pendingOwnershipState == nil {
		state, stateErr := c.stateStore.load()
		if stateErr != nil {
			cycle.phases[multisiteSetup].fail(reasonInternal)
			return
		}
		c.pendingOwnershipState = state
	}

	reconciliationResumed := false
	if c.pendingOwnershipState != nil && c.pendingOwnershipState.ReconciliationPending {
		if !c.resumeMultisiteReconciliation(ctx, cycle) {
			return
		}
		reconciliationResumed = true
	}

	if c.pendingOwnershipState == nil && !reconciliationResumed {
		if !c.reconcileMultisiteRoutes(ctx, cycle) {
			return
		}
	}
	if c.pendingOwnershipState == nil {
		if !c.verifyMultisiteBucketUnversioned(ctx, c.client, c.Bucket, cycle.phases[multisiteSetup]) {
			return
		}
		if !c.verifyMultisiteBucketUnversioned(
			ctx, c.destinationClient, c.Destination.Bucket, cycle.phases[multisiteSetup],
		) {
			return
		}
		if !c.startMultisiteProbe(cycle) {
			return
		}
	}

	state := c.pendingOwnershipState
	switch multisitePhase(state.Phase) {
	case multisiteSourcePut:
		c.putSourceAndAdvance(ctx, cycle)
	case multisiteReplication:
		c.collectReplicationPhase(ctx, cycle)
	case multisiteSourceDelete:
		c.deleteSourceAndAdvance(ctx, cycle)
	case multisiteDeleteWait:
		c.waitForDestinationDelete(ctx, cycle)
	case multisiteCleanup:
		c.cleanupMultisiteState(ctx, cycle, true)
	default:
		cycle.phases[multisiteCleanup].fail(reasonInternal)
	}
}

type multisiteEndpointKeys struct {
	keys      []string
	truncated bool
}

func (c *Collector) listMultisiteEndpointKeys(
	ctx context.Context,
	endpoint endpointConfig,
	client s3Client,
	cycle *multisiteCycle,
) (multisiteEndpointKeys, bool) {
	// Ownership is Agent/job-specific. Another Agent may intentionally probe the
	// same route without surrendering its active objects.
	probePrefix := multisiteProbePrefix(endpoint.Prefix, c.stateStore.ownerTag)
	start := c.now()
	keys, truncated, attempts, err := client.ListObjects(ctx, endpoint.Bucket, probePrefix, maxOwnedKeys+1)
	elapsed := c.now().Sub(start)
	setup := cycle.phases[multisiteSetup]
	setup.duration += elapsed
	setup.addOperation(attempts)
	if err != nil {
		setup.fail(errorReason(err))
		return multisiteEndpointKeys{}, false
	}

	owned := make([]string, 0, len(keys))
	for _, key := range keys {
		if isMultisiteProbeKey(key, probePrefix, c.stateStore.ownerTag) {
			owned = append(owned, key)
		}
	}
	return multisiteEndpointKeys{keys: owned, truncated: truncated}, true
}

func (c *Collector) reconcileMultisiteRoutes(ctx context.Context, cycle *multisiteCycle) bool {
	// A keyless blocker is durable before the first LIST. A crash during either
	// request therefore cannot expose an unreconciled route to a replacement.
	if !c.rememberMultisiteReconciliationBlocker(nil, cycle) {
		return false
	}

	source, sourceOK := c.listMultisiteEndpointKeys(
		ctx, c.sourceEndpoint(), c.client, cycle,
	)
	if sourceOK && !c.rememberMultisiteReconciliationBlocker(source.keys, cycle) {
		return false
	}
	destination, destinationOK := c.listMultisiteEndpointKeys(
		ctx, c.destinationEndpoint(), c.destinationClient, cycle,
	)
	if !destinationOK {
		return false
	}
	if !sourceOK {
		c.rememberMultisiteReconciliationBlocker(destination.keys, cycle)
		return false
	}

	if source.truncated || destination.truncated ||
		len(source.keys) > maxOwnedKeys || len(destination.keys) > maxOwnedKeys {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return false
	}
	if !c.publishMultisiteReconciliation(c.pendingOwnershipState, source.keys, destination.keys, cycle) {
		return false
	}
	if c.pendingOwnershipState == nil {
		return true
	}

	if !c.cleanupMultisiteOwnedKeys(ctx, cycle, cleanupBatchSize) {
		return false
	}
	cleanup := cycle.phases[multisiteCleanup]
	if c.pendingOwnershipState == nil {
		cleanup.succeed()
	} else if c.pendingOwnershipState.CleanupQuarantinedAt != nil {
		cleanup.wait(reasonQuarantinePending)
	} else {
		cleanup.wait(reasonOrphanCleanupPending)
	}
	c.markOrphanCleanupPending(cycle)
	return false
}

func (c *Collector) resumeMultisiteReconciliation(ctx context.Context, cycle *multisiteCycle) bool {
	source, sourceOK := c.listMultisiteEndpointKeys(
		ctx, c.sourceEndpoint(), c.client, cycle,
	)
	if sourceOK && !c.mergeMultisiteDiscovery(source, cycle) {
		return false
	}
	destination, destinationOK := c.listMultisiteEndpointKeys(
		ctx, c.destinationEndpoint(), c.destinationClient, cycle,
	)
	if !destinationOK {
		return false
	}
	if !sourceOK {
		c.mergeMultisiteDiscovery(destination, cycle)
		return false
	}
	if source.truncated || destination.truncated ||
		len(source.keys) > maxOwnedKeys || len(destination.keys) > maxOwnedKeys {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return false
	}
	return c.publishMultisiteReconciliation(
		c.pendingOwnershipState, source.keys, destination.keys, cycle,
	)
}

func (c *Collector) mergeMultisiteDiscovery(discovery multisiteEndpointKeys, cycle *multisiteCycle) bool {
	if discovery.truncated || len(discovery.keys) > maxOwnedKeys {
		return true
	}
	if err := c.reserveOwnershipPublication(); err != nil {
		cycle.phases[multisiteSetup].fail(publicationFailureReason(err))
		return false
	}
	defer c.releaseOwnershipHandoff()

	original := c.pendingOwnershipState
	state := original.clone()
	added := false
	for _, key := range discovery.keys {
		before := len(state.PendingKeys)
		if !c.addMultisitePair(state, key) {
			// Keep the existing durable blocker. An over-limit union cannot be
			// represented exactly and must not release the route.
			return true
		}
		if len(state.PendingKeys) > before {
			added = true
		}
	}
	if !added {
		return true
	}
	state.RetiredAt = nil
	if err := c.saveOwnership(state); err != nil {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) addMultisitePair(state *ownershipState, key string) bool {
	base := path.Base(key)
	return state.ownsOrAddsKey(ownedKey{Scope: ownershipSource, Key: c.stateStore.sourceProbePrefix + base}) &&
		state.ownsOrAddsKey(
			ownedKey{Scope: ownershipDestination, Key: c.stateStore.destinationProbePrefix + base},
		)
}

func (c *Collector) rememberMultisiteReconciliationBlocker(keys []string, cycle *multisiteCycle) bool {
	if err := c.reserveOwnershipPublication(); err != nil {
		cycle.phases[multisiteSetup].fail(publicationFailureReason(err))
		return false
	}
	defer c.releaseOwnershipHandoff()

	state := &ownershipState{
		Phase: string(multisiteCleanup), CreatedAt: c.now(), ReconciliationPending: true,
	}
	exactKeysFit := true
	for _, key := range keys {
		if !c.addMultisitePair(state, key) {
			exactKeysFit = false
			break
		}
	}
	if !exactKeysFit {
		state.PendingKeys = nil
	}

	if err := c.saveOwnership(state); err != nil {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) publishMultisiteReconciliation(
	existing *ownershipState, sourceKeys, destinationKeys []string, cycle *multisiteCycle,
) bool {
	if err := c.reserveOwnershipPublication(); err != nil {
		cycle.phases[multisiteSetup].fail(publicationFailureReason(err))
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
	for _, key := range sourceKeys {
		if !c.addMultisitePair(state, key) {
			exactKeysFit = false
			break
		}
	}
	if exactKeysFit {
		for _, key := range destinationKeys {
			if !c.addMultisitePair(state, key) {
				exactKeysFit = false
				break
			}
		}
	}
	if !exactKeysFit {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		if existing != nil {
			return false
		}
		state.PendingKeys = nil
		state.ReconciliationPending = true
		state.TerminalReason = ""
		if err := c.saveOwnership(state); err != nil {
			cycle.phases[multisiteSetup].fail(reasonInternal)
		} else {
			c.pendingOwnershipState = state
		}
		return false
	}

	if len(state.PendingKeys) == 0 {
		if existing == nil {
			return true
		}
		if err := c.stateStore.clear(); err != nil {
			cycle.phases[multisiteSetup].fail(reasonInternal)
			return false
		}
		c.pendingOwnershipState = nil
		cycle.phases[multisiteSetup].succeed()
		return true
	}

	// Publish the complete cross-endpoint union atomically. No destructive
	// cleanup is executable until this single durable save succeeds.
	if err := c.saveOwnership(state); err != nil {
		cycle.phases[multisiteSetup].fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	return true
}

func (c *Collector) cleanupMultisiteOwnedKeys(ctx context.Context, cycle *multisiteCycle, limit int) bool {
	state := c.pendingOwnershipState
	cleanup := cycle.phases[multisiteCleanup]
	if state == nil || len(state.PendingKeys) == 0 {
		return true
	}

	batch := c.pendingCleanupBatch(limit)
	if len(batch) == 0 {
		cleanup.fail(reasonInternal)
		return false
	}
	if state.CleanupConfirmedAt == nil && state.CleanupQuarantinedAt != nil {
		horizon := c.multisiteCleanupHorizon()
		if state.CleanupDeleteAttempted {
			horizon += c.multisiteDestructiveRetryGrace()
		}
		if c.now().Sub(*state.CleanupQuarantinedAt) < horizon {
			cleanup.wait(reasonQuarantinePending)
			return true
		}
	}
	if state.CleanupConfirmedAt != nil &&
		c.now().Sub(*state.CleanupConfirmedAt) < time.Duration(c.UpdateEvery)*time.Second {
		cleanup.wait(reasonQuarantinePending)
		return true
	}

	// Alternate endpoint order across confirmation rounds. Neither round is a
	// distributed snapshot, but together they give each endpoint a later proof
	// after the other endpoint and retain ownership throughout the confirmation
	// interval before a route can change.
	destinationFirst := state.CleanupConfirmedAt != nil
	proof := c.proveMultisiteBatchAbsent(ctx, cycle, batch, destinationFirst)
	if proof == cleanupProofFailed {
		return false
	}
	if proof == cleanupProofAbsent {
		if state.CleanupConfirmedAt == nil {
			now := c.now()
			if state.CleanupQuarantinedAt == nil {
				state.CleanupQuarantinedAt = &now
			}
			state.CleanupConfirmedAt = &now
			state.CleanupDeleteAttempted = false
			if err := c.saveOwnership(state); err != nil {
				cleanup.fail(reasonInternal)
				return false
			}
			cleanup.wait(reasonQuarantinePending)
			return true
		}
		return c.finishMultisiteBatch(cycle, batch)
	}

	// A confirmation-triggered DELETE starts a new bounded protocol. Publish the
	// reset before any destructive retry so a still-present result, request
	// failure, or crash cannot reuse the previous confirmation.
	retriedAt := c.now()
	state.CleanupQuarantinedAt = &retriedAt
	state.CleanupConfirmedAt = nil
	state.CleanupDeleteAttempted = true
	if err := c.saveOwnership(state); err != nil {
		cleanup.fail(reasonInternal)
		return false
	}

	if !c.deleteMultisiteBatch(ctx, cycle, batch) {
		c.markMultisiteRetryCompleted(state, cleanup)
		return false
	}
	if c.proveMultisiteBatchAbsent(ctx, cycle, batch, false) != cleanupProofAbsent {
		c.markMultisiteRetryCompleted(state, cleanup)
		return false
	}
	if !c.markMultisiteRetryCompleted(state, cleanup) {
		return false
	}
	cleanup.wait(reasonQuarantinePending)
	return true
}

func (c *Collector) pendingCleanupBatch(limit int) []ownedKey {
	state := c.pendingOwnershipState
	if len(state.PendingKeys) == 0 {
		return nil
	}

	// Cleanup progresses one probe basename at a time. This keeps source and
	// destination counterparts in every cross-bucket proof batch; splitting a
	// pair across cycles would let one endpoint be confirmed without observing
	// the other endpoint at all.
	base := path.Base(state.PendingKeys[0].Key)
	batch := make([]ownedKey, 0, limit)
	for _, owned := range state.PendingKeys {
		if path.Base(owned.Key) != base {
			continue
		}
		if len(batch) >= limit {
			return nil
		}
		batch = append(batch, owned)
	}
	if len(batch) == 0 {
		return nil
	}
	return batch
}

func (c *Collector) deleteMultisiteBatch(ctx context.Context, cycle *multisiteCycle, batch []ownedKey) bool {
	cleanup := cycle.phases[multisiteCleanup]
	for _, owned := range batch {
		client := c.client
		bucket := c.Bucket
		if owned.Scope == ownershipDestination {
			client = c.destinationClient
			bucket = c.Destination.Bucket
		}
		if !c.verifyMultisiteBucketUnversioned(ctx, client, bucket, cleanup) {
			c.markOrphanCleanupPending(cycle)
			return false
		}

		start := c.now()
		attempts, err := client.DeleteObject(ctx, bucket, owned.Key)
		elapsed := c.now().Sub(start)
		cleanup.duration += elapsed
		cleanup.addOperation(attempts)
		if err != nil {
			cleanup.fail(errorReason(err))
			c.markOrphanCleanupPending(cycle)
			return false
		}
	}
	return true
}

func (c *Collector) markMultisiteRetryCompleted(state *ownershipState, cleanup *multisiteResult) bool {
	previousQuarantine := state.CleanupQuarantinedAt
	previousAttempted := state.CleanupDeleteAttempted
	completedAt := c.now()
	state.CleanupQuarantinedAt = &completedAt
	state.CleanupDeleteAttempted = false
	if err := c.saveOwnership(state); err != nil {
		state.CleanupQuarantinedAt = previousQuarantine
		state.CleanupDeleteAttempted = previousAttempted
		cleanup.fail(reasonInternal)
		return false
	}
	return true
}

func (c *Collector) finishMultisiteBatch(cycle *multisiteCycle, batch []ownedKey) bool {
	original := c.pendingOwnershipState
	cleanup := cycle.phases[multisiteCleanup]
	state := original.clone()
	for _, owned := range batch {
		state.removeOwnedKey(owned)
	}
	if state.hasActiveObject() || len(state.PendingKeys) > 0 {
		nextQuarantineAt := c.now()
		state.CleanupQuarantinedAt = &nextQuarantineAt
		state.CleanupConfirmedAt = nil
		state.CleanupDeleteAttempted = false
		if err := c.saveOwnership(state); err != nil {
			cleanup.fail(reasonInternal)
			return false
		}
		c.pendingOwnershipState = state
		return true
	}
	if err := c.stateStore.clear(); err != nil {
		cleanup.fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = nil
	cleanup.succeed()
	return true
}

func (c *Collector) markOrphanCleanupPending(cycle *multisiteCycle) {
	setup := cycle.phases[multisiteSetup]
	setup.state = stateFailed
	setup.reason = reasonOrphanCleanupPending
}

func (c *Collector) startMultisiteProbe(cycle *multisiteCycle) bool {
	if err := c.reserveOwnershipPublication(); err != nil {
		cycle.phases[multisiteSetup].fail(publicationFailureReason(err))
		return false
	}
	defer c.releaseOwnershipHandoff()

	start := c.now()
	payload, payloadErr := c.randomRead(probePayloadBytes)
	var sourceKey string
	var destinationKey string
	var keyErr error
	if payloadErr == nil {
		sourceKey, destinationKey, keyErr = c.newMultisiteProbeKeys()
	}
	setup := cycle.phases[multisiteSetup]
	setup.duration += c.now().Sub(start)
	if payloadErr != nil || keyErr != nil {
		setup.fail(reasonInternal)
		return false
	}

	digest := sha256.Sum256(payload)
	c.probePayload = payload
	now := c.now()
	state := &ownershipState{
		Phase:          string(multisiteSourcePut),
		SourceKey:      sourceKey,
		DestinationKey: destinationKey,
		PayloadDigest:  hex.EncodeToString(digest[:]),
		CreatedAt:      now,
	}
	if err := c.saveOwnership(state); err != nil {
		setup.fail(reasonInternal)
		return false
	}
	c.pendingOwnershipState = state
	setup.succeed()
	return true
}

func (c *Collector) newMultisiteProbeKeys() (string, string, error) {
	suffix, err := c.randomRead(8)
	if err != nil {
		return "", "", err
	}
	ownerTag := c.stateStore.ownerTag
	name := fmt.Sprintf("probe-%d-%s-%s.bin", c.now().UnixNano(), hex.EncodeToString(suffix), ownerTag)
	return c.stateStore.sourceProbePrefix + name, c.stateStore.destinationProbePrefix + name, nil
}

func (c *Collector) putSourceAndAdvance(ctx context.Context, cycle *multisiteCycle) {
	state := c.pendingOwnershipState
	result := cycle.phases[multisiteSourcePut]

	if !state.SourcePutAttempted {
		if len(c.probePayload) == 0 {
			result.fail(reasonRestartAbandoned)
			if err := c.stateStore.clear(); err != nil {
				result.fail(reasonInternal)
			}
			c.pendingOwnershipState = nil
			return
		}
		state.SourcePutAttempted = true
		if err := c.saveOwnership(state); err != nil {
			state.SourcePutAttempted = false
			result.fail(reasonInternal)
			return
		}
	}

	if state.SourcePutAt == nil {
		if len(c.probePayload) > 0 {
			c.advanceReplicationAfterPut(ctx, cycle)
			return
		}
		c.recoverPreparedState(ctx, cycle)
		return
	}
	c.collectReplicationPhase(ctx, cycle)
}

func (c *Collector) advanceReplicationAfterPut(ctx context.Context, cycle *multisiteCycle) {
	state := c.pendingOwnershipState
	result := cycle.phases[multisiteSourcePut]

	start := c.now()
	attempts, err := c.client.PutObject(ctx, c.Bucket, state.SourceKey, c.probePayload)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		c.transitionMultisiteFailure(ctx, cycle, multisiteSourcePut, errorReason(err))
		return
	}

	now := c.now()
	state.SourcePutAt = &now
	state.Phase = string(multisiteReplication)
	result.succeed()
	if !c.persistTransition(cycle, multisiteSourcePut) {
		return
	}
	c.collectReplicationPhase(ctx, cycle)
}

func (c *Collector) collectReplicationPhase(ctx context.Context, cycle *multisiteCycle) {
	state := c.pendingOwnershipState
	if state.SourcePutAt == nil {
		if !c.recoverPreparedState(ctx, cycle) {
			return
		}
		state = c.pendingOwnershipState
	}

	sourcePutAt := *state.SourcePutAt
	if elapsed := c.now().Sub(sourcePutAt); elapsed >= time.Duration(c.ReplicationTimeoutMS)*time.Millisecond {
		c.setRPOStatus(cycle, elapsed)
		c.failReplication(ctx, cycle, reasonReplicationTimeout)
		return
	}

	start := c.now()
	body, attempts, err := c.destinationClient.GetObject(ctx, c.Destination.Bucket, state.DestinationKey, int64(probePayloadBytes+1))
	elapsed := c.now().Sub(start)
	result := cycle.phases[multisiteReplication]
	result.duration += elapsed
	result.addOperation(attempts)
	if err == nil || isNoSuchKeyError(err) {
		if elapsed = c.now().Sub(sourcePutAt); elapsed >= time.Duration(c.ReplicationTimeoutMS)*time.Millisecond {
			c.setRPOStatus(cycle, elapsed)
			c.failReplication(ctx, cycle, reasonReplicationTimeout)
			return
		}
	}

	if err != nil {
		if isNoSuchKeyError(err) {
			c.setRPOStatus(cycle, c.now().Sub(sourcePutAt))
			result.wait(reasonNotVisible)
			return
		}
		result.fail(errorReason(err))
		return
	}
	c.setRPOStatus(cycle, c.now().Sub(sourcePutAt))

	bodyDigest := sha256.Sum256(body)
	expectedDigest, decodeErr := hex.DecodeString(state.PayloadDigest)
	if decodeErr != nil {
		result.fail(reasonInternal)
		c.transitionMultisiteFailure(ctx, cycle, multisiteReplication, reasonInternal)
		return
	}
	if !bytes.Equal(bodyDigest[:], expectedDigest) {
		mismatch := float64(1)
		cycle.payloadMismatch = &mismatch
		result.fail(reasonPayloadMismatch)
		c.transitionMultisiteFailure(ctx, cycle, multisiteReplication, reasonPayloadMismatch)
		return
	}

	now := c.now()
	mismatch := float64(0)
	cycle.payloadMismatch = &mismatch
	lagMS := float64(now.Sub(sourcePutAt).Nanoseconds()) / 1e6
	cycle.replicationLagMS = &lagMS
	result.succeed()
	state.DestinationVisibleAt = &now
	state.Phase = string(multisiteSourceDelete)
	if !c.persistTransition(cycle, multisiteReplication) {
		return
	}
	c.deleteSourceAndAdvance(ctx, cycle)
}

func (c *Collector) recoverPreparedState(ctx context.Context, cycle *multisiteCycle) bool {
	state := c.pendingOwnershipState
	result := cycle.phases[multisiteSourcePut]

	start := c.now()
	exists, report, err := c.client.ObjectExists(ctx, c.Bucket, state.SourceKey)
	result.duration += c.now().Sub(start)
	result.addOperations(report.operations, report.attempts)
	if err != nil {
		result.fail(errorReason(err))
		return false
	}
	if !exists {
		result.fail(reasonRestartAbandoned)
		state.Phase = string(multisiteCleanup)
		state.TerminalReason = reasonRestartAbandoned
		if err := c.saveOwnership(state); err != nil {
			result.fail(reasonInternal)
		}
		return false
	}

	// The state was persisted before PUT. If a commit survived the crash, the
	// preparation timestamp is a conservative lower bound for visibility timing.
	created := state.CreatedAt
	state.SourcePutAt = &created
	state.Phase = string(multisiteReplication)
	result.succeed()
	result.reason = reasonRecovered
	if err := c.saveOwnership(state); err != nil {
		result.fail(reasonInternal)
		return false
	}
	return true
}

func (c *Collector) deleteSourceAndAdvance(ctx context.Context, cycle *multisiteCycle) {
	state := c.pendingOwnershipState
	result := cycle.phases[multisiteSourceDelete]

	// Replication can span many cycles, so recheck before the first destructive
	// source operation in this invocation.
	if !c.verifyMultisiteBucketUnversioned(ctx, c.client, c.Bucket, result) {
		return
	}
	if !c.verifyMultisiteBucketUnversioned(ctx, c.destinationClient, c.Destination.Bucket, result) {
		return
	}
	if state.SourceDeleteAttemptedAt == nil {
		attemptedAt := c.now()
		state.SourceDeleteAttemptedAt = &attemptedAt
		if err := c.saveOwnership(state); err != nil {
			state.SourceDeleteAttemptedAt = nil
			result.fail(reasonInternal)
			return
		}
	}

	start := c.now()
	attempts, err := c.client.DeleteObject(ctx, c.Bucket, state.SourceKey)
	result.duration += c.now().Sub(start)
	result.addOperation(attempts)
	if err != nil {
		result.fail(errorReason(err))
		return
	}

	now := c.now()
	state.SourceDeletedAt = &now
	result.succeed()
	if c.VerifyDelete {
		state.Phase = string(multisiteDeleteWait)
		if !c.persistTransition(cycle, multisiteSourceDelete) {
			return
		}
		c.waitForDestinationDelete(ctx, cycle)
		return
	}

	cycle.phases[multisiteDeleteWait].state = stateSkipped
	cycle.phases[multisiteDeleteWait].reason = reasonDeleteVerificationDisabled
	state.Phase = string(multisiteCleanup)
	state.TerminalReason = reasonDeleteVerificationDisabled
	c.persistTransition(cycle, multisiteSourceDelete)
}

func (c *Collector) waitForDestinationDelete(ctx context.Context, cycle *multisiteCycle) {
	state := c.pendingOwnershipState
	if state.SourceDeletedAt == nil {
		c.transitionMultisiteFailure(ctx, cycle, multisiteDeleteWait, reasonInternal)
		return
	}
	sourceDeletedAt := *state.SourceDeletedAt
	measurementStart := sourceDeletedAt
	if state.SourceDeleteAttemptedAt != nil {
		measurementStart = *state.SourceDeleteAttemptedAt
	}
	elapsed := c.now().Sub(measurementStart)
	if elapsed >= time.Duration(c.DeleteTimeoutMS)*time.Millisecond {
		c.setDeleteStatus(cycle, elapsed)
		cycle.phases[multisiteDeleteWait].fail(reasonDeleteTimeout)
		c.transitionMultisiteFailure(ctx, cycle, multisiteDeleteWait, reasonDeleteTimeout)
		return
	}

	start := c.now()
	exists, report, err := c.destinationClient.ObjectExists(ctx, c.Destination.Bucket, state.DestinationKey)
	elapsed = c.now().Sub(start)
	result := cycle.phases[multisiteDeleteWait]
	result.duration += elapsed
	result.addOperations(report.operations, report.attempts)
	if err != nil {
		result.fail(errorReason(err))
		return
	}
	elapsed = c.now().Sub(measurementStart)
	if elapsed >= time.Duration(c.DeleteTimeoutMS)*time.Millisecond {
		c.setDeleteStatus(cycle, elapsed)
		cycle.phases[multisiteDeleteWait].fail(reasonDeleteTimeout)
		c.transitionMultisiteFailure(ctx, cycle, multisiteDeleteWait, reasonDeleteTimeout)
		return
	}
	c.setDeleteStatus(cycle, elapsed)
	if exists {
		result.wait(reasonStillPresent)
		return
	}

	now := c.now()
	lagMS := float64(now.Sub(measurementStart).Nanoseconds()) / 1e6
	cycle.deleteLagMS = &lagMS
	result.succeed()
	state.DestinationGoneAt = &now
	state.Phase = string(multisiteCleanup)
	c.persistTransition(cycle, multisiteDeleteWait)
}

func (c *Collector) verifyMultisiteBucketUnversioned(
	ctx context.Context,
	client s3Client,
	bucket string,
	result *multisiteResult,
) bool {
	start := c.now()
	status, attempts, err := client.GetBucketVersioning(ctx, bucket)
	result.duration += c.now().Sub(start)
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

func (c *Collector) proveMultisiteBatchAbsent(
	ctx context.Context,
	cycle *multisiteCycle,
	batch []ownedKey,
	destinationFirst bool,
) cleanupProofStatus {
	wanted := make(map[ownedKey]struct{}, len(batch))
	for _, owned := range batch {
		wanted[owned] = struct{}{}
	}

	endpoints := []struct {
		scope  ownershipScope
		client s3Client
		bucket string
	}{
		{ownershipSource, c.client, c.Bucket},
		{ownershipDestination, c.destinationClient, c.Destination.Bucket},
	}
	if destinationFirst {
		endpoints[0], endpoints[1] = endpoints[1], endpoints[0]
	}
	for _, endpoint := range endpoints {
		if !ownsScope(batch, endpoint.scope) {
			continue
		}
		probePrefix := c.stateStore.sourceProbePrefix
		if endpoint.scope == ownershipDestination {
			probePrefix = c.stateStore.destinationProbePrefix
		}
		start := c.now()
		keys, truncated, attempts, err := endpoint.client.ListObjects(
			ctx, endpoint.bucket, probePrefix, maxOwnedKeys+1,
		)
		elapsed := c.now().Sub(start)
		cleanup := cycle.phases[multisiteCleanup]
		cleanup.duration += elapsed
		cleanup.addOperation(attempts)
		if err != nil {
			cleanup.fail(errorReason(err))
			return cleanupProofFailed
		}
		if truncated || len(keys) > maxOwnedKeys {
			cleanup.fail(reasonInternal)
			return cleanupProofFailed
		}
		for _, key := range keys {
			owned := ownedKey{Scope: endpoint.scope, Key: key}
			if _, exists := wanted[owned]; exists {
				cleanup.fail(reasonStillPresent)
				return cleanupProofPresent
			}
		}
	}
	return cleanupProofAbsent
}

type cleanupProofStatus int

const (
	cleanupProofAbsent cleanupProofStatus = iota
	cleanupProofPresent
	cleanupProofFailed
)

func ownsScope(keys []ownedKey, scope ownershipScope) bool {
	for _, owned := range keys {
		if owned.Scope == scope {
			return true
		}
	}
	return false
}

func (c *Collector) cleanupMultisiteState(ctx context.Context, cycle *multisiteCycle, processPending bool) {
	state := c.pendingOwnershipState
	result := cycle.phases[multisiteCleanup]
	if state.ReconciliationPending {
		result.wait(reasonReconciliationPending)
		return
	}
	if state.TerminalReason == reasonPayloadMismatch {
		// Preserve the categorical incident across every cleanup/restart cycle,
		// including bounded quarantine waits, until a later verified payload emits 0.
		mismatch := float64(1)
		cycle.payloadMismatch = &mismatch
	}
	if state.CleanupConfirmedAt == nil && state.CleanupQuarantinedAt != nil {
		horizon := c.multisiteCleanupHorizon()
		if state.CleanupDeleteAttempted {
			horizon += c.multisiteDestructiveRetryGrace()
		}
		if c.now().Sub(*state.CleanupQuarantinedAt) < horizon {
			result.wait(reasonQuarantinePending)
			return
		}
	}
	// A bucket may have become versioned after startup. Prove the destructive
	// delete contract again before either authenticated DELETE is sent.
	if !c.verifyMultisiteBucketUnversioned(ctx, c.client, c.Bucket, result) {
		return
	}
	if !c.verifyMultisiteBucketUnversioned(ctx, c.destinationClient, c.Destination.Bucket, result) {
		return
	}

	if state.hasActiveObject() {
		start := c.now()
		attempts, err := c.client.DeleteObject(ctx, c.Bucket, state.SourceKey)
		result.duration += c.now().Sub(start)
		result.addOperation(attempts)
		if err != nil {
			result.fail(errorReason(err))
			return
		}

		start = c.now()
		attempts, err = c.destinationClient.DeleteObject(ctx, c.Destination.Bucket, state.DestinationKey)
		result.duration += c.now().Sub(start)
		result.addOperation(attempts)
		if err != nil {
			result.fail(errorReason(err))
			return
		}

		start = c.now()
		sourceExists, report, err := c.client.ObjectExists(ctx, c.Bucket, state.SourceKey)
		result.duration += c.now().Sub(start)
		result.addOperations(report.operations, report.attempts)
		if err != nil {
			result.fail(errorReason(err))
			return
		}

		start = c.now()
		destinationExists, report, err := c.destinationClient.ObjectExists(
			ctx, c.Destination.Bucket, state.DestinationKey,
		)
		result.duration += c.now().Sub(start)
		result.addOperations(report.operations, report.attempts)
		if err != nil {
			result.fail(errorReason(err))
			return
		}
		if sourceExists || destinationExists {
			result.fail(reasonStillPresent)
			return
		}

		destinationOwned := ownedKey{Scope: ownershipDestination, Key: state.DestinationKey}
		sourceOwned := ownedKey{Scope: ownershipSource, Key: state.SourceKey}
		needed := 0
		if !state.ownsKey(sourceOwned) {
			needed++
		}
		if !state.ownsKey(destinationOwned) {
			needed++
		}
		if len(state.PendingKeys)+needed > maxOwnedKeys {
			result.fail(reasonInternal)
			return
		}
		if !state.ownsKey(destinationOwned) {
			state.PendingKeys = append(state.PendingKeys, destinationOwned)
		}
		if !state.ownsKey(sourceOwned) {
			state.PendingKeys = append(state.PendingKeys, sourceOwned)
		}
		state.SourceKey = ""
		state.DestinationKey = ""
		state.PayloadDigest = ""
		cleanupQuarantinedAt := c.now()
		state.CleanupQuarantinedAt = &cleanupQuarantinedAt
		if err := c.saveOwnership(state); err != nil {
			result.fail(reasonInternal)
			return
		}
		// Keep the old route's exact keys journalized through the configured
		// replication/delete horizon and reversed confirmation interval.
		result.wait(reasonQuarantinePending)
		return
	}

	if len(state.PendingKeys) > 0 {
		if !processPending {
			result.wait(reasonOrphanCleanupPending)
			return
		}
		if !c.cleanupMultisiteOwnedKeys(ctx, cycle, cleanupBatchSize) {
			return
		}
		if c.pendingOwnershipState == nil {
			result.succeed()
			return
		}
		if c.pendingOwnershipState.CleanupQuarantinedAt != nil {
			result.wait(reasonQuarantinePending)
		} else {
			result.wait(reasonOrphanCleanupPending)
		}
		return
	}

	result.succeed()
	if err := c.stateStore.clear(); err != nil {
		result.fail(reasonInternal)
		return
	}
	c.pendingOwnershipState = nil
}

func (c *Collector) failReplication(ctx context.Context, cycle *multisiteCycle, reason string) {
	cycle.phases[multisiteReplication].fail(reason)
	c.transitionMultisiteFailure(ctx, cycle, multisiteReplication, reason)
}

func (c *Collector) transitionMultisiteFailure(
	ctx context.Context,
	cycle *multisiteCycle,
	phase multisitePhase,
	reason string,
) {
	state := c.pendingOwnershipState
	state.Phase = string(multisiteCleanup)
	state.TerminalReason = reason
	if err := c.saveOwnership(state); err != nil {
		cycle.phases[phase].fail(reasonInternal)
	}
}

func (c *Collector) persistTransition(cycle *multisiteCycle, phase multisitePhase) bool {
	if err := c.saveOwnership(c.pendingOwnershipState); err != nil {
		cycle.phases[phase].fail(reasonInternal)
		return false
	}
	return true
}

func (c *Collector) setRPOStatus(cycle *multisiteCycle, elapsed time.Duration) {
	exceeded := float64(0)
	if elapsed >= time.Duration(c.RPOThresholdMS)*time.Millisecond {
		exceeded = 1
	}
	cycle.rpoExceeded = &exceeded
}

func (c *Collector) setDeleteStatus(cycle *multisiteCycle, elapsed time.Duration) {
	exceeded := float64(0)
	if elapsed >= time.Duration(c.DeleteThresholdMS)*time.Millisecond {
		exceeded = 1
	}
	cycle.deleteExceeded = &exceeded
}

func (c *Collector) cleanupPendingOwnershipObject(ctx context.Context) error {
	state := c.pendingOwnershipState
	if state == nil {
		return nil
	}
	if multisitePhase(state.Phase) != multisiteCleanup {
		state.Phase = string(multisiteCleanup)
		state.TerminalReason = reasonShutdownCleanup
		if err := c.saveOwnership(state); err != nil {
			return err
		}
	}
	cycle := newMultisiteCycle()
	c.cleanupMultisiteState(ctx, cycle, false)
	if state := c.pendingOwnershipState; state != nil && state.hasActiveObject() {
		return fmt.Errorf("multisite probe cleanup remains pending")
	}
	return nil
}
