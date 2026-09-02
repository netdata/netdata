// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

func (e *Engine) startProbe(
	ctx context.Context,
	operations *[]contract.OperationResult,
	rules []s3client.ReplicationRule,
) (*contract.ProbeResult, error) {
	object, err := e.generator.Next()
	if err != nil {
		return e.persistTerminal(contract.FailedProbe(contract.ReasonInternal)), nil
	}
	if err := e.validateReplicationRules(rules, object.Key); err != nil {
		return nil, err
	}
	owned := entry{
		Key:       object.Key,
		Digest:    object.Digest,
		CreatedAt: e.now().UTC(),
		Phase:     phasePutIntent,
	}
	e.state.Entries = append(e.state.Entries, owned)
	e.state.ActiveKey = owned.Key
	if err := e.persist(); err != nil {
		e.remove(owned.Key)
		return e.persistTerminal(contract.FailedProbe(contract.ReasonOwnership)), nil
	}

	var put s3client.PutResult
	_, err = e.call(
		ctx,
		operations,
		contract.EndpointSource,
		contract.OperationPut,
		func(callCtx context.Context) error {
			var callErr error
			put, callErr = e.source.Put(
				callCtx,
				e.sourceBucket,
				object.Key,
				object.Payload,
				s3client.PutOptions{
					IfNoneMatch: true,
				},
			)
			return callErr
		},
	)
	active := e.active()
	if active == nil {
		return e.persistTerminal(contract.FailedProbe(contract.ReasonInternal)), nil
	}
	if err != nil {
		active.Phase = phaseReconcilePut
		e.retire(active)
		return e.persistTerminal(contract.FailedProbe(contract.ReasonRequest)), nil
	}
	if put.VersionID == "" || put.ETag == "" {
		active.Phase = phaseReconcilePut
		e.retire(active)
		return e.persistTerminal(contract.FailedProbe(contract.ReasonOwnership)), nil
	}
	now := e.now().UTC()
	active.SourceObjectID = put.VersionID
	active.SourceObjectETag = put.ETag
	active.PutAt = &now
	active.MeasureWrite = true
	active.Phase = phaseWaitObject
	if err := e.persist(); err != nil {
		e.retire(active)
		return e.persistTerminal(contract.FailedProbe(contract.ReasonOwnership)), nil
	}
	return e.advanceActive(ctx, active, operations), nil
}

func (e *Engine) advanceActive(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) *contract.ProbeResult {
	for range 6 {
		switch owned.Phase {
		case phasePutIntent:
			owned.Phase = phaseReconcilePut
			e.retire(owned)
			return e.persistTerminal(contract.FailedProbe(contract.ReasonRequest))
		case phaseReconcilePut:
			key := owned.Key
			resolved, err := e.reconcilePut(ctx, owned, operations)
			if err != nil {
				result := e.persistTerminal(contract.FailedProbe(reasonForError(err)))
				return result
			}
			if !resolved {
				if e.find(key) == nil {
					if e.state.LastTerminal != nil {
						return contract.CloneProbe(e.state.LastTerminal)
					}
					return e.persistTerminal(contract.FailedProbe(contract.ReasonRequest))
				}
				return waitingProbe(owned, e.now().UTC(), e.writeObjective, e.deleteObjective)
			}
		case phaseWaitObject:
			proceed, result, err := e.observeObject(ctx, owned, operations, true)
			if err != nil {
				return e.persistTerminal(contract.FailedProbe(reasonForError(err)))
			}
			if result != nil {
				return result
			}
			if !proceed {
				return waitingProbe(owned, e.now().UTC(), e.writeObjective, e.deleteObjective)
			}
		case phaseDeleteIntent:
			if result := e.createMarker(ctx, owned, operations); result != nil {
				return withPayloadComparison(owned, result)
			}
		case phaseReconcileDelete:
			resolved, err := e.reconcileDelete(ctx, owned, operations)
			if err != nil {
				result := e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(reasonForError(err))))
				return result
			}
			if !resolved {
				return waitingProbe(owned, e.now().UTC(), e.writeObjective, e.deleteObjective)
			}
		case phaseWaitMarker:
			proceed, result, err := e.observeMarker(ctx, owned, operations, true)
			if err != nil {
				return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(reasonForError(err))))
			}
			if result != nil {
				return withPayloadComparison(owned, result)
			}
			if !proceed {
				return waitingProbe(owned, e.now().UTC(), e.writeObjective, e.deleteObjective)
			}
		case phaseExactCleanup:
			result := successProbe(owned, e.writeObjective, e.deleteObjective)
			removed, err := e.cleanupExact(ctx, owned, operations)
			if err != nil {
				e.retire(owned)
				return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonCleanup)))
			}
			if !removed {
				return waitingProbe(owned, e.now().UTC(), e.writeObjective, e.deleteObjective)
			}
			e.remove(owned.Key)
			result = e.persistTerminal(result)
			return result
		case phaseBlocked:
			e.retire(owned)
			return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)))
		default:
			return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)))
		}
	}
	return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonInternal)))
}

