// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type pendingStoreState struct {
	config  secretstore.Config
	release <-chan struct{}
	update  chan struct{}
	version uint64
	running bool
}

func (c *Controller) allocateDesiredVersion() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextDesired++
	if c.nextDesired == 0 {
		return 0, errors.New("jobmgr secrets: desired Store version wrapped")
	}
	return c.nextDesired, nil
}

func (c *Controller) retainPending(
	config secretstore.Config,
	version uint64,
	release <-chan struct{},
) {
	if c == nil || config == nil || config.ExposedKey() == "" || version == 0 {
		return
	}
	key := config.ExposedKey()
	c.mu.Lock()
	if c.projectionCtx == nil || c.projectionCtx.Err() != nil || c.commands == nil {
		c.mu.Unlock()
		return
	}
	state := c.pending[key]
	if state != nil && version < state.version {
		c.mu.Unlock()
		return
	}
	start := state == nil
	if start {
		state = &pendingStoreState{
			update:  make(chan struct{}, 1),
			running: true,
		}
		c.pending[key] = state
	}
	state.config = config
	state.release = release
	state.version = version
	ctx := c.projectionCtx
	if !start {
		if state.running {
			if len(state.update) == 0 {
				state.update <- struct{}{}
			}
		} else {
			state.running = true
			start = true
		}
	}
	c.mu.Unlock()
	if start {
		go c.pendingLoop(ctx, key, state)
	}
}

func (c *Controller) pendingLoop(
	ctx context.Context,
	key string,
	state *pendingStoreState,
) {
	for {
		c.mu.Lock()
		if c.pending[key] != state {
			c.mu.Unlock()
			return
		}
		release := state.release
		update := state.update
		version := state.version
		c.mu.Unlock()

		if release != nil {
			select {
			case <-release:
			case <-update:
				continue
			case <-ctx.Done():
				return
			}
		} else {
			// No release means there is no physical owner to await. Retry
			// through SubmitPreparedAndWait, which serializes the next attempt
			// on the resource lane; physical contention supplies a release.
			select {
			case <-update:
			case <-ctx.Done():
				return
			default:
			}
		}
		if !c.pendingCurrent(key, state, version) {
			continue
		}
		if err := c.retryPending(ctx, key, state, version); err != nil {
			if ctx.Err() != nil {
				return
			}
			jobmgr.ObserveDiagnostic(c.diagnostics, jobmgr.DiagnosticEvent{
				Level:      jobmgr.DiagnosticError,
				Name:       "secret Store pending retry failed",
				Resource:   key,
				Generation: c.epoch,
				Err:        err,
			})
			c.mu.Lock()
			if c.pending[key] != state {
				c.mu.Unlock()
				return
			}
			select {
			case <-state.update:
				c.mu.Unlock()
				continue
			default:
				state.running = false
				c.mu.Unlock()
				return
			}
		}
	}
}

func (c *Controller) pendingCurrent(
	key string,
	state *pendingStoreState,
	version uint64,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending[key] == state && state.version == version
}

func (c *Controller) retryPending(
	ctx context.Context,
	key string,
	state *pendingStoreState,
	version uint64,
) error {
	c.mu.Lock()
	if c.pending[key] != state || state.version != version || c.commands == nil {
		c.mu.Unlock()
		return nil
	}
	config := state.config
	commands := c.commands
	c.nextRetry++
	retry := c.nextRetry
	if retry == 0 {
		c.mu.Unlock()
		return errors.New("jobmgr secrets: pending retry identity wrapped")
	}
	c.mu.Unlock()

	if entry, ok := c.entry(key); ok {
		if entry.config.SourceTypePriority() > config.SourceTypePriority() ||
			entry.config.Hash() == config.Hash() &&
				entry.status == dyncfg.StatusRunning {
			c.clearPendingThrough(key, version)
			return nil
		}
	}
	kind, name, err := secretstore.ParseStoreKey(key)
	if err != nil {
		return err
	}
	stage, err := c.operations.prepare(storeOperationSpec{
		target: secretTarget{
			command: dyncfg.CommandUpdate,
			key:     key,
			kind:    kind,
			name:    name,
		},
		config:         config,
		expected:       c.store.Generation(key),
		mode:           storeOperationMutation,
		supersede:      true,
		desiredVersion: version,
	})
	if err != nil {
		return err
	}
	resourceID := secretResourceID(key)
	plan := c.planPendingRetry(resourceID, config, version, stage)
	return commands.SubmitPreparedAndWait(ctx, jobmgr.Request{
		UID:     fmt.Sprintf("jobmgr-secret-retry-%d-%d", c.epoch, retry),
		LaneKey: resourceID,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/secrets/retry",
	}, plan)
}

