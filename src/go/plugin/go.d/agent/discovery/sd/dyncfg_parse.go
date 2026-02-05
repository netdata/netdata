// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"

	"gopkg.in/yaml.v2"
)

// parseDyncfgPayload parses a dyncfg JSON payload into a pipeline.Config.
// Since pipeline.Config now has proper JSON tags matching the schema,
// we can unmarshal directly without type-specific parsing.
func parseDyncfgPayload(payload []byte, discovererType string, configDefaults confgroup.Registry) (pipeline.Config, error) {
	var cfg pipeline.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal %s config: %w", discovererType, err)
	}

	cfg.ConfigDefaults = configDefaults

	// Validate that the config has the expected discoverer type
	if got := cfg.Discoverer.Type(); got != discovererType {
		if got == "" {
			return pipeline.Config{}, fmt.Errorf("no discoverer configured, expected %q", discovererType)
		}
		return pipeline.Config{}, fmt.Errorf("config has discoverer type %q, expected %q", got, discovererType)
	}

	// Perform full semantic validation (name, discoverer, services rules)
	if err := pipeline.ValidateConfig(cfg); err != nil {
		return pipeline.Config{}, err
	}

	return cfg, nil
}

// pipelineKey returns a unique key for a dyncfg pipeline.
// Format: "dyncfg:{discovererType}:{name}"
func pipelineKey(discovererType, name string) string {
	return fmt.Sprintf("dyncfg:%s:%s", discovererType, name)
}

// userConfigFromPayload converts a JSON payload to YAML format for user editing.
// It unmarshals JSON into pipeline.Config, then marshals to YAML.
// If jobName is provided (non-empty), it overrides the name from payload.
// This ensures consistent field ordering and validates the structure.
func userConfigFromPayload(payload []byte, discovererType, jobName string) ([]byte, error) {
	var cfg pipeline.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	// Use jobName if provided, otherwise keep name from payload
	if jobName != "" {
		cfg.Name = jobName
	}
	// If still no name, use default
	if cfg.Name == "" {
		cfg.Name = "test"
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}

	return bs, nil
}
