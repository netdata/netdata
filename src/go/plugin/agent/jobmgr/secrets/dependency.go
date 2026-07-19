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

func (sdi *SecretDependencyIndex) PrepareJobChange(
	id string,
	postimage *dyncfg.GraphConfig,
) (func(), error) {
	if sdi == nil || id == "" {
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
		sdi.commitJobChange(id, next)
	}, nil
}

func (sdi *SecretDependencyIndex) Affected(
	storeKey string,
	runningOnly bool,
) []secretstore.JobRef {
	if sdi == nil || storeKey == "" {
		return nil
	}
	sdi.mu.RLock()
	jobs := sdi.byStore[storeKey]
	refs := make([]secretstore.JobRef, 0, len(jobs))
	for id := range jobs {
		dependency, ok := sdi.jobs[id]
		if !ok || runningOnly && !dependency.running {
			continue
		}
		refs = append(refs, secretstore.JobRef{
			ID: id, Display: dependency.display,
		})
	}
	sdi.mu.RUnlock()
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ID == refs[j].ID {
			return refs[i].Display < refs[j].Display
		}
		return refs[i].ID < refs[j].ID
	})
	return refs
}

func (sdi *SecretDependencyIndex) Census() SecretDependencyCensus {
	if sdi == nil {
		return SecretDependencyCensus{}
	}
	sdi.mu.RLock()
	defer sdi.mu.RUnlock()
	census := SecretDependencyCensus{
		Jobs: len(sdi.jobs), StoreKeys: len(sdi.byStore),
		Commits: sdi.commits,
	}
	for _, jobs := range sdi.byStore {
		census.References += len(jobs)
	}
	return census
}

func (sdi *SecretDependencyIndex) commitJobChange(
	id string,
	next *jobDependency,
) {
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
			display: next.display, running: next.running,
			storeKeys: append([]string(nil), next.storeKeys...),
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
	sdi.commits++
}
