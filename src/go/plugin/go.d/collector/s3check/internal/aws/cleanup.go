// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/fairqueue"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

func (e *Engine) cleanupBacklog(
	ctx context.Context,
	limit int,
	operations *[]contract.OperationResult,
) (result contract.CleanupResult, retErr error) {
	defer func() {
		result.Pending = len(e.state.Entries)
		result.Backpressure = result.Pending >= e.queueCapacity
		if retErr == nil {
			retErr = e.journal.MutationError()
		}
	}()
	keys := make([]string, 0, len(e.state.Entries))
	for _, owned := range e.state.Entries {
		keys = append(keys, owned.Key)
	}
	selected, next := fairqueue.Select(keys, e.state.ActiveKey, e.state.CleanupCursor, limit)
	e.state.CleanupCursor = next
	for _, key := range selected {
		owned := e.find(key)
		if owned == nil {
			continue
		}
		advanceErr := e.advanceRetired(ctx, owned, operations)
		if err := e.journal.MutationError(); err != nil {
			return result, err
		}
		if advanceErr != nil {
			if errors.Is(advanceErr, errJournalPersistence) {
				return result, advanceErr
			}
			continue
		}
	}
	if len(e.state.Entries) == 0 {
		e.state.CleanupCursor = 0
	} else {
		e.state.CleanupCursor %= len(e.state.Entries)
		if len(selected) > 0 {
			if err := e.persist(); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func (e *Engine) advanceRetired(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) error {
	for range 6 {
		switch owned.Phase {
		case phasePutIntent, phaseReconcilePut:
			owned.Phase = phaseReconcilePut
			resolved, err := e.reconcilePut(ctx, owned, operations)
			if err != nil {
				return err
			}
			if !resolved {
				return nil
			}
		case phaseWaitObject:
			proceed, _, err := e.observeObject(ctx, owned, operations, false)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}
		case phaseDeleteIntent:
			if owned.DeleteAttemptAt == nil {
				now := e.now().UTC()
				owned.DeleteAttemptAt = &now
				if err := e.persist(); err != nil {
					return err
				}
			}
			var deleted s3client.DeleteResult
			// This is the same conditional, retry-safe delete used by createMarker.
			_, err := e.call(
				ctx,
				operations,
				contract.EndpointSource,
				contract.OperationCleanup,
				func(callCtx context.Context) error {
					var callErr error
					deleted, callErr = e.source.Delete(
						callCtx, e.sourceBucket, owned.Key, s3client.DeleteOptions{
							IfMatch: owned.SourceObjectETag,
						},
					)
					return callErr
				},
			)
			if err != nil {
				owned.Phase = phaseReconcileDelete
				return e.persist()
			}
			if !deleted.DeleteMarker || deleted.VersionID == "" {
				owned.Phase = phaseReconcileDelete
				return e.persist()
			}
			owned.SourceMarkerID = deleted.VersionID
			owned.MeasureDelete = false
			owned.Phase = phaseWaitMarker
			if err := e.persist(); err != nil {
				return err
			}
		case phaseReconcileDelete:
			resolved, err := e.reconcileDelete(ctx, owned, operations)
			if err != nil || !resolved {
				return err
			}
		case phaseWaitMarker:
			proceed, _, err := e.observeMarker(ctx, owned, operations, false)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}
		case phaseExactCleanup:
			removed, err := e.cleanupExact(ctx, owned, operations)
			if err != nil || !removed {
				return err
			}
			e.remove(owned.Key)
			return e.persist()
		case phaseBlocked:
			return nil
		default:
			return errors.New("invalid AWS cleanup phase")
		}
	}
	return errors.New("AWS cleanup transition budget exhausted")
}

func (e *Engine) cleanupExact(
	ctx context.Context,
	owned *entry,
	operations *[]contract.OperationResult,
) (bool, error) {
	if !owned.ObjectSeen || !owned.MarkerSeen ||
		owned.SourceObjectID == "" || owned.DestinationObjectID == "" ||
		owned.SourceMarkerID == "" || owned.DestinationMarkerID == "" {
		return false, errors.New("AWS exact cleanup lacks proven replicated identities")
	}
	targets := []struct {
		client   s3client.Client
		bucket   string
		endpoint contract.Endpoint
		version  string
	}{
		{e.source, e.sourceBucket, contract.EndpointSource, owned.SourceObjectID},
		{e.destination, e.destinationBucket, contract.EndpointDestination, owned.DestinationObjectID},
		{e.source, e.sourceBucket, contract.EndpointSource, owned.SourceMarkerID},
		{e.destination, e.destinationBucket, contract.EndpointDestination, owned.DestinationMarkerID},
	}
	for _, target := range targets {
		_, err := e.call(
			ctx,
			operations,
			target.endpoint,
			contract.OperationCleanup,
			func(callCtx context.Context) error {
				_, callErr := target.client.Delete(
					callCtx, target.bucket, owned.Key, s3client.DeleteOptions{
						VersionID: target.version,
					},
				)
				return callErr
			},
		)
		if err != nil {
			return false, err
		}
	}

	sourceVersions, err := e.listExact(
		ctx, e.source, e.sourceBucket, contract.EndpointSource, contract.OperationCleanup, owned.Key, operations,
	)
	if err != nil {
		return false, err
	}
	destinationVersions, err := e.listExact(
		ctx,
		e.destination,
		e.destinationBucket,
		contract.EndpointDestination,
		contract.OperationCleanup,
		owned.Key,
		operations,
	)
	if err != nil {
		return false, err
	}
	return len(sourceVersions) == 0 && len(destinationVersions) == 0, nil
}

func (e *Engine) listExact(
	ctx context.Context,
	client s3client.Client,
	bucket string,
	endpoint contract.Endpoint,
	operation contract.Operation,
	key string,
	operations *[]contract.OperationResult,
) ([]s3client.Version, error) {
	var versions []s3client.Version
	var keyMarker, versionMarker string
	for range maxListPages {
		var page s3client.VersionPage
		_, err := e.call(ctx, operations, endpoint, operation, func(callCtx context.Context) error {
			var callErr error
			page, callErr = client.ListVersions(
				callCtx, bucket, key, keyMarker, versionMarker, listPageSize,
			)
			return callErr
		})
		if err != nil {
			return nil, err
		}
		for _, version := range page.Versions {
			if version.Key != key {
				continue
			}
			if version.VersionID == "" {
				return nil, ownershipError("exact version has no ID")
			}
			versions = append(versions, version)
			if len(versions) > maxListedVersions {
				return nil, fmt.Errorf(
					"%w: exact-key version count exceeds %d",
					errOwnershipInvariant,
					maxListedVersions,
				)
			}
		}
		if !page.Truncated {
			return versions, nil
		}
		if page.NextKeyMarker == "" && page.NextVersionIDMarker == "" {
			return nil, ownershipError("truncated version page has no continuation markers")
		}
		keyMarker = page.NextKeyMarker
		versionMarker = page.NextVersionIDMarker
	}
	return nil, fmt.Errorf("%w: exact-key version listing exceeds %d pages", errOwnershipInvariant, maxListPages)
}

func (e *Engine) find(key string) *entry {
	for index := range e.state.Entries {
		if e.state.Entries[index].Key == key {
			return &e.state.Entries[index]
		}
	}
	return nil
}

func splitVersions(versions []s3client.Version) (objects, markers []s3client.Version) {
	for _, version := range versions {
		switch version.Kind {
		case s3client.VersionObject:
			objects = append(objects, version)
		case s3client.VersionDeleteMarker:
			markers = append(markers, version)
		}
	}
	return objects, markers
}
