// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"cmp"
	"errors"
	"slices"
	"sync"

	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

// SecretDependencyIndex is the acknowledged job-to-Store dependency
// authority. Reads scale with the selected Store's dependents, not the full
// DynCfg graph.
type SecretDependencyIndex struct {
	configs *secretconfig.ConfigResolver
	mu      sync.RWMutex // guards the maps

	jobs              map[string]jobDependency       // per-job dependency record by job full name
	byStore           map[string]map[string]struct{} // job set by store key (reverse index)
	activationEnabled func(string) bool              // current activation authority, bound before commands start
}

type jobDependency struct {
	display   string
	running   bool
	accepted  bool
	storeKeys []string
}

// SetActivationEnabled binds the current activation authority. Accepted graph
// status alone cannot distinguish enabled work from passive registration.
func (sdi *SecretDependencyIndex) SetActivationEnabled(enabled func(string) bool) {
	sdi.mu.Lock()
	sdi.activationEnabled = enabled
	sdi.mu.Unlock()
}

func NewSecretDependencyIndex(configs *secretconfig.ConfigResolver) *SecretDependencyIndex {
	return &SecretDependencyIndex{
		configs: configs,
		jobs:    make(map[string]jobDependency),
		byStore: make(map[string]map[string]struct{}),
	}
}

func (sdi *SecretDependencyIndex) PrepareJobChange(id string, postimage *dyncfg.GraphConfig) (func(), error) {
	if sdi == nil || id == "" {
		return nil, errors.New("jobmgr secrets: invalid dependency change")
	}
	var next *jobDependency
	if postimage != nil {
		if postimage.ID != id {
			return nil, errors.New("jobmgr secrets: dependency postimage identity differs")
		}
		var config confgroup.Config
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil {
			return nil, err
		}
		if config == nil || config.FullName() != id {
			return nil, errors.New("jobmgr secrets: dependency configuration identity differs")
		}
		keys, err := sdi.configs.StoreReferences(config)
		if err != nil {
			return nil, err
		}

		dependency := jobDependency{
			display:   config.Module() + ":" + config.Name(),
			running:   postimage.Status == dyncfg.StatusRunning.String(),
			accepted:  postimage.Status == dyncfg.StatusAccepted.String(),
			storeKeys: slices.Clone(keys),
		}
		next = &dependency
	}
	return func() {
		sdi.commitJobChange(id, next)
	}, nil
}

func (sdi *SecretDependencyIndex) Affected(storeKey string, activeOnly bool) []secretstore.JobRef {
	if sdi == nil || storeKey == "" {
		return nil
	}
	sdi.mu.RLock()
	jobs := sdi.byStore[storeKey]
	refs := make([]secretstore.JobRef, 0, len(jobs))
	for id := range jobs {
		dependency, ok := sdi.jobs[id]
		if !ok || activeOnly && !dependency.running && !dependency.accepted {
			continue
		}
		refs = append(refs, secretstore.JobRef{
			ID:      id,
			Display: dependency.display,
		})
	}
	var accepted map[string]bool
	if activeOnly {
		accepted = make(map[string]bool, len(refs))
		for _, ref := range refs {
			accepted[ref.ID] = sdi.jobs[ref.ID].accepted
		}
	}
	enabled := sdi.activationEnabled
	sdi.mu.RUnlock()
	if activeOnly {
		refs = slices.DeleteFunc(refs, func(ref secretstore.JobRef) bool {
			return accepted[ref.ID] && (enabled == nil || !enabled(ref.ID))
		})
	}
	slices.SortFunc(refs, func(a, b secretstore.JobRef) int {
		if a.ID != b.ID {
			return cmp.Compare(a.ID, b.ID)
		}
		return cmp.Compare(a.Display, b.Display)
	})
	return refs
}

func (sdi *SecretDependencyIndex) Affects(storeKey, id string, activeOnly bool) bool {
	if sdi == nil || storeKey == "" || id == "" {
		return false
	}
	sdi.mu.RLock()
	if _, ok := sdi.byStore[storeKey][id]; !ok {
		sdi.mu.RUnlock()
		return false
	}
	dependency, ok := sdi.jobs[id]
	enabled := sdi.activationEnabled
	sdi.mu.RUnlock()
	return ok && (!activeOnly || dependency.running || dependency.accepted && enabled != nil && enabled(id))
}

func (sdi *SecretDependencyIndex) commitJobChange(id string, next *jobDependency) {
	sdi.mu.Lock()
	defer sdi.mu.Unlock()
	if previous, ok := sdi.jobs[id]; ok {
		for _, key := range previous.storeKeys {
			jobs := sdi.byStore[key]
			delete(jobs, id)
			if len(jobs) == 0 {
				delete(sdi.byStore, key)
			}
		}
		delete(sdi.jobs, id)
	}
	if next != nil {
		cloned := jobDependency{
			display:   next.display,
			running:   next.running,
			accepted:  next.accepted,
			storeKeys: slices.Clone(next.storeKeys),
		}
		sdi.jobs[id] = cloned
		for _, key := range cloned.storeKeys {
			jobs := sdi.byStore[key]
			if jobs == nil {
				jobs = make(map[string]struct{})
				sdi.byStore[key] = jobs
			}
			jobs[id] = struct{}{}
		}
	}
}