func (e *Engine) reconcilePut(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) (bool, error) {
	versions, err := e.listExact(
		ctx, e.source, e.sourceBucket, contract.EndpointSource, contract.OperationReconcile, owned.Key, operations,
	)
	if err != nil {
		return false, err
	}
	objects, markers := splitVersions(versions)
	if len(markers) != 0 || len(objects) > 1 {
		owned.Phase = phaseBlocked
		_ = e.persist()
		return false, ownershipError("unexpected source versions while reconciling PUT")
	}
	if len(objects) == 0 {
		destinationVersions, listErr := e.listExact(
			ctx,
			e.destination,
			e.destinationBucket,
			contract.EndpointDestination,
			contract.OperationReconcile,
			owned.Key,
			operations,
		)
		if listErr != nil {
			return false, listErr
		}
		if len(destinationVersions) != 0 {
			owned.Phase = phaseBlocked
			_ = e.persist()
			return false, ownershipError("destination version exists without its source version")
		}
		now := e.now().UTC()
		if owned.PutAbsentSince == nil {
			owned.PutAbsentSince = &now
			return false, e.persist()
		}
		quietFor := e.updateEvery
		if requestTimeout := max(e.sourceRequestTimeout, e.destinationRequestTimeout); quietFor < requestTimeout {
			quietFor = requestTimeout
		}
		if now.Sub(owned.CreatedAt) < e.writeTimeout || now.Sub(*owned.PutAbsentSince) < quietFor {
			return false, nil
		}
		e.remove(owned.Key)
		return false, e.persist()
	}
	owned.PutAbsentSince = nil

	var got s3client.GetResult
	_, err = e.call(
		ctx,
		operations,
		contract.EndpointSource,
		contract.OperationReconcile,
		func(callCtx context.Context) error {
			var callErr error
			got, callErr = e.source.Get(
				callCtx, e.sourceBucket, owned.Key, objects[0].VersionID, probe.PayloadBytes,
			)
			return callErr
		},
	)
	if err != nil {
		return false, err
	}
	if got.VersionID == "" || got.ETag == "" || probe.Digest(got.Payload) != owned.Digest {
		owned.Phase = phaseBlocked
		_ = e.persist()
		return false, ownershipError("cannot prove reconciled source object ownership")
	}
	owned.SourceObjectID = got.VersionID
	owned.SourceObjectETag = got.ETag
	owned.MeasureWrite = false
	owned.Phase = phaseWaitObject
	return true, e.persist()
}

