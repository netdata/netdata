// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"

	"gopkg.in/yaml.v2"
)

// Internal metadata keys (excluded from JSON output, same pattern as confgroup.Config)
const (
	ikeySource         = "__source__"
	ikeySourceType     = "__source_type__"
	ikeyDiscovererType = "__discoverer_type__"
	ikeyPipelineKey    = "__pipeline_key__"
	ikeyStatus         = "__status__"
)

// sdConfig represents a service discovery pipeline configuration.
// Uses map[string]any with __ metadata fields, same pattern as confgroup.Config.
// The actual config data is stored alongside metadata and parsed to pipeline.Config only when needed.
type sdConfig map[string]any

func (c sdConfig) Source() string         { v, _ := c[ikeySource].(string); return v }
func (c sdConfig) SourceType() string     { v, _ := c[ikeySourceType].(string); return v }
func (c sdConfig) DiscovererType() string { v, _ := c[ikeyDiscovererType].(string); return v }
func (c sdConfig) PipelineKey() string    { v, _ := c[ikeyPipelineKey].(string); return v }
func (c sdConfig) Name() string           { v, _ := c["name"].(string); return v }

func (c sdConfig) Status() dyncfg.Status {
	v, _ := c[ikeyStatus].(dyncfg.Status)
	return v
}

func (c sdConfig) SetSource(v string) sdConfig         { c[ikeySource] = v; return c }
func (c sdConfig) SetSourceType(v string) sdConfig     { c[ikeySourceType] = v; return c }
func (c sdConfig) SetDiscovererType(v string) sdConfig { c[ikeyDiscovererType] = v; return c }
func (c sdConfig) SetPipelineKey(v string) sdConfig    { c[ikeyPipelineKey] = v; return c }
func (c sdConfig) SetStatus(v dyncfg.Status) sdConfig  { c[ikeyStatus] = v; return c }

// Key returns the logical key for exposedConfigs: "discovererType:name"
func (c sdConfig) Key() string {
	return c.DiscovererType() + ":" + c.Name()
}

// UID returns the unique key for seenConfigs: "source:discovererType:name"
func (c sdConfig) UID() string {
	return c.Source() + ":" + c.Key()
}

// SourceTypePriority returns priority based on source type.
// Higher value = higher priority. Matches confgroup.Config pattern.
func (c sdConfig) SourceTypePriority() int {
	switch c.SourceType() {
	case confgroup.TypeDyncfg:
		return 16
	case confgroup.TypeUser:
		return 8
	case confgroup.TypeStock:
		return 2
	default:
		return 0
	}
}

// Clone returns a deep copy of the config using JSON marshal/unmarshal.
func (c sdConfig) Clone() sdConfig {
	data, err := json.Marshal(c)
	if err != nil {
		// Fallback to shallow copy if marshal fails (shouldn't happen)
		clone := make(sdConfig, len(c))
		for k, v := range c {
			clone[k] = v
		}
		return clone
	}
	var clone sdConfig
	if err := json.Unmarshal(data, &clone); err != nil {
		// Fallback to shallow copy
		clone = make(sdConfig, len(c))
		for k, v := range c {
			clone[k] = v
		}
		return clone
	}
	// Restore metadata from original (JSON may lose type info for type aliases)
	clone.SetSource(c.Source())
	clone.SetSourceType(c.SourceType())
	clone.SetDiscovererType(c.DiscovererType())
	clone.SetPipelineKey(c.PipelineKey())
	clone.SetStatus(c.Status())
	return clone
}

// ToPipelineConfig converts sdConfig to pipeline.Config for actually running the pipeline.
// This parses the config data (excluding __ fields) into the typed struct.
func (c sdConfig) ToPipelineConfig(configDefaults confgroup.Registry) (pipeline.Config, error) {
	// Marshal without __ fields, then unmarshal to pipeline.Config
	data := c.DataJSON()

	var cfg pipeline.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal pipeline config: %w", err)
	}

	cfg.ConfigDefaults = configDefaults

	// Set source based on source type
	switch c.SourceType() {
	case confgroup.TypeDyncfg:
		cfg.Source = fmt.Sprintf("dyncfg=%s", c.Source())
	default:
		cfg.Source = fmt.Sprintf("file=%s", c.Source())
	}

	return cfg, nil
}

// DataJSON returns JSON representation of config data (excluding __ metadata fields).
// Used for dyncfg get command and for converting to pipeline.Config.
func (c sdConfig) DataJSON() []byte {
	data := make(map[string]any, len(c))
	for k, v := range c {
		if !strings.HasPrefix(k, "__") {
			data[k] = v
		}
	}
	b, _ := json.Marshal(data)
	return b
}

// cleanName sanitizes a name for use in dyncfg IDs.
// Replaces spaces and colons with underscores to avoid parsing issues.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

