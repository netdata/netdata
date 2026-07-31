// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type pendingJobPlanner func(confgroup.Config, pendingJobToken) (jobmgr.WorkPlan, error)

type pendingJobToken struct {
	uid           string
	baselineUID   string
	version       uint64
	requireAbsent bool // retry only while the graph still has no current job
}

type pendingJobIndex struct {
	mu sync.Mutex
	wg sync.WaitGroup

	entries  map[string]*pendingJob
	commands jobmgr.PreparedCommandPort
	plan     pendingJobPlanner
	failure  func(error)
	run      uint64
	version  uint64
	bound    bool
	closed   bool
	failed   bool
	stop     chan struct{}
	done     chan struct{}
	failOnce sync.Once
}

type pendingJob struct {
	config  confgroup.Config
	release <-chan struct{}
	update  chan struct{}
	token   pendingJobToken
}

func newPendingJobIndex() *pendingJobIndex {
	return &pendingJobIndex{
		entries: make(map[string]*pendingJob),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (pji *pendingJobIndex) bind(
	commands jobmgr.PreparedCommandPort,
	plan pendingJobPlanner,
	run uint64,
	failure func(error),
) error {
	if pji == nil || commands == nil || plan == nil || run == 0 || failure == nil {
		return errors.New("job output: invalid pending-job binding")
	}
	pji.mu.Lock()
	if pji.bound || pji.closed {
		pji.mu.Unlock()
		return errors.New("job output: pending jobs already bound")
	}
	pji.commands = commands
	pji.plan = plan
	pji.failure = failure
	pji.run = run
	pji.bound = true
	pji.mu.Unlock()
	go pji.join()
	return nil
}

func (pji *pendingJobIndex) retain(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
) {
	pji.retainWithRequirement(config, release, baselineUID, false)
}

func (pji *pendingJobIndex) retainAbsent(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
) {
	pji.retainWithRequirement(config, release, baselineUID, true)
}

func (pji *pendingJobIndex) retainWithRequirement(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
	requireAbsent bool,
) {
	if pji == nil || config == nil || config.FullName() == "" || config.UID() == "" {
		return
	}
	cloned, err := config.Clone()
	if err != nil {
		return
	}
	id := cloned.FullName()
	pji.mu.Lock()
	if !pji.bound || pji.closed || pji.failed {
		pji.mu.Unlock()
		return
	}
	pji.version++
	if pji.version == 0 {
		pji.mu.Unlock()
		pji.fail(errors.New("job output: pending-job version wrapped"))
		return
	}
	token := pendingJobToken{
		uid:           cloned.UID(),
		baselineUID:   baselineUID,
		version:       pji.version,
		requireAbsent: requireAbsent,
	}
	if current := pji.entries[id]; current != nil {
		current.config = cloned
		current.release = release
		current.token = token
		notifyPendingJob(current.update)
		pji.mu.Unlock()
		return
	}
	entry := &pendingJob{
		config:  cloned,
		release: release,
		update:  make(chan struct{}, 1),
		token:   token,
	}
	pji.entries[id] = entry
	pji.wg.Add(1)
	pji.mu.Unlock()
	go pji.runEntry(id, entry)
}

func (pji *pendingJobIndex) runEntry(id string, entry *pendingJob) {
	defer pji.wg.Done()
	for {
		pji.mu.Lock()
		if pji.closed || pji.failed || pji.entries[id] != entry {
			pji.mu.Unlock()
			return
		}
		config := entry.config
		release := entry.release
		token := entry.token
		update := entry.update
		pji.mu.Unlock()

		if release != nil {
			select {
			case <-release:
			case <-update:
				continue
			case <-pji.stop:
				return
			}
		}
		if !pji.isCurrent(id, token) {
			continue
		}
		if err := pji.dispatch(config, token); err != nil {
			if lifecycle.ContainsOnlyCurrentStoppingRejections(err, pji.run) {
				return
			}
			pji.fail(err)
			return
		}
		select {
		case <-update:
		case <-pji.stop:
			return
		}
	}
}

func (pji *pendingJobIndex) dispatch(config confgroup.Config, token pendingJobToken) error {
	work, err := pji.plan(config, token)
	if err != nil {
		return err
	}
	return pji.commands.SubmitPrepared(context.Background(), jobmgr.Request{
		UID: fmt.Sprintf(
			"jobmgr-pending-job-%d-%d",
			pji.run,
			token.version,
		),
		LaneKey: config.FullName(),
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/jobs/pending",
	}, work)
}

func (pji *pendingJobIndex) isCurrent(id string, token pendingJobToken) bool {
	if pji == nil || id == "" || token.version == 0 {
		return false
	}
	pji.mu.Lock()
	defer pji.mu.Unlock()
	entry := pji.entries[id]
	return entry != nil && entry.token == token
}

func (pji *pendingJobIndex) settle(id string, token pendingJobToken) {
	if pji == nil || id == "" || token.version == 0 {
		return
	}
	pji.mu.Lock()
	entry := pji.entries[id]
	if entry != nil && entry.token == token {
		delete(pji.entries, id)
		notifyPendingJob(entry.update)
	}
	pji.mu.Unlock()
}

func (pji *pendingJobIndex) cancel(id string) {
	if pji == nil || id == "" {
		return
	}
	pji.mu.Lock()
	entry := pji.entries[id]
	if entry != nil {
		delete(pji.entries, id)
		notifyPendingJob(entry.update)
	}
	pji.mu.Unlock()
}

func (pji *pendingJobIndex) stopWorker() {
	if pji == nil {
		return
	}
	pji.mu.Lock()
	if pji.closed {
		pji.mu.Unlock()
		return
	}
	pji.closed = true
	for id, entry := range pji.entries {
		delete(pji.entries, id)
		notifyPendingJob(entry.update)
	}
	close(pji.stop)
	pji.mu.Unlock()
}

func (pji *pendingJobIndex) wait(ctx context.Context) error {
	if pji == nil || ctx == nil {
		return errors.New("job output: invalid pending-job wait")
	}
	pji.mu.Lock()
	bound := pji.bound
	done := pji.done
	pji.mu.Unlock()
	if !bound {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (pji *pendingJobIndex) join() {
	<-pji.stop
	pji.wg.Wait()
	close(pji.done)
}

func (pji *pendingJobIndex) fail(err error) {
	if err == nil {
		return
	}
	pji.failOnce.Do(func() {
		pji.mu.Lock()
		pji.failed = true
		failure := pji.failure
		alreadyClosed := pji.closed
		if !alreadyClosed {
			pji.closed = true
			for id, entry := range pji.entries {
				delete(pji.entries, id)
				notifyPendingJob(entry.update)
			}
			close(pji.stop)
		}
		pji.mu.Unlock()
		if failure != nil {
			failure(err)
		}
	})
}

func notifyPendingJob(update chan struct{}) {
	select {
	case update <- struct{}{}:
	default:
	}
}

func (dcjc *DynCfgJobController) retainPendingAfterApply(
	config confgroup.Config,
	namespace jobmgr.ProcessAttemptNamespace,
	baselineUID string,
) func() {
	return dcjc.retainPendingAfterApplyWithRequirement(
		config,
		namespace,
		baselineUID,
		false,
	)
}

func (dcjc *DynCfgJobController) retainAbsentPendingAfterApply(
	config confgroup.Config,
	namespace jobmgr.ProcessAttemptNamespace,
	baselineUID string,
) func() {
	return dcjc.retainPendingAfterApplyWithRequirement(
		config,
		namespace,
		baselineUID,
		true,
	)
}

func (dcjc *DynCfgJobController) retainPendingAfterApplyWithRequirement(
	config confgroup.Config,
	namespace jobmgr.ProcessAttemptNamespace,
	baselineUID string,
	requireAbsent bool,
) func() {
	return func() {
		if dcjc == nil || dcjc.scheduler == nil || dcjc.scheduler.pending == nil ||
			dcjc.factory == nil || dcjc.factory.config.Attempts == nil ||
			config == nil || config.FullName() == "" {
			return
		}
		identity := jobAttemptIdentity(namespace, config.FullName())
		release, ok := dcjc.factory.config.Attempts.ProcessAttemptReleased(identity)
		if !ok {
			immediate := make(chan struct{})
			close(immediate)
			release = immediate
		}
		if requireAbsent {
			dcjc.scheduler.pending.retainAbsent(config, release, baselineUID)
		} else {
			dcjc.scheduler.pending.retain(config, release, baselineUID)
		}
	}
}

func (dcjc *DynCfgJobController) pendingSettlement(
	id string,
	token pendingJobToken,
) func() {
	if dcjc == nil || dcjc.scheduler == nil || dcjc.scheduler.pending == nil ||
		id == "" || token.version == 0 {
		return nil
	}
	return func() {
		dcjc.scheduler.pending.settle(id, token)
	}
}

func (dcjc *DynCfgJobController) pendingDesiredSettlement(
	config confgroup.Config,
	token pendingJobToken,
) func() {
	if dcjc == nil || dcjc.scheduler == nil || dcjc.scheduler.pending == nil ||
		config == nil || config.FullName() == "" || config.UID() == "" {
		return nil
	}
	if token.version != 0 {
		return dcjc.pendingSettlement(config.FullName(), token)
	}
	return func() {
		dcjc.scheduler.pending.cancel(config.FullName())
	}
}
