// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const restartGracePeriod = time.Minute

type pipelineToken struct {
	epoch, generation uint64
	key, exposed      string
}
type pipelineEventKind uint8

const (
	pipelinePrepared pipelineEventKind = iota + 1
	pipelineReleased
	pipelineStarted
	pipelineGroups
	pipelineCompleted
	pipelineFailed
	pipelineContained
)

type pipelineEvent struct {
	token    pipelineToken
	kind     pipelineEventKind
	prepared sdPipeline
	owner    *pipelineRuntime
	groups   []*confgroup.Group
	err      error
	release  <-chan struct{}
}
type pipelineSlot struct {
	token                         pipelineToken
	config                        sdConfig
	cancel                        context.CancelFunc
	ctx                           context.Context
	prepared                      sdPipeline
	current                       *pipelineRuntime
	published, preparing, waiting bool
	sources                       map[string]struct{}
	pending                       map[string]struct{}
	expires                       time.Time
}
type pipelineRuntime struct {
	token  pipelineToken
	live   bool
	cancel context.CancelFunc
	done   chan struct{}
}
type pipelineOutput struct {
	group *confgroup.Group
	owner *pipelineRuntime
}

// PipelineManager's state is advanced only by the SD event loop. The mutex also
// permits read-only inspection while tests and control-plane readers run.
// Opaque work and physical joins belong to process attempts, never this lock.
type PipelineManager struct {
	*logger.Logger
	mux        sync.Mutex
	ctx        context.Context
	epoch      uint64
	plugin     string
	attempts   jobmgr.ProcessAttemptAuthority
	prepare    func(context.Context, sdConfig) (sdPipeline, error)
	events     chan pipelineEvent
	pipelines  map[string]*pipelineSlot
	logical    map[string]*pipelineSlot // desired logical owners
	incumbents map[string]*pipelineSlot // live runtimes/snapshots, including retained old names
	sequence   uint64
	output     map[string][]pipelineOutput
}

func NewPipelineManager(log *logger.Logger) *PipelineManager {
	return &PipelineManager{
		Logger:     log,
		events:     make(chan pipelineEvent),
		pipelines:  make(map[string]*pipelineSlot),
		logical:    make(map[string]*pipelineSlot),
		incumbents: make(map[string]*pipelineSlot),
		output:     make(map[string][]pipelineOutput),
	}
}
func (m *PipelineManager) bind(d *ServiceDiscovery) {
	m.ctx, m.epoch, m.plugin, m.attempts, m.prepare = d.ctx, d.epoch, d.pluginName, d.attempts, d.preparePipeline
}