func (e *Engine) observeObject(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
	active bool,
) (bool, *contract.ProbeResult, error) {
	var got s3client.GetResult
	_, err := e.call(
		ctx, operations, contract.EndpointDestination, contract.OperationWriteVisibility,
		func(callCtx context.Context) error {
			var callErr error
			got, callErr = e.destination.Get(
				callCtx, e.destinationBucket, owned.Key, "", probe.PayloadBytes,
			)
			return callErr
		},
	)
	if errors.Is(err, s3client.ErrObjectNotFound) {
		if active && e.writeTimedOut(owned) {
			e.retire(owned)
			result := contract.FailedProbe(contract.ReasonVisibilityTimeout)
			result.WriteVisibility = writeResult(owned, e.now().UTC(), e.writeObjective)
			result = e.persistTerminal(result)
			return false, result, nil
		}
		return false, nil, nil
	}
	if err != nil {
		if active {
			e.retire(owned)
			result := e.persistTerminal(contract.FailedProbe(contract.ReasonRequest))
			return false, result, nil
		}
		return false, nil, err
	}
	if got.VersionID == "" || probe.Digest(got.Payload) != owned.Digest {
		owned.Phase = phaseBlocked
		if active {
			e.retire(owned)
			result := contract.FailedProbe(contract.ReasonPayloadMismatch)
			result.PayloadCompared = true
			result.PayloadMismatch = true
			result = e.persistTerminal(result)
			return false, result, nil
		}
		_ = e.persist()
		return false, nil, ownershipError("destination object payload or version does not match ownership")
	}
	now := e.now().UTC()
	owned.DestinationObjectID = got.VersionID
	owned.VisibleAt = &now
	owned.ObjectSeen = true
	owned.Phase = phaseDeleteIntent
	if err := e.persist(); err != nil && active {
		e.retire(owned)
		return false, e.persistTerminal(
			withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)),
		), nil
	} else if err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

func (e *Engine) createMarker(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) *contract.ProbeResult {
	if owned.DeleteAttemptAt == nil {
		now := e.now().UTC()
		owned.DeleteAttemptAt = &now
		if err := e.persist(); err != nil {
			e.retire(owned)
			return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)))
		}
	}
	var deleted s3client.DeleteResult
	// IfMatch makes a retry idempotent: after a successful delete, the current
	// delete marker cannot match the original object's ETag. AWS requires both
	// s3:DeleteObject and s3:GetObject for this conditional request.
	_, err := e.call(
		ctx,
		operations,
		contract.EndpointSource,
		contract.OperationDelete,
		func(callCtx context.Context) error {
			var callErr error
			deleted, callErr = e.source.Delete(
				callCtx,
				e.sourceBucket,
				owned.Key,
				s3client.DeleteOptions{
					IfMatch: owned.SourceObjectETag,
				},
			)
			return callErr
		},
	)
	if err != nil {
		owned.Phase = phaseReconcileDelete
		owned.MeasureDelete = false
		_ = e.persist()
		return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonRequest)))
	}
	if !deleted.DeleteMarker || deleted.VersionID == "" {
		owned.Phase = phaseReconcileDelete
		owned.MeasureDelete = false
		_ = e.persist()
		return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)))
	}
	owned.SourceMarkerID = deleted.VersionID
	deletedAt := e.now().UTC()
	owned.DeleteAt = &deletedAt
	owned.MeasureDelete = true
	owned.Phase = phaseWaitMarker
	if err := e.persist(); err != nil {
		e.retire(owned)
		return e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)))
	}
	return nil
}

func (e *Engine) reconcileDelete(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) (bool, error) {
	versions, err := e.listExact(
		ctx, e.source, e.sourceBucket, contract.EndpointSource, contract.OperationReconcile, owned.Key, operations,
	)
	if err != nil {
		return false, err
	}
	objects, markers := splitVersions(versions)
	if len(objects) != 1 || objects[0].VersionID != owned.SourceObjectID || len(markers) > 1 {
		owned.Phase = phaseBlocked
		_ = e.persist()
		return false, ownershipError("unexpected source versions while reconciling DELETE")
	}
	if len(markers) == 0 {
		owned.Phase = phaseDeleteIntent
		return true, e.persist()
	}
	owned.SourceMarkerID = markers[0].VersionID
	owned.MeasureDelete = false
	owned.Phase = phaseWaitMarker
	return true, e.persist()
}

