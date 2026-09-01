// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

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
	selected, next := fairqueue.Select(keys, "", e.state.CleanupCursor, limit)
	e.state.CleanupCursor = next
	for _, key := range selected {
		index := e.entryIndex(key)
		if index < 0 {
			continue
		}
		owned := &e.state.Entries[index]
		_, deleteErr := e.call(ctx, operations, contract.OperationCleanup, func(callCtx context.Context) error {
			_, err := e.client.Delete(callCtx, e.bucket, owned.Key, s3client.DeleteOptions{})
			return err
		})
		if deleteErr != nil {
			continue
		}

		absent := false
		_, getErr := e.call(ctx, operations, contract.OperationCleanup, func(callCtx context.Context) error {
			_, err := e.client.Get(callCtx, e.bucket, owned.Key, "", probe.PayloadBytes)
			if errors.Is(err, s3client.ErrObjectNotFound) {
				absent = true
				return nil
			}
			return err
		})
		if getErr != nil {
			continue
		}
		if !absent {
			owned.AbsentObservedAt = nil
			if err := e.persist(); err != nil {
				return result, err
			}
			continue
		}
		if !owned.PutConfirmed {
			now := e.now().UTC()
			if owned.AbsentObservedAt == nil {
				owned.AbsentObservedAt = &now
				if err := e.persist(); err != nil {
					return result, err
				}
				continue
			}
			quietFor := max(e.updateEvery, e.requestTimeout)
			if now.Sub(*owned.AbsentObservedAt) < quietFor {
				continue
			}
		}

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