func (m *PipelineManager) enable(cfg sdConfig, prepared sdPipeline, old sdConfig) (func(), error) {
	if cfg == nil || cfg.PipelineKey() == "" || m.ctx == nil || m.attempts == nil {
		return nil, errors.New("service discovery: invalid activation")
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.ctx.Err() != nil {
		return nil, context.Cause(m.ctx)
	}
	m.sequence++
	if m.sequence == 0 {
		return nil, errors.New("service discovery: activation generation exhausted")
	}
	slot := m.pipelines[cfg.PipelineKey()]
	if slot == nil {
		slot = &pipelineSlot{
			sources: make(map[string]struct{}),
			pending: make(map[string]struct{}),
		}
		m.pipelines[cfg.PipelineKey()] = slot
	}
	if previous := m.logical[cfg.ExposedKey()]; previous != nil && previous != slot {
		// File-to-DynCfg conversion has distinct origin keys but one logical owner.
		if previous.cancel != nil {
			previous.cancel()
		}
		m.retireLocked(previous, false)
		delete(m.pipelines, previous.token.key)
	}
	if incumbent := m.incumbents[cfg.ExposedKey()]; incumbent != nil && incumbent != slot {
		// A failed/pending rename may still own this name physically while its
		// slot desires another name. Retire only that incumbent, not its successor.
		m.retireLocked(incumbent, false)
	}
	if slot.cancel != nil {
		slot.cancel()
	}
	if slot.token.exposed != "" {
		delete(m.logical, slot.token.exposed)
	}
	slot.token = pipelineToken{
		epoch:      m.epoch,
		generation: m.sequence,
		key:        cfg.PipelineKey(),
		exposed:    cfg.ExposedKey(),
	}
	slot.config = cfg
	// Both supersession and generation shutdown normally retire desired work.
	// Forward parent cancellation explicitly so it cannot win with a generic cause.
	desiredCtx, cancelDesired := context.WithCancelCause(context.WithoutCancel(m.ctx))
	stopParent := context.AfterFunc(m.ctx, func() { cancelDesired(jobmgr.ErrProcessAttemptRetired) })
	slot.ctx = desiredCtx
	slot.cancel = func() {
		stopParent()
		cancelDesired(jobmgr.ErrProcessAttemptRetired)
	}
	slot.prepared = prepared
	slot.preparing, slot.waiting, slot.published = false, false, false
	m.logical[cfg.ExposedKey()] = slot
	// A file rename keeps its incumbent until preparation succeeds. An UPDATE
	// already owns a successful preflight, so adoption cuts the incumbent now.
	if prepared != nil {
		m.retireLocked(slot, old != nil && old.SourceType() == confgroup.TypeDyncfg)
	}
	token := slot.token
	return func() { m.publish(token) }, nil
}
func (m *PipelineManager) publish(token pipelineToken) {
	m.mux.Lock()
	defer m.mux.Unlock()
	slot := m.pipelines[token.key]
	if slot == nil || slot.token != token || slot.ctx.Err() != nil {
		return
	}
	slot.published = true
	m.advanceLocked(slot)
}
func (m *PipelineManager) advanceLocked(slot *pipelineSlot) {
	if !slot.published || slot.preparing || slot.waiting || slot.ctx.Err() != nil {
		return
	}
	if slot.prepared == nil {
		slot.preparing = true
		token, ctx, cfg := slot.token, slot.ctx, slot.config
		go func() {
			p, err := m.prepare(ctx, cfg)
			m.post(ctx, pipelineEvent{
				token:    token,
				kind:     pipelinePrepared,
				prepared: p,
				err:      err,
			})
		}()
		return
	}
	if slot.current != nil {
		if slot.current.live {
			m.retireLocked(slot, false)
		}
		select {
		case <-slot.current.done:
			slot.current = nil
		default:
			m.waitLocked(slot, slot.current.done)
			return
		}
	}
	p := slot.prepared
	slot.prepared = nil
	ownerCtx, cancel := context.WithCancel(m.ctx)
	owner := &pipelineRuntime{
		token:  slot.token,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	owner.live = true
	identity := m.runtimeIdentity("logical", slot.config.ExposedKey())
	cfg, token := slot.config, slot.token
	// Workers report terminal settlement only after their process reservation is
	// released. A blocked actor must never retain a partially acquired alias pair.
	settlement := make(chan pipelineEvent, 1)
	contain := func(cause error) {
		cancel()
		m.postAsync(m.ctx, pipelineEvent{
			token: token,
			kind:  pipelineContained,
			owner: owner,
			err:   cause,
		})
	}
	attempt, err := m.attempts.StartProcessAttempt(slot.ctx, jobmgr.ProcessAttemptPlan{
		Identity:      identity,
		Target:        m.epoch,
		OnContainment: contain,
		Work: func(ctx context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			physicalDone := make(chan struct{})
			defer close(physicalDone)
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			fail := func(err error, release <-chan struct{}) {
				settlement <- pipelineEvent{
					token:    token,
					kind:     pipelineFailed,
					owner:    owner,
					prepared: p,
					err:      err,
					release:  release,
				}
			}
			if cfg.SourceType() != confgroup.TypeDyncfg {
				alias := m.runtimeIdentity("file", cfg.PipelineKey())
				admitted := make(chan error, 1)
				guard, guardErr := m.attempts.StartProcessAttempt(
					ctx,
					jobmgr.ProcessAttemptPlan{
						Identity:      alias,
						Target:        m.epoch,
						OnContainment: contain,
						Work: func(_ context.Context, a jobmgr.ProcessAttemptAdmission) error {
							err := a.Admit()
							admitted <- err
							if err != nil {
								return nil
							}
							<-physicalDone
							return nil
						},
					},
				)
				if guardErr == nil {
					guardErr = <-admitted
				}
				if guardErr != nil {
					var released <-chan struct{}
					if guard != nil {
						guard.Cut(context.Canceled)
						released = guard.Released()
					} else {
						released, _ = m.attempts.ProcessAttemptReleased(alias)
					}
					fail(guardErr, released)
					return nil
				}
			}
			if err := admission.Admit(); err != nil {
				fail(err, nil)
				return nil
			}
			if ownerCtx.Err() != nil {
				fail(context.Cause(ownerCtx), nil)
				return nil
			}
			m.post(ownerCtx, pipelineEvent{
				token: token,
				kind:  pipelineStarted,
				owner: owner,
			})
			err := m.runPipeline(ownerCtx, owner, p)
			kind := pipelineCompleted
			if err != nil {
				kind = pipelineFailed
			}
			settlement <- pipelineEvent{
				token: token,
				kind:  kind,
				owner: owner,
				err:   err,
			}
			return err
		},
	})
	if err != nil {
		owner.live = false
		cancel()
		close(owner.done)
		slot.prepared = p
		if errors.Is(err, jobmgr.ErrProcessAttemptBusy) {
			released, _ := m.attempts.ProcessAttemptReleased(identity)
			m.waitLocked(slot, released)
			return
		}
		m.postAsync(slot.ctx, pipelineEvent{
			token: token,
			kind:  pipelineFailed,
			err:   err,
		})
		return
	}
	slot.current = owner
	m.incumbents[owner.token.exposed] = slot
	go func() {
		<-attempt.Released()
		close(owner.done)
		select {
		case event := <-settlement:
			m.post(m.ctx, event)
		default:
			// The process authority contains panics before Work can record settlement.
			m.post(
				m.ctx,
				pipelineEvent{
					token: token,
					kind:  pipelineFailed,
					owner: owner,
					err:   jobmgr.ErrProcessAttemptWorkerPanic,
				},
			)
		}
	}()
}

func (m *PipelineManager) runtimeIdentity(domain, key string) jobmgr.ProcessAttemptIdentity {
	return jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       jobmgr.ProcessAttemptIdentityKey("sd-runtime-"+domain, m.plugin, key),
		Resource:  "service discovery pipeline",
	}
}
func (m *PipelineManager) waitLocked(slot *pipelineSlot, release <-chan struct{}) {
	slot.waiting = true
	if release == nil {
		closed := make(chan struct{})
		close(closed)
		release = closed
	}
	token, ctx := slot.token, slot.ctx
	go func() {
		select {
		case <-release:
			m.post(ctx, pipelineEvent{
				token: token,
				kind:  pipelineReleased,
			})
		case <-ctx.Done():
		}
	}()
}
func (m *PipelineManager) post(ctx context.Context, event pipelineEvent) {
	select {
	case m.events <- event:
	case <-ctx.Done():
	}
}
func (m *PipelineManager) postAsync(ctx context.Context, event pipelineEvent) { go m.post(ctx, event) }

func (m *PipelineManager) runPipeline(ctx context.Context, owner *pipelineRuntime, pl sdPipeline) error {
	groups := make(chan []*confgroup.Group)
	done := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			if recover() != nil {
				err = jobmgr.ErrProcessAttemptWorkerPanic
			}
			done <- err
		}()
		pl.Run(ctx, groups)
	}()
	for {
		select {
		case grps := <-groups:
			m.post(ctx, pipelineEvent{
				token:  owner.token,
				kind:   pipelineGroups,
				owner:  owner,
				groups: grps,
			})
		case err := <-done:
			return err
		case <-ctx.Done():
			// Physical cancellation retains the process identity until Run and all
			// children return. The actor owns the logical publication cut.
			return <-done
		}
	}
}