func (e *Engine) observeMarker(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
	active bool,
) (bool, *contract.ProbeResult, error) {
	_, err := e.call(
		ctx, operations, contract.EndpointDestination, contract.OperationDeleteVisibility,
		func(callCtx context.Context) error {
			_, callErr := e.destination.Get(
				callCtx, e.destinationBucket, owned.Key, "", probe.PayloadBytes,
			)
			return callErr
		},
	)
	if err == nil {
		if active && e.deleteTimedOut(owned) {
			e.retire(owned)
			result := contract.FailedProbe(contract.ReasonDeleteTimeout)
			result.WriteVisibility = writeResult(owned, e.now().UTC(), e.writeObjective)
			result.DeleteVisibility = deleteResult(owned, e.now().UTC(), e.deleteObjective)
			result = withPayloadComparison(owned, result)
			result = e.persistTerminal(result)
			return false, result, nil
		}
		return false, nil, nil
	}
	if !errors.Is(err, s3client.ErrObjectNotFound) {
		if active {
			e.retire(owned)
			result := e.persistTerminal(withPayloadComparison(owned, contract.FailedProbe(contract.ReasonRequest)))
			return false, result, nil
		}
		return false, nil, err
	}

	versions, listErr := e.listExact(
		ctx,
		e.destination,
		e.destinationBucket,
		contract.EndpointDestination,
		contract.OperationReconcile,
		owned.Key,
		operations,
	)
	if listErr != nil {
		if active {
			return false, e.persistTerminal(
				withPayloadComparison(owned, contract.FailedProbe(reasonForError(listErr))),
			), nil
		}
		return false, nil, listErr
	}
	objects, markers := splitVersions(versions)
	if len(objects) != 1 || objects[0].VersionID != owned.DestinationObjectID ||
		len(markers) != 1 || !markers[0].IsLatest {
		if active && e.deleteTimedOut(owned) {
			e.retire(owned)
			result := e.persistTerminal(
				withPayloadComparison(owned, contract.FailedProbe(contract.ReasonDeleteTimeout)),
			)
			return false, result, nil
		}
		return false, nil, nil
	}
	owned.DestinationMarkerID = markers[0].VersionID
	owned.MarkerSeen = true
	markerAt := e.now().UTC()
	owned.MarkerAt = &markerAt
	owned.Phase = phaseExactCleanup
	if err := e.persist(); err != nil && active {
		e.retire(owned)
		return false, e.persistTerminal(
			withPayloadComparison(owned, contract.FailedProbe(contract.ReasonOwnership)),
		), nil
	} else if err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

func (e *Engine) writeTimedOut(owned *entry) bool {
	base := owned.CreatedAt
	if owned.PutAt != nil {
		base = *owned.PutAt
	}
	return e.now().UTC().Sub(base) >= e.writeTimeout
}

func (e *Engine) deleteTimedOut(owned *entry) bool {
	if owned.DeleteAttemptAt == nil {
		return false
	}
	return e.now().UTC().Sub(*owned.DeleteAttemptAt) >= e.deleteTimeout
}

func (e *Engine) call(
	parent context.Context,
	operations *[]contract.OperationResult,
	endpoint contract.Endpoint,
	name contract.Operation,
	fn func(context.Context) error,
) (time.Duration, error) {
	requestTimeout := e.sourceRequestTimeout
	if endpoint == contract.EndpointDestination {
		requestTimeout = e.destinationRequestTimeout
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	started := time.Now()
	err := fn(ctx)
	duration := time.Since(started)
	cancel()
	if operations != nil {
		status := contract.StatusSuccess
		reason := contract.ReasonNone
		if err != nil && !errors.Is(err, s3client.ErrObjectNotFound) {
			status = contract.StatusFailed
			reason = contract.ReasonRequest
		}
		*operations = append(*operations, contract.OperationResult{
			Name:     name,
			Endpoint: endpoint,
			Status:   status,
			Reason:   reason,
			Duration: duration,
			Err:      err,
		})
	}
	return duration, err
}
