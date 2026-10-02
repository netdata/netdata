// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtimehub coordinates only admitted DEM runtime data. Desired
// configuration, startup, retries and job status belong to the Agent framework.
package runtimehub

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
)

type Site struct {
	Route      *ingest.Route
	Aggregator *agg.Aggregator
	Generation string
	Redactor   *secrets.Redactor
}
type Availability struct {
	Serving   bool
	Listen    string
	PublicURL string
	TLS       bool
}
type registration struct {
	data   *Site
	ctx    context.Context
	cancel context.CancelFunc
	users  sync.WaitGroup
}
type receiver struct{ availability Availability }

type Hub struct {
	mu           sync.Mutex
	sites        map[string]*registration
	receiver     *receiver
	lastReceiver Availability
}

func New() *Hub {
	return &Hub{
		sites: map[string]*registration{},
	}
}

// Register begins admission for one exact runtime. Retire revokes admission,
// interrupts its HTTP producers and waits for every admitted reader to release.
// Waiting never holds the registry mutex or blocks admission to other sites.
func (h *Hub) Register(key string, site *Site) (func(), error) {
	h.mu.Lock()
	if _, exists := h.sites[key]; exists {
		h.mu.Unlock()
		return nil, errors.New("site runtime already registered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	entry := &registration{
		data:   site,
		ctx:    ctx,
		cancel: cancel,
	}
	h.sites[key] = entry
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		if h.sites[key] == entry {
			delete(h.sites, key)
		}
		h.mu.Unlock()
		entry.cancel()
		entry.users.Wait()
	}, nil
}
func (h *Hub) AcquireSite(key string) (*Site, context.Context, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.sites[key]
	if !ok {
		return nil, nil, nil, false
	}
	entry.users.Add(1)
	return entry.data, entry.ctx, entry.users.Done, true
}
func (h *Hub) Acquire(key string) (*ingest.Route, context.Context, func(), bool) {
	data, ctx, release, ok := h.AcquireSite(key)
	if !ok {
		return nil, nil, nil, false
	}
	return data.Route, ctx, release, true
}
func (h *Hub) Keys() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]string, 0, len(h.sites))
	for key := range h.sites {
		keys = append(keys, key)
	}
	return keys
}
func (h *Hub) PublishReceiver(state Availability) func() {
	entry := &receiver{
		availability: state,
	}
	h.mu.Lock()
	h.receiver = entry
	h.lastReceiver = state
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.receiver == entry {
			h.receiver = nil
			h.lastReceiver.Serving = false
		}
	}
}
func (h *Hub) Availability() Availability {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.receiver != nil {
		return h.receiver.availability
	}
	return h.lastReceiver
}
