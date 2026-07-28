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

func (index *pendingJobIndex) bind(
	commands jobmgr.PreparedCommandPort,
	plan pendingJobPlanner,
	run uint64,
	failure func(error),
) error {
	if index == nil || commands == nil || plan == nil || run == 0 || failure == nil {
		return errors.New("job output: invalid pending-job binding")
	}
	index.mu.Lock()
	if index.bound || index.closed {
		index.mu.Unlock()
		return errors.New("job output: pending jobs already bound")
	}
	index.commands = commands
	index.plan = plan
	index.failure = failure
	index.run = run
	index.bound = true
	index.mu.Unlock()
	go index.join()
	return nil
}

func (index *pendingJobIndex) retain(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
) {
	index.retainWithRequirement(config, release, baselineUID, false)
}

func (index *pendingJobIndex) retainAbsent(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
) {
	index.retainWithRequirement(config, release, baselineUID, true)
}

func (index *pendingJobIndex) retainWithRequirement(
	config confgroup.Config,
	release <-chan struct{},
	baselineUID string,
	requireAbsent bool,
) {
	if index == nil || config == nil || config.FullName() == "" || config.UID() == "" {
		return
	}
	cloned, err := config.Clone()
	if err != nil {
		return
	}
	id := cloned.FullName()
	index.mu.Lock()
	if !index.bound || index.closed || index.failed {
		index.mu.Unlock()
		return
	}
	index.version++
	if index.version == 0 {
		index.mu.Unlock()
		index.fail(errors.New("job output: pending-job version wrapped"))
		return
	}
	token := pendingJobToken{
		uid:           cloned.UID(),
		baselineUID:   baselineUID,
		version:       index.version,
		requireAbsent: requireAbsent,
	}
	if current := index.entries[id]; current != nil {
		current.config = cloned
		current.release = release
		current.token = token
		notifyPendingJob(current.update)
		index.mu.Unlock()
		return
	}
	entry := &pendingJob{
		config:  cloned,
		release: release,
		update:  make(chan struct{}, 1),
		token:   token,
	}
	index.entries[id] = entry
	index.wg.Add(1)
	index.mu.Unlock()
	go index.runEntry(id, entry)
}

func (index *pendingJobIndex) runEntry(id string, entry *pendingJob) {
	defer index.wg.Done()
	for {
		index.mu.Lock()
		if index.closed || index.failed || index.entries[id] != entry {
			index.mu.Unlock()
			return
		}
		config := entry.config
		release := entry.release
		token := entry.token
		update := entry.update
		index.mu.Unlock()

		if release != nil {
			select {
			case <-release:
			case <-update:
				continue
			case <-index.stop:
				return
			}
		}
		if !index.isCurrent(id, token) {
			continue
		}
		if err := index.dispatch(config, token); err != nil {
			if lifecycle.ContainsOnlyCurrentStoppingRejections(err, index.run) {
				return
			}
			index.fail(err)
			return
		}
		select {
		case <-update:
		case <-index.stop:
			return
		}
	}
}

func (index *pendingJobIndex) dispatch(config confgroup.Config, token pendingJobToken) error {
	work, err := index.plan(config, token)
	if err != nil {
		return err
	}
	return index.commands.SubmitPrepared(context.Background(), jobmgr.Request{
		UID: fmt.Sprintf(
			"jobmgr-pending-job-%d-%d",
			index.run,
			token.version,
		),
		LaneKey: config.FullName(),
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/jobs/pending",
	}, work)
}

func (index *pendingJobIndex) isCurrent(id string, token pendingJobToken) bool {
	if index == nil || id == "" || token.version == 0 {
		return false
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	entry := index.entries[id]
	return entry != nil && entry.token == token
}

func (index *pendingJobIndex) settle(id string, token pendingJobToken) {
	if index == nil || id == "" || token.version == 0 {
		return
	}
	index.mu.Lock()
	entry := index.entries[id]
	if entry != nil && entry.token == token {
		delete(index.entries, id)
		notifyPendingJob(entry.update)
	}
	index.mu.Unlock()
}

func (index *pendingJobIndex) cancel(id string) {
	if index == nil || id == "" {
		return
	}
	index.mu.Lock()
	entry := index.entries[id]
	if entry != nil {
		delete(index.entries, id)
		notifyPendingJob(entry.update)
	}
	index.mu.Unlock()
}

func (index *pendingJobIndex) stopWorker() {
	if index == nil {
		return
	}
	index.mu.Lock()
	if index.closed {
		index.mu.Unlock()
		return
	}
	index.closed = true
	for id, entry := range index.entries {
		delete(index.entries, id)
		notifyPendingJob(entry.update)
	}
	close(index.stop)
	index.mu.Unlock()
}

func (index *pendingJobIndex) wait(ctx context.Context) error {
	if index == nil || ctx == nil {
		return errors.New("job output: invalid pending-job wait")
	}
	index.mu.Lock()
	bound := index.bound
	done := index.done
	index.mu.Unlock()
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

func (index *pendingJobIndex) joined() bool {
	if index == nil {
		return true
	}
	index.mu.Lock()
	bound := index.bound
	done := index.done
	index.mu.Unlock()
	if !bound {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (index *pendingJobIndex) join() {
	<-index.stop
	index.wg.Wait()
	close(index.done)
}

func (index *pendingJobIndex) fail(err error) {
	if err == nil {
		return
	}
	index.failOnce.Do(func() {
		index.mu.Lock()
		index.failed = true
		failure := index.failure
		alreadyClosed := index.closed
		if !alreadyClosed {
			index.closed = true
			for id, entry := range index.entries {
				delete(index.entries, id)
				notifyPendingJob(entry.update)
			}
			close(index.stop)
		}
		index.mu.Unlock()
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
		identity := jobmgr.ProcessAttemptIdentity{
			Namespace: namespace,
			Key:       config.FullName(),
			Resource:  candidateDiagnosticResource(config.FullName()),
		}
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
