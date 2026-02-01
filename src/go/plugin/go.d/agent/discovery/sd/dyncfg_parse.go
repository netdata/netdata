// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
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
	if got := cfg.Discoverer.Type(); got != "" && got != discovererType {
		return pipeline.Config{}, fmt.Errorf("config has discoverer type %q, expected %q", got, discovererType)
	}

	return cfg, nil
}

// pipelineKey returns a unique key for a dyncfg pipeline.
// Format: "dyncfg:{discovererType}:{name}"
func pipelineKey(discovererType, name string) string {
	return fmt.Sprintf("dyncfg:%s:%s", discovererType, name)
}
