// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"sync"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type activationDependency struct {
	kind string
	name string
}

func (cmf *ConfigModuleFactory) dependencyKeys(config confgroup.Config) ([]activationDependency, error) {
	var keys []activationDependency
	if name := config.Vnode(); name != "" {
		keys = append(keys, activationDependency{
			kind: "vnode",
			name: name,
		})
	}
	stores, err := cmf.config.Configs.StoreReferences(config)
	if err != nil {
		return nil, err
	}
	for _, name := range stores {
		keys = append(keys, activationDependency{
			kind: "secretstore",
			name: name,
		})
	}
	return keys, nil
}

// activationWaitsForDependency distinguishes missing framework-owned names from
// provider failures, which retain the collector's operational retry policy.
func (cmf *ConfigModuleFactory) activationWaitsForDependency(config confgroup.Config, err error) bool {
	if err == nil {
		return false
	}
	failure := jobConfigFailure(err, "")
	if config.Vnode() != "" && failure.Stage == "vnode" {
		switch failure.Reason {
		case "unavailable", "pending_vnode", "missing_vnode":
			return true
		}
	}
	var resolution *secretresolver.AtomicResolveError
	if !errors.As(err, &resolution) || resolution.Kind != secretresolver.AtomicErrorScope ||
		!errors.Is(err, secretstore.ErrStoreNotFound) {
		return false
	}
	keys, keyErr := cmf.dependencyKeys(config)
	if keyErr != nil {
		return false
	}
	for _, key := range keys {
		if key.kind == "secretstore" {
			return true
		}
	}
	return false
}

// Register before the first dependency lookup, using a buffered wake channel.
// Keep the registration through retries and drain only before a new lookup: a
// change after the lookup then remains queued until the waiter observes it.
// The subscriber owns the channel and must not close it while registered.
type activationDependencyIndex struct {
	mu            sync.Mutex
	subscriptions map[string][]activationDependency
	watchers      map[activationDependency]map[string]chan<- struct{}
}

func (index *activationDependencyIndex) replace(id string, keys []activationDependency, wake chan<- struct{}) {
	index.mu.Lock()
	defer index.mu.Unlock()
	index.removeLocked(id)
	if len(keys) == 0 || wake == nil {
		return
	}
	if index.subscriptions == nil {
		index.subscriptions = make(map[string][]activationDependency)
		index.watchers = make(map[activationDependency]map[string]chan<- struct{})
	}
	index.subscriptions[id] = append([]activationDependency(nil), keys...)
	for _, key := range keys {
		if index.watchers[key] == nil {
			index.watchers[key] = make(map[string]chan<- struct{})
		}
		index.watchers[key][id] = wake
	}
}

func (index *activationDependencyIndex) remove(id string) {
	index.mu.Lock()
	defer index.mu.Unlock()
	index.removeLocked(id)
}

func (index *activationDependencyIndex) removeLocked(id string) {
	for _, key := range index.subscriptions[id] {
		delete(index.watchers[key], id)
		if len(index.watchers[key]) == 0 {
			delete(index.watchers, key)
		}
	}
	delete(index.subscriptions, id)
}

func (index *activationDependencyIndex) notify(kind, name string) {
	index.mu.Lock()
	defer index.mu.Unlock()
	for _, wake := range index.watchers[activationDependency{
		kind: kind,
		name: name,
	}] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}
