// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"errors"
	"sort"
	"sync"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

// SecretDependencyIndex is the acknowledged job-to-Store dependency
// authority. Reads scale with the selected Store's dependents, not the full
// DynCfg graph.
type SecretDependencyIndex struct {
	mu sync.RWMutex

	jobs    map[string]jobDependency
	byStore map[string]map[string]struct{}
	commits uint64
}

type jobDependency struct {
	display   string
	running   bool
	storeKeys []string
}

type SecretDependencyCensus struct {
	Jobs       int
	StoreKeys  int
	References int
	Commits    uint64
}

func NewSecretDependencyIndex() *SecretDependencyIndex {
	return &SecretDependencyIndex{
		jobs:    make(map[string]jobDependency),
		byStore: make(map[string]map[string]struct{}),
	}
}

func (index *SecretDependencyIndex) PrepareJobChange(
	id string,
	postimage *dyncfg.GraphConfig,
) (func(), error) {
	if index == nil || id == "" {
		return nil, errors.New("jobmgr secrets: invalid dependency change")
	}
	var next *jobDependency
	if postimage != nil {
		if postimage.ID != id {
			return nil, errors.New(
				"jobmgr secrets: dependency postimage identity differs",
			)
		}
		var config confgroup.Config
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil {
			return nil, err
		}
		if config == nil || config.FullName() != id {
			return nil, errors.New(
				"jobmgr secrets: dependency configuration identity differs",
			)
		}
		keys, err := secretresolver.StoreReferences(
			map[string]any(config),
		)
		if err != nil {
			return nil, err
		}
		dependency := jobDependency{
			display:   config.Module() + ":" + config.Name(),
			running:   postimage.Status == dyncfg.StatusRunning.String(),
			storeKeys: append([]string(nil), keys...),
		}
		next = &dependency
	}
	return func() {
		index.commitJobChange(id, next)
	}, nil
}

func (index *SecretDependencyIndex) Affected(
	storeKey string,
	runningOnly bool,
) []secretstore.JobRef {
	if index == nil || storeKey == "" {
		return nil
	}
	index.mu.RLock()
	jobs := index.byStore[storeKey]
	refs := make([]secretstore.JobRef, 0, len(jobs))
	for id := range jobs {
		dependency, ok := index.jobs[id]
		if !ok || runningOnly && !dependency.running {
			continue
		}
		refs = append(refs, secretstore.JobRef{
			ID: id, Display: dependency.display,
		})
	}
	index.mu.RUnlock()
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ID == refs[j].ID {
			return refs[i].Display < refs[j].Display
		}
		return refs[i].ID < refs[j].ID
	})
	return refs
}

func (index *SecretDependencyIndex) Census() SecretDependencyCensus {
	if index == nil {
		return SecretDependencyCensus{}
	}
	index.mu.RLock()
	defer index.mu.RUnlock()
	census := SecretDependencyCensus{
		Jobs: len(index.jobs), StoreKeys: len(index.byStore),
		Commits: index.commits,
	}
	for _, jobs := range index.byStore {
		census.References += len(jobs)
	}
	return census
}

func (index *SecretDependencyIndex) commitJobChange(
	id string,
	next *jobDependency,
) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if previous, ok := index.jobs[id]; ok {
		for _, key := range previous.storeKeys {
			jobs := index.byStore[key]
			delete(jobs, id)
			if len(jobs) == 0 {
				delete(index.byStore, key)
			}
		}
		delete(index.jobs, id)
	}
	if next != nil {
		cloned := jobDependency{
			display: next.display, running: next.running,
			storeKeys: append([]string(nil), next.storeKeys...),
		}
		index.jobs[id] = cloned
		for _, key := range cloned.storeKeys {
			jobs := index.byStore[key]
			if jobs == nil {
				jobs = make(map[string]struct{})
				index.byStore[key] = jobs
			}
			jobs[id] = struct{}{}
		}
	}
	index.commits++
}
