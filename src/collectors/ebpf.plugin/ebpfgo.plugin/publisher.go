// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// PublisherService is a singleton coordinator managing the shared metrics store
// for all eBPF collectors. This replaces per-collector store instances with
// a unified publisher service.
type PublisherService struct {
	store metrix.CollectorStore
	mu    sync.RWMutex
}

var (
	publisherInstance *PublisherService
	publisherOnce     sync.Once
)

// GetPublisher returns the singleton PublisherService instance.
func GetPublisher() *PublisherService {
	publisherOnce.Do(func() {
		publisherInstance = &PublisherService{
			store: metrix.NewCollectorStore(),
		}
	})
	return publisherInstance
}

// MetricStore returns the shared metrics store for writing collector metrics.
func (p *PublisherService) MetricStore() metrix.CollectorStore {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.store
}

// Close releases the publisher service resources.
// Called during plugin shutdown.
func (p *PublisherService) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.store != nil {
		// Store may have cleanup requirements in future iterations
	}
}