func (c *Controller) planPendingRetry(
	resourceID string,
	config secretstore.Config,
	version uint64,
	stage *PreparedStoreOperation,
) jobmgr.WorkPlan {
	key := config.ExposedKey()
	return jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim, jobmgr.DynCfgJobGraphClaim},
		NoResponse: true,
		Stage:      stage,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                resourceID,
			AllocateSuccessor: true,
			CompositeChildLaneConflict: func(lane string) bool {
				return c.dependencies.Affects(key, lane, true)
			},
			PrepareComposite: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (jobmgr.PreparedCompositeResourceTransaction, error) {
				prepared, err := c.preparePendingAttempt(config, version, current, scope, permit, stage)
				if prepared == nil {
					return nil, err
				}
				composite, ok := prepared.(jobmgr.PreparedCompositeResourceTransaction)
				if !ok {
					return nil, errors.Join(
						err,
						errors.New("jobmgr secrets: pending retry transaction is not composite"),
					)
				}
				return composite, err
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}
}

func (c *Controller) preparePendingAttempt(
	config secretstore.Config,
	version uint64,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	stage *PreparedStoreOperation,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	if permit.Valid() || !c.pendingVersion(config.ExposedKey(), version) {
		return c.noop(scope, current, mustSecretMessage(204, ""), nil, nil)
	}
	if entry, ok := c.entry(config.ExposedKey()); ok {
		if entry.config.SourceTypePriority() > config.SourceTypePriority() ||
			entry.config.Hash() == config.Hash() &&
				entry.status == dyncfg.StatusRunning {
			return c.noopWithCommit(
				scope,
				current,
				mustSecretMessage(204, ""),
				nil,
				nil,
				func() {
					c.clearPendingThrough(config.ExposedKey(), version)
				},
			)
		}
	}
	operation, err := takeStoreOperation(stage)
	if err != nil {
		return nil, err
	}
	defer operation.releaseUntransferred(&transaction, &resultErr)
	result := operation.result
	expected := c.store.Generation(config.ExposedKey())
	if (expected == 0) != (current == nil) ||
		(expected != 0) != scope.Current.Valid() {
		return nil, errors.New("jobmgr secrets: pending Store resource differs from active generation")
	}
	if result.expected != expected {
		result.retryable = true
		result.err = errors.New("jobmgr secrets: pending Store changed while preparation was staged")
	}
	if result.retryable {
		return c.prepareRetryableResult(scope, current, result, expected == 0)
	}
	return c.prepareStoreMutation(scope, current, operation, expected == 0)
}

func (c *Controller) prepareRetryableResult(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	result storeOperationResult,
	installFailure bool,
) (lifecycle.PreparedResourceTransaction, error) {
	entry := secretEntry{
		config: result.config,
		status: dyncfg.StatusFailed,
	}
	spec := preparedSecretSpec{
		scope:      scope,
		current:    current,
		result:     mustSecretMessage(503, "Secretstore configuration is still busy."),
		cleanup:    func() error { return nil },
		controller: c,
		commit: func() {
			c.retainPending(result.config, result.desiredVersion, result.release)
		},
	}
	if installFailure {
		spec.entry = &entry
		spec.cleanup = c.configCreateCleanup(entry)
	}
	return newPreparedSecretTransaction(spec)
}

func (c *Controller) pendingVersion(key string, version uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.pending[key]
	return state != nil && state.version == version
}

func (c *Controller) clearPendingThrough(key string, version uint64) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	state := c.pending[key]
	if state == nil || version != 0 && state.version > version {
		c.mu.Unlock()
		return
	}
	delete(c.pending, key)
	select {
	case state.update <- struct{}{}:
	default:
	}
	c.mu.Unlock()
}