func (m *PipelineManager) handle(event pipelineEvent) (sdConfig, dyncfg.Status) {
	m.mux.Lock()
	defer m.mux.Unlock()
	slot := m.pipelines[event.token.key]
	if slot == nil {
		return nil, ""
	}
	if event.kind == pipelineContained {
		if slot.current != event.owner || !event.owner.live {
			return nil, ""
		}
		m.retireLocked(slot, false)
		if slot.token == event.token {
			return slot.config, dyncfg.StatusFailed
		}
		return nil, ""
	}
	if event.kind == pipelineGroups {
		if slot.current != event.owner || !event.owner.live {
			return nil, ""
		}
		for _, group := range event.groups {
			if group == nil {
				continue
			}
			delete(slot.pending, group.Source)
			if len(group.Configs) == 0 {
				delete(slot.sources, group.Source)
				// The accepted removal must survive retirement of its former owner.
				m.queueLocked(group, nil)
			} else {
				slot.sources[group.Source] = struct{}{}
				m.queueLocked(group, event.owner)
			}
		}
		return nil, ""
	}
	if event.kind == pipelineFailed && event.owner != nil && slot.current == event.owner && event.owner.live &&
		slot.token != event.token {
		// A retained incumbent can fail while a different desired revision is
		// preparing. Its sources must retire without changing the successor's health.
		m.retireLocked(slot, false)
		return nil, ""
	}
	if slot.token != event.token || slot.ctx.Err() != nil {
		return nil, ""
	}
	switch event.kind {
	case pipelinePrepared:
		slot.preparing = false
		if event.err != nil {
			var contained *materializationError
			if errors.As(event.err, &contained) &&
				(errors.Is(event.err, jobmgr.ErrProcessAttemptBusy) || errors.Is(event.err, jobmgr.ErrProcessAttemptDeadline)) {
				release, _ := m.attempts.ProcessAttemptReleased(contained.identity)
				m.waitLocked(slot, release)
				return nil, ""
			}
			return slot.config, dyncfg.StatusFailed
		}
		slot.prepared = event.prepared
		m.advanceLocked(slot)
	case pipelineReleased:
		slot.waiting = false
		m.advanceLocked(slot)
	case pipelineStarted:
		if slot.current == event.owner && event.owner.live {
			return slot.config, dyncfg.StatusRunning
		}
	case pipelineFailed:
		if event.owner != nil && slot.current != event.owner {
			return nil, ""
		}
		if event.owner != nil {
			m.retireLocked(slot, false)
		}
		if errors.Is(event.err, jobmgr.ErrProcessAttemptBusy) {
			slot.prepared = event.prepared
			m.waitLocked(slot, event.release)
			return nil, ""
		}
		return slot.config, dyncfg.StatusFailed
	case pipelineCompleted:
		// Finite discovery is a successful active snapshot, not a failed runtime.
	}
	return nil, ""
}
func (m *PipelineManager) retireLocked(slot *pipelineSlot, grace bool) {
	if slot.current != nil {
		if m.incumbents[slot.current.token.exposed] == slot {
			delete(m.incumbents, slot.current.token.exposed)
		}
		slot.current.live = false
		slot.current.cancel()
	}
	if grace {
		for source := range slot.sources {
			slot.pending[source] = struct{}{}
		}
		slot.sources = make(map[string]struct{})
		slot.expires = time.Now().Add(restartGracePeriod)
		return
	}
	for source := range slot.sources {
		m.queueLocked(&confgroup.Group{
			Source: source,
		}, nil)
	}
	for source := range slot.pending {
		m.queueLocked(&confgroup.Group{
			Source: source,
		}, nil)
	}
	slot.sources = make(map[string]struct{})
	slot.pending = make(map[string]struct{})
}
func (m *PipelineManager) Stop(key string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	slot := m.pipelines[key]
	if slot == nil {
		return
	}
	if slot.cancel != nil {
		slot.cancel()
	}
	slot.prepared = nil
	m.retireLocked(slot, false)
	delete(m.logical, slot.token.exposed)
	delete(m.pipelines, key)
}
func (m *PipelineManager) StopAll() {
	m.mux.Lock()
	keys := make([]string, 0, len(m.pipelines))
	for key := range m.pipelines {
		keys = append(keys, key)
	}
	m.mux.Unlock()
	for _, key := range keys {
		m.Stop(key)
	}
}
func (m *PipelineManager) IsRunning(key string) bool {
	m.mux.Lock()
	defer m.mux.Unlock()
	s := m.pipelines[key]
	return s != nil && s.current != nil && s.current.live
}
func (m *PipelineManager) processGracePeriodRemovals(context.Context) {
	m.mux.Lock()
	defer m.mux.Unlock()
	now := time.Now()
	for _, slot := range m.pipelines {
		if slot.expires.IsZero() || now.Before(slot.expires) {
			continue
		}
		for source := range slot.pending {
			m.queueLocked(&confgroup.Group{
				Source: source,
			}, nil)
		}
		slot.pending = make(map[string]struct{})
		slot.expires = time.Time{}
	}
}
func (m *PipelineManager) queueLocked(group *confgroup.Group, owner *pipelineRuntime) {
	queue := m.output[group.Source]
	next := pipelineOutput{
		group: group,
		owner: owner,
	}
	// Preserve removal-before-successor ordering; coalesce snapshots and repeated
	// removals for the same actual source without introducing a backlog policy.
	if owner == nil {
		queue = []pipelineOutput{next}
	} else if len(queue) > 0 && queue[len(queue)-1].owner != nil {
		queue[len(queue)-1] = next
	} else {
		queue = append(queue, next)
	}
	m.output[group.Source] = queue
}
func (m *PipelineManager) nextOutput() ([]*confgroup.Group, func()) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for source, queue := range m.output {
		for len(queue) > 0 && queue[0].owner != nil && !queue[0].owner.live {
			queue = queue[1:]
		}
		if len(queue) == 0 {
			delete(m.output, source)
			continue
		}
		m.output[source] = queue
		item := queue[0]
		return []*confgroup.Group{item.group}, func() {
			m.mux.Lock()
			defer m.mux.Unlock()
			q := m.output[source]
			if len(q) > 0 && q[0] == item {
				if len(q) == 1 {
					delete(m.output, source)
				} else {
					m.output[source] = q[1:]
				}
			}
		}
	}
	return nil, nil
}
