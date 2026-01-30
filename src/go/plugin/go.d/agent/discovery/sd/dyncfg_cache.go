// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
)

// sdConfig represents a service discovery pipeline configuration for dyncfg
type sdConfig struct {
	discovererType string // net_listeners, docker, k8s, snmp
	name           string
	source         string // file path or "dyncfg"
	sourceType     string // "file" or "dyncfg"
	status         dyncfg.Status
	content        []byte // raw YAML content
}

func (c *sdConfig) key() string {
	return c.discovererType + ":" + c.name
}

func (c *sdConfig) isDyncfg() bool {
	return c.sourceType == "dyncfg"
}

// exposedSDConfigs tracks SD configs exposed via dyncfg UI
type exposedSDConfigs struct {
	mux   sync.RWMutex
	items map[string]*sdConfig // [discovererType:name]
}

func newExposedSDConfigs() *exposedSDConfigs {
	return &exposedSDConfigs{
		items: make(map[string]*sdConfig),
	}
}

func (c *exposedSDConfigs) add(cfg *sdConfig) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.items[cfg.key()] = cfg
}

func (c *exposedSDConfigs) remove(discovererType, name string) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.items, discovererType+":"+name)
}

func (c *exposedSDConfigs) lookup(discovererType, name string) (*sdConfig, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	cfg, ok := c.items[discovererType+":"+name]
	return cfg, ok
}

func (c *exposedSDConfigs) lookupByKey(key string) (*sdConfig, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	cfg, ok := c.items[key]
	return cfg, ok
}

func (c *exposedSDConfigs) forEach(fn func(cfg *sdConfig)) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for _, cfg := range c.items {
		fn(cfg)
	}
}

func (c *exposedSDConfigs) forEachByType(discovererType string, fn func(cfg *sdConfig)) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for _, cfg := range c.items {
		if cfg.discovererType == discovererType {
			fn(cfg)
		}
	}
}

func (c *exposedSDConfigs) count() int {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return len(c.items)
}

func (c *exposedSDConfigs) countByType(discovererType string) int {
	c.mux.RLock()
	defer c.mux.RUnlock()
	count := 0
	for _, cfg := range c.items {
		if cfg.discovererType == discovererType {
			count++
		}
	}
	return count
}
