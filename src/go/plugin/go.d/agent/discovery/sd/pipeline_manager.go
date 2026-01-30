// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
)

const (
	restartGracePeriod = 30 * time.Second
)

// PipelineManager manages the lifecycle of discovery pipelines.
// It handles starting, stopping, and restarting pipelines, tracks sources
// for cleanup, and implements a grace period mechanism for restarts.
type PipelineManager struct {
	*logger.Logger

	newPipeline func(cfg pipeline.Config) (sdPipeline, error)
	send        func(ctx context.Context, groups []*confgroup.Group)

	mux             sync.Mutex
	pipelines       map[string]*runningPipeline       // [pipelineKey]
	pipelineSources map[string]map[string]struct{}    // [pipelineKey][source]
	pendingRemovals map[string]*pendingRemoval        // [pipelineKey]
}

type runningPipeline struct {
	cfg    pipeline.Config
	cancel context.CancelFunc
	done   chan struct{}
}

type pendingRemoval struct {
	sources   map[string]struct{}
	timestamp time.Time
}

// NewPipelineManager creates a new PipelineManager.
func NewPipelineManager(
	log *logger.Logger,
	newPipeline func(cfg pipeline.Config) (sdPipeline, error),
	send func(ctx context.Context, groups []*confgroup.Group),
) *PipelineManager {
	return &PipelineManager{
		Logger:          log,
		newPipeline:     newPipeline,
		send:            send,
		pipelines:       make(map[string]*runningPipeline),
		pipelineSources: make(map[string]map[string]struct{}),
		pendingRemovals: make(map[string]*pendingRemoval),
	}
}

// Start starts a new pipeline with the given key and config.
// If a pipeline with the same key is already running, it will be stopped first.
func (m *PipelineManager) Start(ctx context.Context, key string, cfg pipeline.Config) error {
	m.mux.Lock()

	// Stop existing pipeline if any (no grace period - this is initial start or replace)
	rp := m.removePipelineLocked(key, true)

	m.mux.Unlock()

	// Wait for old pipeline outside the lock
	if rp != nil {
		m.waitForPipeline(key, rp)
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	return m.startPipelineLocked(ctx, key, cfg)
}

// Stop stops a pipeline and sends removal groups for all its tracked sources.
func (m *PipelineManager) Stop(key string) {
	m.mux.Lock()
	rp := m.removePipelineLocked(key, true)
	m.mux.Unlock()

	// Wait for pipeline outside the lock
	if rp != nil {
		m.waitForPipeline(key, rp)
	}
}

// Restart stops a pipeline and starts it with new config, using grace period
// to avoid removing discovered jobs that will be re-discovered.
func (m *PipelineManager) Restart(ctx context.Context, key string, cfg pipeline.Config) error {
	// Validate new config first by creating the pipeline (outside lock)
	pl, err := m.newPipeline(cfg)
	if err != nil {
		// New config is invalid, keep old pipeline running
		return err
	}

	m.mux.Lock()

	// Mark current sources as pending removal (grace period)
	if sources, ok := m.pipelineSources[key]; ok && len(sources) > 0 {
		m.pendingRemovals[key] = &pendingRemoval{
			sources:   copySourcesMap(sources),
			timestamp: time.Now(),
		}
		m.Debugf("pipeline '%s': marked %d sources for pending removal (grace period)", key, len(sources))
	}

	// Stop old pipeline without cleanup (sources are pending, not removed)
	rp := m.removePipelineLocked(key, false)

	m.mux.Unlock()

	// Wait for old pipeline outside the lock
	if rp != nil {
		m.waitForPipeline(key, rp)
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	// Start the already-created new pipeline
	return m.startPipelineWithInstanceLocked(ctx, key, cfg, pl)
}

// StopAll stops all running pipelines with cleanup.
func (m *PipelineManager) StopAll() {
	// Collect and remove all pipelines while holding the lock
	m.mux.Lock()
	toStop := make(map[string]*runningPipeline, len(m.pipelines))
	for key := range m.pipelines {
		if rp := m.removePipelineLocked(key, true); rp != nil {
			toStop[key] = rp
		}
	}
	m.mux.Unlock()

	// Wait for all pipelines outside the lock
	for key, rp := range toStop {
		m.waitForPipeline(key, rp)
	}
}

// RunGracePeriodCleanup runs the grace period cleanup loop.
// It should be called as a goroutine and will run until ctx is cancelled.
func (m *PipelineManager) RunGracePeriodCleanup(ctx context.Context) {
	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			m.processGracePeriodRemovals(ctx)
		}
	}
}

// IsRunning returns true if a pipeline with the given key is running.
func (m *PipelineManager) IsRunning(key string) bool {
	m.mux.Lock()
	defer m.mux.Unlock()

	_, ok := m.pipelines[key]
	return ok
}

