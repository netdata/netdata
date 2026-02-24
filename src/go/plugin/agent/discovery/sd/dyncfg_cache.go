// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/gohugoio/hashstructure"
	"gopkg.in/yaml.v2"
)

// Internal metadata keys (excluded from JSON output, same pattern as confgroup.Config)
const (
	ikeySource         = "__source__"
	ikeySourceType     = "__source_type__"
	ikeyDiscovererType = "__discoverer_type__"
	ikeyPipelineKey    = "__pipeline_key__"
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

// HashIncludeMap implements hashstructure.HashIncludeMap to exclude __ metadata keys from hashing.
// Same pattern as confgroup.Config.
func (c sdConfig) HashIncludeMap(_ string, k, _ any) (bool, error) {
	s := k.(string)
	return !strings.HasPrefix(s, "__") && !strings.HasSuffix(s, "__"), nil
}

// Hash returns a hash of the config data (excluding __ metadata keys).
// Used for comparing configs to detect changes.
func (c sdConfig) Hash() uint64 {
	hash, _ := hashstructure.Hash(c, nil)
	return hash
}

func (c sdConfig) SetSource(v string) sdConfig         { c[ikeySource] = v; return c }
func (c sdConfig) SetSourceType(v string) sdConfig     { c[ikeySourceType] = v; return c }
func (c sdConfig) SetDiscovererType(v string) sdConfig { c[ikeyDiscovererType] = v; return c }
func (c sdConfig) SetPipelineKey(v string) sdConfig    { c[ikeyPipelineKey] = v; return c }

// ExposedKey returns the logical key for ExposedCache: "discovererType:name"
func (c sdConfig) ExposedKey() string {
	return c.DiscovererType() + ":" + c.Name()
}

// UID returns the unique key for seenConfigs: "source:discovererType:name"
func (c sdConfig) UID() string {
	return c.Source() + ":" + c.ExposedKey()
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

	return m, nil
}

// newSDConfigFromJSON creates an sdConfig from JSON payload.
// Used when receiving dyncfg add/update commands.
// The name parameter is forced onto the config (from dyncfg job ID), matching jobmgr pattern.
func newSDConfigFromJSON(data []byte, name, source, sourceType, discovererType, pipelineKey string) (sdConfig, error) {
	var m sdConfig
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("unmarshal json: got nil map")
	}

	// Force name from dyncfg job ID (matching jobmgr pattern: cfg.SetName(name))
	// This ensures sdConfig.ExposedKey() matches the dyncfg job ID regardless of payload content
	m["name"] = cleanName(name)

	// Add metadata
	m.SetSource(source)
	m.SetSourceType(sourceType)
	m.SetDiscovererType(discovererType)
	m.SetPipelineKey(pipelineKey)

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
