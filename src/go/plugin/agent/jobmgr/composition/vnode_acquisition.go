// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

const vnodeRetryInterval = 10 * time.Second
const vnodeRefreshInterval = time.Hour

type vnodeWorker struct {
	token  *agentdiscovery.AcquisitionToken
	cancel context.CancelFunc
}

// Acquisition belongs to the run, independently of attached metric jobs. Only
// short result commits use the vnode transaction lane; network I/O never does.
type vnodeAcquisition struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	commands jobmgr.PreparedCommandPort
	workers  map[string]vnodeWorker
	wg       sync.WaitGroup
	done     chan struct{}
	stopped  bool
}

func (vb *vnodeBinding) startAcquisition(ctx context.Context, commands jobmgr.PreparedCommandPort) {
	if vb.acquirer == nil {
		return
	}
	a := &vb.acquisition
	a.mu.Lock()
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.commands = commands
	a.workers = make(map[string]vnodeWorker)
	a.done = make(chan struct{})
	a.mu.Unlock()
	for _, entry := range vb.config.Entries() {
		vb.syncAcquisition(entry.ID)
	}
}
func (vb *vnodeBinding) syncAcquisition(name string) {
	a := &vb.acquisition
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil || a.stopped {
		return
	}
	token, config := vb.config.Acquisition(name)
	old, exists := a.workers[name]
	if exists && old.token == token {
		return
	}
	if exists {
		old.cancel()
		delete(a.workers, name)
	}
	if token == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.workers[name] = vnodeWorker{token: token, cancel: cancel}
	a.wg.Add(1)
	go func() { defer a.wg.Done(); vb.acquireLoop(ctx, a.commands, name, token, config) }()
}
func (vb *vnodeBinding) stopAcquisition() {
	a := &vb.acquisition
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil || a.stopped {
		return
	}
	a.stopped = true
	a.cancel()
	go func() { a.wg.Wait(); close(a.done) }()
}
func (vb *vnodeBinding) waitAcquisition(ctx context.Context) error {
	a := &vb.acquisition
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (vb *vnodeBinding) acquireLoop(ctx context.Context, commands jobmgr.PreparedCommandPort, name string, token *agentdiscovery.AcquisitionToken, config vnodes.SNMPConfig) {
	for ctx.Err() == nil {
		metadata, err := vb.acquirer.Acquire(ctx, config.Copy())
		if ctx.Err() != nil {
			return
		}
		if metadata == nil && err == nil {
			err = errors.New("acquisition returned no identity")
		}
		publishErr := vb.publishAcquisition(ctx, commands, name, token, metadata, err != nil)
		interval := vnodeRefreshInterval
		current, exists := vb.config.Authored(name)
		if err != nil || publishErr != nil || exists && current.Failed {
			interval = vnodeRetryInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (vb *vnodeBinding) publishAcquisition(ctx context.Context, commands jobmgr.PreparedCommandPort, name string, token *agentdiscovery.AcquisitionToken, metadata *vnodes.Metadata, failed bool) error {
	result := mustDynCfgMessage(204, "")
	plan := jobmgr.WorkPlan{Claims: []string{joboutput.DynCfgJobGraphClaim}, NoResponse: true, Transaction: &jobmgr.ResourceTransactionPlan{
		ID: "vnode:" + name,
		Prepare: func(_ context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
			if current != nil || scope.Current.Valid() || scope.Successor.Valid() || permit.Valid() {
				return nil, errors.New("invalid vnode acquisition transaction scope")
			}
			prepared, err := vb.config.PrepareMetadata(token, metadata, failed)
			if errors.Is(err, agentdiscovery.ErrVNodeRevision) {
				return vb.noop(scope, result, nil)
			}
			if err != nil {
				return nil, err
			}
			entry, ok := vb.config.Authored(name)
			if !ok {
				return nil, agentdiscovery.ErrVNodeRevision
			}
			cleanup := vb.configCreateCleanup(entry.Config)
			return newPreparedVNodeTransaction(scope, prepared, result, func() error {
				if err := prepared.MetadataError(); err != nil {
					jobmgr.ObserveDiagnostic(vb.diagnostics, jobmgr.DiagnosticEvent{
						Level: jobmgr.DiagnosticWarning, Name: "vnode acquired metadata rejected",
						Resource: "vnode:" + name, Generation: vb.epoch, Err: err,
					})
				}
				return cleanup()
			})
		},
	}}
	return commands.SubmitPreparedAndWait(ctx, jobmgr.Request{UID: "jobmgr-vnode-acquire-" + uuid.NewString(), LaneKey: "vnode:" + name, Source: lifecycle.SourceJobManager, Route: "internal/vnodes/acquire"}, plan)
}