// Keys returns the keys of all running pipelines.
func (m *PipelineManager) Keys() []string {
	m.mux.Lock()
	defer m.mux.Unlock()

	keys := make([]string, 0, len(m.pipelines))
	for k := range m.pipelines {
		keys = append(keys, k)
	}
	return keys
}

func (m *PipelineManager) startPipelineLocked(ctx context.Context, key string, cfg pipeline.Config) error {
	pl, err := m.newPipeline(cfg)
	if err != nil {
		return err
	}

	return m.startPipelineWithInstanceLocked(ctx, key, cfg, pl)
}

func (m *PipelineManager) startPipelineWithInstanceLocked(ctx context.Context, key string, cfg pipeline.Config, pl sdPipeline) error {
	plCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	rp := &runningPipeline{
		cfg:    cfg,
		cancel: cancel,
		done:   done,
	}

	m.pipelines[key] = rp
	m.pipelineSources[key] = make(map[string]struct{})

	go func() {
		defer close(done)
		m.runPipeline(plCtx, key, pl)
	}()

	m.Infof("pipeline '%s' started", key)
	return nil
}

func (m *PipelineManager) runPipeline(ctx context.Context, key string, pl sdPipeline) {
	groups := make(chan []*confgroup.Group)
	done := make(chan struct{})

	go func() {
		defer close(done)
		pl.Run(ctx, groups)
	}()

	for {
		select {
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				m.Warningf("pipeline '%s': timeout waiting for shutdown", key)
			}
			return
		case <-done:
			return
		case grps := <-groups:
			m.onGroupsReceived(ctx, key, grps)
		}
	}
}

func (m *PipelineManager) onGroupsReceived(ctx context.Context, key string, groups []*confgroup.Group) {
	m.mux.Lock()

	// Track sources
	for _, grp := range groups {
		if m.pipelineSources[key] == nil {
			m.pipelineSources[key] = make(map[string]struct{})
		}
		m.pipelineSources[key][grp.Source] = struct{}{}

		// Cancel pending removal for re-discovered sources
		if pending, ok := m.pendingRemovals[key]; ok {
			if _, wasPending := pending.sources[grp.Source]; wasPending {
				delete(pending.sources, grp.Source)
				m.Debugf("pipeline '%s': source '%s' re-discovered, cancelled pending removal", key, grp.Source)
			}
		}
	}

	m.mux.Unlock()

	// Forward groups
	m.send(ctx, groups)
}

// removePipelineLocked removes a pipeline from the map, cancels it, and optionally
// schedules cleanup. Returns the runningPipeline so caller can wait outside the lock.
// Must be called with m.mux held.
func (m *PipelineManager) removePipelineLocked(key string, cleanup bool) *runningPipeline {
	rp, ok := m.pipelines[key]
	if !ok {
		return nil
	}

	// Cancel the pipeline (it will stop asynchronously)
	rp.cancel()

	// Remove from map so it's not visible to other operations
	delete(m.pipelines, key)

	if cleanup {
		m.cleanupSourcesLocked(key)
	}

	return rp
}

// waitForPipeline waits for a pipeline to finish. Must be called without holding m.mux.
func (m *PipelineManager) waitForPipeline(key string, rp *runningPipeline) {
	<-rp.done
	m.Infof("pipeline '%s' stopped", key)
}

func (m *PipelineManager) cleanupSourcesLocked(key string) {
	sources, ok := m.pipelineSources[key]
	if !ok || len(sources) == 0 {
		return
	}

	// Use a timeout context to avoid blocking forever if consumer stopped reading
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send empty groups to remove discovered jobs
	for source := range sources {
		m.Debugf("pipeline '%s': sending removal for source '%s'", key, source)
		m.send(ctx, []*confgroup.Group{{Source: source}})
	}

	delete(m.pipelineSources, key)
	delete(m.pendingRemovals, key)
}

func (m *PipelineManager) processGracePeriodRemovals(ctx context.Context) {
	// Collect expired removals while holding the lock
	type removal struct {
		key    string
		source string
	}
	var toRemove []removal

	m.mux.Lock()
	now := time.Now()

	for key, pending := range m.pendingRemovals {
		if now.Sub(pending.timestamp) < restartGracePeriod {
			continue
		}

		// Grace period expired - collect sources that weren't re-discovered
		for source := range pending.sources {
			toRemove = append(toRemove, removal{key: key, source: source})

			// Remove from tracked sources
			if sources, ok := m.pipelineSources[key]; ok {
				delete(sources, source)
			}
		}

		delete(m.pendingRemovals, key)
	}
	m.mux.Unlock()

	// Send removals outside the lock to avoid blocking other operations
	for _, r := range toRemove {
		m.Infof("pipeline '%s': grace period expired, removing source '%s'", r.key, r.source)
		m.send(ctx, []*confgroup.Group{{Source: r.source}})
	}
}

func copySourcesMap(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
