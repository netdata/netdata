// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"gopkg.in/yaml.v2"
)

// parseDyncfgPayload parses a dyncfg JSON payload into a runtime pipeline.Config.
// The dyncfg job name is authoritative and overrides any serialized payload name.
func parseDyncfgPayload(payload []byte, discovererType, name string, configDefaults confgroup.Registry, reg Registry, validate bool) (pipeline.Config, error) {
	if reg == nil {
		return pipeline.Config{}, fmt.Errorf("discoverer registry is not configured")
	}

	var cfg pipeline.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal %s config: %w", discovererType, err)
	}

	cfg.Name = name
	cfg.ConfigDefaults = configDefaults

	// Validate that the config has the expected discoverer type
	if got := cfg.Discoverer.Type(); got != discovererType {
		if got == "" {
			return pipeline.Config{}, fmt.Errorf("no discoverer configured, expected %q", discovererType)
		}
		return pipeline.Config{}, fmt.Errorf("config has discoverer type %q, expected %q", got, discovererType)
	}
	desc, ok := reg.Get(discovererType)
	if !ok {
		return pipeline.Config{}, fmt.Errorf("unknown discoverer type %q", discovererType)
	}
	if _, err := desc.ParseJSONConfig(cfg.Discoverer.Config); err != nil {
		return pipeline.Config{}, fmt.Errorf("invalid %q discoverer config: %w", discovererType, err)
	}

	if validate {
		// Perform full semantic validation (name, discoverer, services rules)
		if err := pipeline.ValidateConfig(cfg); err != nil {
			return pipeline.Config{}, err
		}
	}

	return cfg, nil
}

// pipelineKey returns a unique key for a dyncfg pipeline.
// Format: "dyncfg:{discovererType}:{name}"
func pipelineKey(discovererType, name string) string {
	return fmt.Sprintf("dyncfg:%s:%s", discovererType, name)
}

// configToJSON converts stored config JSON to JSON via typed struct.
// This ensures consistent field ordering matching the struct definition.
func configToJSON(data []byte) ([]byte, error) {
	var cfg pipeline.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	bs, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	return bs, nil
}

// userConfigFromPayload converts a JSON payload to YAML format for user editing.
// The returned YAML always uses jobName (or "test") as the top-level pipeline name.
func userConfigFromPayload(payload []byte, discovererType, jobName string) ([]byte, error) {
	var cfg pipeline.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	cfg.Name = jobName
	if cfg.Name == "" {
		cfg.Name = "test"
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}

	return bs, nil
}
