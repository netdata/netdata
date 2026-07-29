// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type pendingPipelineToken struct {
	key     string
	version uint64
}

type pendingPipelineEntry struct {
	config  sdConfig
	release <-chan struct{}
	update  chan struct{}
	token   pendingPipelineToken
}

type pendingPipelineIndex struct {
	mu sync.Mutex
	wg sync.WaitGroup

	entries map[string]*pendingPipelineEntry
	retry   chan pendingPipelineToken
	ctx     context.Context
	version uint64
}

func newPendingPipelineIndex(ctx context.Context) *pendingPipelineIndex {
	return &pendingPipelineIndex{
		entries: make(map[string]*pendingPipelineEntry),
		retry:   make(chan pendingPipelineToken),
		ctx:     ctx,
	}
}

func (index *pendingPipelineIndex) retain(
	config sdConfig,
	release <-chan struct{},
) {
	if index == nil || config == nil || config.ExposedKey() == "" || release == nil {
		return
	}
	cloned, err := cloneSDConfig(config)
	if err != nil {
		return
	}
	key := cloned.ExposedKey()
	index.mu.Lock()
	index.version++
	if index.version == 0 {
		index.mu.Unlock()
		return
	}
	token := pendingPipelineToken{key: key, version: index.version}
	if entry := index.entries[key]; entry != nil {
		entry.config = cloned
		entry.release = release
		entry.token = token
		notifyPendingPipeline(entry.update)
		index.mu.Unlock()
		return
	}
	entry := &pendingPipelineEntry{
		config:  cloned,
		release: release,
		update:  make(chan struct{}, 1),
		token:   token,
	}
	index.entries[key] = entry
	index.wg.Add(1)
	index.mu.Unlock()
	go index.runEntry(key, entry)
}

func (index *pendingPipelineIndex) runEntry(key string, entry *pendingPipelineEntry) {
	defer index.wg.Done()
	for {
		index.mu.Lock()
		if index.entries[key] != entry {
			index.mu.Unlock()
			return
		}
		release := entry.release
		update := entry.update
		token := entry.token
		index.mu.Unlock()

		select {
		case <-release:
		case <-update:
			continue
		case <-index.ctx.Done():
			return
		}
		select {
		case index.retry <- token:
		case <-update:
			continue
		case <-index.ctx.Done():
			return
		}
		select {
		case <-update:
		case <-index.ctx.Done():
			return
		}
	}
}

func (index *pendingPipelineIndex) take(
	token pendingPipelineToken,
) (sdConfig, bool) {
	if index == nil || token.key == "" || token.version == 0 {
		return nil, false
	}
	index.mu.Lock()
	entry := index.entries[token.key]
	if entry == nil || entry.token != token {
		index.mu.Unlock()
		return nil, false
	}
	delete(index.entries, token.key)
	notifyPendingPipeline(entry.update)
	config := entry.config
	entry.config = nil
	index.mu.Unlock()
	return config, true
}

func (index *pendingPipelineIndex) cancel(key string) {
	if index == nil || key == "" {
		return
	}
	index.mu.Lock()
	entry := index.entries[key]
	if entry != nil {
		delete(index.entries, key)
		notifyPendingPipeline(entry.update)
	}
	index.mu.Unlock()
}

func (index *pendingPipelineIndex) wait() {
	if index != nil {
		index.wg.Wait()
	}
}

func notifyPendingPipeline(update chan struct{}) {
	select {
	case update <- struct{}{}:
	default:
	}
}

func cloneSDConfig(config sdConfig) (sdConfig, error) {
	if config == nil {
		return nil, errors.New("service discovery: nil config clone")
	}
	return newSDConfigFromJSON(
		config.DataJSON(),
		config.Name(),
		config.Source(),
		config.SourceType(),
		config.DiscovererType(),
		config.PipelineKey(),
	)
}

func (d *ServiceDiscovery) retainPendingPipeline(config sdConfig, err error) {
	if d == nil || d.pending == nil || config == nil ||
		config.SourceType() == confgroup.TypeDyncfg {
		return
	}
	var contained *materializationError
	if !errors.As(err, &contained) ||
		!errors.Is(err, jobmgr.ErrProcessAttemptBusy) &&
			!errors.Is(err, jobmgr.ErrProcessAttemptDeadline) {
		return
	}
	release, ok := d.attempts.ProcessAttemptReleased(contained.identity)
	if !ok {
		immediate := make(chan struct{})
		close(immediate)
		release = immediate
	}
	d.pending.retain(config, release)
}

func (d *ServiceDiscovery) retryPendingPipeline(token pendingPipelineToken) {
	if d == nil || d.pending == nil {
		return
	}
	config, ok := d.pending.take(token)
	if !ok {
		return
	}
	entry, exists := d.exposed.LookupByKey(config.ExposedKey())
	if !exists ||
		entry.Cfg.UID() != config.UID() ||
		entry.Cfg.Hash() != config.Hash() ||
		entry.Status != dyncfg.StatusFailed {
		return
	}
	d.autoEnableConfig(config)
}

func (d *ServiceDiscovery) cancelPendingPipeline(config sdConfig) {
	if d != nil && d.pending != nil && config != nil {
		d.pending.cancel(config.ExposedKey())
	}
}
