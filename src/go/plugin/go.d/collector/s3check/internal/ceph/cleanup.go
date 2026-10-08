// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/fairqueue"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

func (e *Engine) cleanupBacklog(
	ctx context.Context,
	limit int,
	operations *[]contract.OperationResult,
) (contract.CleanupResult, error) {
	result := contract.CleanupResult{}
	keys := make([]string, 0, len(e.state.Entries))
	for _, owned := range e.state.Entries {
		keys = append(keys, owned.Key)
	}
	selected, next := fairqueue.Select(keys, e.state.ActiveKey, e.state.CleanupCursor, limit)
	e.state.CleanupCursor = next
	for _, key := range selected {
		index := e.entryIndex(key)
		if index < 0 {
			continue
		}
		owned := &e.state.Entries[index]
		if owned.Phase != phaseCleanup {
			continue
		}
		now := e.now().UTC()

		_, sourceDeleteErr := e.call(
			ctx,
			operations,
			contract.EndpointSource,
			contract.OperationCleanup,
			func(callCtx context.Context) error {
				_, err := e.source.Delete(callCtx, e.sourceBucket, owned.Key, s3client.DeleteOptions{})
				return err
			},
		)
		if sourceDeleteErr == nil && owned.DeleteAt == nil {
			owned.DeleteAt = &now
			if deadline := now.Add(e.deleteTimeout); deadline.After(owned.CleanupAfter) {
				owned.CleanupAfter = deadline
			}
			if err := e.persist(); err != nil {
				return result, err
			}
		}
		_, destinationDeleteErr := e.call(
			ctx,
			operations,
			contract.EndpointDestination,
			contract.OperationCleanup,
			func(callCtx context.Context) error {
				_, err := e.destination.Delete(callCtx, e.destinationBucket, owned.Key, s3client.DeleteOptions{})
				return err
			},
		)
		if sourceDeleteErr != nil || destinationDeleteErr != nil {
			continue
		}

		sourceAbsent, sourceErr := e.absent(
			ctx,
			operations,
			e.source,
			e.sourceBucket,
			contract.EndpointSource,
			owned.Key,
		)
		destinationAbsent, destinationErr := e.absent(
			ctx,
			operations,
			e.destination,
			e.destinationBucket,
			contract.EndpointDestination,
			owned.Key,
		)
		if sourceErr != nil || destinationErr != nil {
			continue
		}
		if !sourceAbsent || !destinationAbsent || now.Before(owned.CleanupAfter) {
			continue
		}

		// CleanupAfter covers ambiguous source requests and both replication
		// windows. Exact absence at both endpoints is the bounded final ownership
		// confirmation. A pre-publication persist failure leaves a conservative
		// stale entry; post-publication uncertainty poisons the current runtime.
		e.state.Entries = append(e.state.Entries[:index], e.state.Entries[index+1:]...)
		if err := e.persist(); err != nil {
			return result, err
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
	result.Pending = len(e.state.Entries)
	result.Backpressure = result.Pending >= e.queueCapacity
	return result, nil
}

func (e *Engine) absent(
	ctx context.Context,
	operations *[]contract.OperationResult,
	client s3client.Client,
	bucket string,
	endpoint contract.Endpoint,
	key string,
) (bool, error) {
	absent := false
	_, err := e.call(ctx, operations, endpoint, contract.OperationCleanup, func(callCtx context.Context) error {
		_, callErr := client.Get(callCtx, bucket, key, "", probe.PayloadBytes)
		if errors.Is(callErr, s3client.ErrObjectNotFound) {
			absent = true
			return nil
		}
		return callErr
	})
	return absent, err
}