// newSDConfigFromYAML creates an sdConfig from YAML bytes.
// Used when loading file configs. Cleans the name for dyncfg compatibility.
func newSDConfigFromYAML(data []byte, source, sourceType, pipelineKey string) (sdConfig, error) {
	// First unmarshal to pipeline.Config to get discoverer type and apply YAML processing
	var cfg pipeline.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// Now marshal to JSON and unmarshal to map for sdConfig
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal to json: %w", err)
	}

	var m sdConfig
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return nil, fmt.Errorf("unmarshal to map: %w", err)
	}

	// Clean the name for dyncfg compatibility
	if name := m.Name(); name != "" {
		m["name"] = cleanName(name)
	}

	// Add metadata
	m.SetSource(source)
	m.SetSourceType(sourceType)
	m.SetDiscovererType(cfg.Discoverer.Type())
	m.SetPipelineKey(pipelineKey)
	m.SetStatus(dyncfg.StatusAccepted)

	return m, nil
}

// newSDConfigFromJSON creates an sdConfig from JSON payload.
// Used when receiving dyncfg add/update commands. Cleans the name for dyncfg compatibility.
func newSDConfigFromJSON(data []byte, source, sourceType, discovererType, pipelineKey string) (sdConfig, error) {
	var m sdConfig
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	// Clean the name for dyncfg compatibility
	if name := m.Name(); name != "" {
		m["name"] = cleanName(name)
	}

	// Add metadata
	m.SetSource(source)
	m.SetSourceType(sourceType)
	m.SetDiscovererType(discovererType)
	m.SetPipelineKey(pipelineKey)
	m.SetStatus(dyncfg.StatusAccepted)

	return m, nil
}

// sourceTypeFromPath determines the source type (stock/user) from a file path.
func sourceTypeFromPath(path string) string {
	// User configs are in /etc/ (e.g., /etc/netdata/sd.d/)
	// Stock configs are in /usr/lib/ or similar system paths
	if strings.Contains(path, "/etc/") {
		return confgroup.TypeUser
	}
	return confgroup.TypeStock
}

// seenSDConfigs tracks all discovered SD configs by unique ID (source + key).
// Multiple sources can produce configs with the same logical name.
type seenSDConfigs struct {
	mux   sync.RWMutex
	items map[string]sdConfig // [UID()]
}

func newSeenSDConfigs() *seenSDConfigs {
	return &seenSDConfigs{
		items: make(map[string]sdConfig),
	}
}

func (c *seenSDConfigs) add(cfg sdConfig) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.items[cfg.UID()] = cfg
}

func (c *seenSDConfigs) remove(uid string) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.items, uid)
}

// lookup returns a deep copy of the config to avoid data races.
func (c *seenSDConfigs) lookup(uid string) (sdConfig, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	cfg, ok := c.items[uid]
	if !ok {
		return nil, false
	}
	return cfg.Clone(), true
}

// lookupBySource returns deep copies of configs from the given source.
func (c *seenSDConfigs) lookupBySource(source string) []sdConfig {
	c.mux.RLock()
	defer c.mux.RUnlock()
	var result []sdConfig
	for _, cfg := range c.items {
		if cfg.Source() == source {
			result = append(result, cfg.Clone())
		}
	}
	return result
}

// exposedSDConfigs tracks SD configs exposed via dyncfg UI by logical key.
// Only one config per logical key (discovererType:name) is exposed at a time.
type exposedSDConfigs struct {
	mux   sync.RWMutex
	items map[string]sdConfig // [Key()]
}

func newExposedSDConfigs() *exposedSDConfigs {
	return &exposedSDConfigs{
		items: make(map[string]sdConfig),
	}
}

func (c *exposedSDConfigs) add(cfg sdConfig) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.items[cfg.Key()] = cfg
}

func (c *exposedSDConfigs) remove(key string) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.items, key)
}

// lookup returns a deep copy of the config to avoid data races.
// Callers can safely read from the returned config without synchronization.
func (c *exposedSDConfigs) lookup(key string) (sdConfig, bool) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	cfg, ok := c.items[key]
	if !ok {
		return nil, false
	}
	return cfg.Clone(), true
}

func (c *exposedSDConfigs) updateStatus(key string, status dyncfg.Status) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if cfg, ok := c.items[key]; ok {
		cfg.SetStatus(status)
	}
}

func (c *exposedSDConfigs) forEach(fn func(cfg sdConfig)) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for _, cfg := range c.items {
		fn(cfg)
	}
}

func (c *exposedSDConfigs) forEachBySource(source string, fn func(cfg sdConfig)) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for _, cfg := range c.items {
		if cfg.Source() == source {
			fn(cfg)
		}
	}
}

func (c *exposedSDConfigs) count() int {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return len(c.items)
}
