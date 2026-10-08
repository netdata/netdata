// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// One accepted revision owns acquisition until success, failure or supersession.
// Provider preparation never occupies the kernel resource lane.
type pendingStoreState struct {
	config  secretstore.Config
	version uint64
	ctx     context.Context
	cancel  context.CancelFunc
	release <-chan struct{}
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

func (c *Controller) acceptedVersion(key string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[key].version
}

func (c *Controller) pendingAcceptedConfig(key string) secretstore.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if !c.pendingAcceptedLocked(key, entry) {
		return nil
	}
	return cloneSecretConfig(entry.config)
}

func (c *Controller) pendingAcceptedVersion(key string, version uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	return entry.version == version && c.pendingAcceptedLocked(key, entry)
}

func (c *Controller) pendingAcceptedLocked(key string, entry secretEntry) bool {
	state := c.pending[key]
	return entry.status == dyncfg.StatusAccepted && entry.config.SourceType() == confgroup.TypeDyncfg &&
		state != nil && state.version == entry.version && state.ctx.Err() == nil
}

// retainPending installs dormant accepted work, or records its next release
// event. Only successful publication may start a newly accepted owner.
func (c *Controller) retainPending(config secretstore.Config, version uint64, release <-chan struct{}) {
	if config == nil || version == 0 {
		return
	}
	key := config.ExposedKey()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.projectionCtx == nil || c.projectionCtx.Err() != nil || c.commands == nil {
		return
	}
	if state := c.pending[key]; state != nil {
		if state.version == version {
			state.release = release
			return
		}
		if state.version > version {
			return
		}
		state.cancel()
	}
	ctx, cancel := context.WithCancel(c.projectionCtx)
	c.pending[key] = &pendingStoreState{
		config:  cloneSecretConfig(config),
		version: version,
		ctx:     ctx,
		cancel:  cancel,
		release: release,
	}
}

func (c *Controller) startPending(key string, version uint64) {
	c.mu.Lock()
	state := c.pending[key]
	if state == nil || state.version != version || state.running || state.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	state.running = true
	c.mu.Unlock()
	go c.pendingLoop(key, state)
}

func (c *Controller) pendingLoop(key string, state *pendingStoreState) {
	defer state.cancel()
	for {
		c.mu.Lock()
		if c.pending[key] != state {
			c.mu.Unlock()
			return
		}
		release := state.release
		c.mu.Unlock()
		if release != nil {
			select {
			case <-release:
			case <-state.ctx.Done():
				return
			}
		}
		if err := c.retryPending(key, state); err != nil {
			if state.ctx.Err() == nil {
				jobmgr.ObserveDiagnostic(c.diagnostics, jobmgr.DiagnosticEvent{
					Level:      jobmgr.DiagnosticError,
					Name:       "secret Store pending retry failed",
					Resource:   key,
					Generation: c.epoch,
					Err:        err,
				})
			}
			return
		}
	}
}

func (c *Controller) retryPending(key string, state *pendingStoreState) error {
	c.mu.Lock()
	if c.pending[key] != state || c.commands == nil || state.ctx.Err() != nil {
		c.mu.Unlock()
		return context.Canceled
	}
	commands, projectionCtx := c.commands, c.projectionCtx
	c.nextRetry++
	retry := c.nextRetry
	c.mu.Unlock()
	if retry == 0 {
		return errors.New("jobmgr secrets: pending retry identity wrapped")
	}
	config := state.config
	stage, err := c.operations.prepare(storeOperationSpec{
		target: secretTarget{
			command: dyncfg.CommandUpdate,
			key:     key,
			kind:    config.Kind(),
			name:    config.Name(),
		},
		config:          config,
		expected:        c.store.Generation(key),
		mode:            storeOperationMutation,
		desiredVersion:  state.version,
		acceptedVersion: state.version,
	})
	if err != nil {
		return err
	}
	defer stage.Release()
	stage.Start()
	select {
	case <-stage.Ready():
	case <-state.ctx.Done():
		stage.Cancel(context.Cause(state.ctx))
		return context.Cause(state.ctx)
	}
	if state.ctx.Err() != nil {
		stage.Cancel(context.Cause(state.ctx))
		return context.Cause(state.ctx)
	}
	resourceID := secretResourceID(key)
	// Completion may remove/cancel this owner in Apply. Its command lifetime
	// belongs to the projection, so that settlement cannot cancel its own reply.
	return commands.SubmitPreparedAndWait(projectionCtx, jobmgr.Request{
		UID:     fmt.Sprintf("jobmgr-secret-retry-%d-%d", c.epoch, retry),
		LaneKey: resourceID,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/secrets/retry",
	}, c.planPendingRetry(resourceID, config, state.version, stage))
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
	entry, exists := c.entry(config.ExposedKey())
	if !exists || entry.version != version || c.store.Generation(config.ExposedKey()) != 0 {
		return c.noop(scope, current, mustSecretMessage(204, ""), nil, nil)
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
		return c.prepareAcceptedRetry(scope, current, result)
	}
	return c.prepareStoreMutation(scope, current, operation, true)
}

func (c *Controller) prepareAcceptedRetry(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	result storeOperationResult,
) (lifecycle.PreparedResourceTransaction, error) {
	return c.noopWithCommit(scope, current, mustSecretMessage(202, ""), nil, nil, func() {
		c.retainPending(result.config, result.desiredVersion, result.release)
	})
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
	state.cancel()
	c.mu.Unlock()
}
