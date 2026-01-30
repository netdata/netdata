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
	pipelineKey    string // key used by PipelineManager (file path or dyncfg:{type}:{name})
	source         string // file path or fn.Source for dyncfg
	sourceType     string // "file" or "dyncfg"
	status         dyncfg.Status
	content        []byte // raw JSON content for dyncfg format
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

func (c *exposedSDConfigs) updateStatus(discovererType, name string, status dyncfg.Status) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		cfg.status = status
	}
}

func (c *exposedSDConfigs) updateContent(discovererType, name string, content []byte) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		cfg.content = content
	}
}

func (c *exposedSDConfigs) getStatus(discovererType, name string) dyncfg.Status {
	c.mux.RLock()
	defer c.mux.RUnlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		return cfg.status
	}
	return dyncfg.StatusAccepted
}

func (c *exposedSDConfigs) getContent(discovererType, name string) []byte {
	c.mux.RLock()
	defer c.mux.RUnlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		// Return a copy to avoid race conditions
		content := make([]byte, len(cfg.content))
		copy(content, cfg.content)
		return content
	}
	return nil
}

func (c *exposedSDConfigs) getPipelineKey(discovererType, name string) string {
	c.mux.RLock()
	defer c.mux.RUnlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		return cfg.pipelineKey
	}
	return ""
}

func (c *exposedSDConfigs) getSource(discovererType, name string) string {
	c.mux.RLock()
	defer c.mux.RUnlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		return cfg.source
	}
	return ""
}

func (c *exposedSDConfigs) getSourceType(discovererType, name string) string {
	c.mux.RLock()
	defer c.mux.RUnlock()
	if cfg, ok := c.items[discovererType+":"+name]; ok {
		return cfg.sourceType
	}
	return ""
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
