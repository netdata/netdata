// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func newDiscoveredConfigsCache() *discoveredConfigs {
	return &discoveredConfigs{
		items: make(map[string]map[uint64]confgroup.Config),
	}
}

func newSeenConfigCache() *seenConfigs {
	return &seenConfigs{
		items: make(map[string]*seenConfig),
	}
}

func newExposedConfigCache() *exposedConfigs {
	return &exposedConfigs{
		items: make(map[string]*seenConfig),
	}
}

func newRunningJobsCache() *runningJobs {
	return &runningJobs{
		mux:   sync.Mutex{},
		items: make(map[string]*module.Job),
	}
}

func newRetryingTasksCache() *retryingTasks {
	return &retryingTasks{
		items: make(map[string]*retryTask),
	}
}

type (
	discoveredConfigs struct {
		// [Source][Hash]
		items map[string]map[uint64]confgroup.Config
	}

	seenConfigs struct {
		// [cfg.UID()]
		items map[string]*seenConfig
	}
	exposedConfigs struct {
		// [cfg.FullName()]
		items map[string]*seenConfig
	}
	seenConfig struct {
		cfg    confgroup.Config
		status dyncfg.Status
	}

	runningJobs struct {
		mux sync.Mutex
		// [cfg.FullName()]
		items map[string]*module.Job
	}

	retryingTasks struct {
		// [cfg.UID()]
		items map[string]*retryTask
	}
	retryTask struct {
		cancel context.CancelFunc
	}
)

func (c *discoveredConfigs) add(group *confgroup.Group) (added, removed []confgroup.Config) {
	cfgs, ok := c.items[group.Source]
	if !ok {
		if len(group.Configs) == 0 {
			return nil, nil
		}
		cfgs = make(map[uint64]confgroup.Config)
		c.items[group.Source] = cfgs
	}

	seen := make(map[uint64]bool)

	for _, cfg := range group.Configs {
		hash := cfg.Hash()
		seen[hash] = true

		if _, ok := cfgs[hash]; ok {
			continue
		}

		cfgs[hash] = cfg
		added = append(added, cfg)
	}

	for hash, cfg := range cfgs {
		if !seen[hash] {
			delete(cfgs, hash)
			removed = append(removed, cfg)
		}
	}

	if len(cfgs) == 0 {
		delete(c.items, group.Source)
	}

	return added, removed
}

func (c *seenConfigs) add(sj *seenConfig) {
	c.items[sj.cfg.UID()] = sj
}
func (c *seenConfigs) remove(cfg confgroup.Config) {
	delete(c.items, cfg.UID())
}
func (c *seenConfigs) lookup(cfg confgroup.Config) (*seenConfig, bool) {
	v, ok := c.items[cfg.UID()]
	return v, ok
}

func (c *exposedConfigs) add(sj *seenConfig) {
	c.items[sj.cfg.FullName()] = sj
}
func (c *exposedConfigs) remove(cfg confgroup.Config) {
	delete(c.items, cfg.FullName())
}
func (c *exposedConfigs) lookup(cfg confgroup.Config) (*seenConfig, bool) {
	v, ok := c.items[cfg.FullName()]
	return v, ok
}

func (c *exposedConfigs) lookupByName(module, job string) (*seenConfig, bool) {
	key := module + "_" + job
	if module == job {
		key = job
	}
	v, ok := c.items[key]
	return v, ok
}

func (c *runningJobs) lock() {
	c.mux.Lock()
}
func (c *runningJobs) unlock() {
	c.mux.Unlock()
}
func (c *runningJobs) add(fullName string, job *module.Job) {
	c.items[fullName] = job
}
func (c *runningJobs) remove(fullName string) {
	delete(c.items, fullName)
}
func (c *runningJobs) lookup(fullName string) (*module.Job, bool) {
	j, ok := c.items[fullName]
	return j, ok
}
func (c *runningJobs) forEach(fn func(fullName string, job *module.Job)) {
	for k, j := range c.items {
		fn(k, j)
	}
}

func (c *retryingTasks) add(cfg confgroup.Config, retry *retryTask) {
	c.items[cfg.UID()] = retry
}
func (c *retryingTasks) remove(cfg confgroup.Config) {
	if v, ok := c.lookup(cfg); ok {
		v.cancel()
	}
	delete(c.items, cfg.UID())
}
func (c *retryingTasks) lookup(cfg confgroup.Config) (*retryTask, bool) {
	v, ok := c.items[cfg.UID()]
	return v, ok
}
