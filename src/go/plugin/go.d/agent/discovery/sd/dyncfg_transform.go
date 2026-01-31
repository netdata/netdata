// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
)

// transformPipelineConfigToDyncfg converts a pipeline.Config to a dyncfg-compatible JSON format.
// This is used when exposing file-based configs through dyncfg so the UI can display them correctly.
func transformPipelineConfigToDyncfg(cfg pipeline.Config, discovererType string) ([]byte, error) {
	if len(cfg.Discover) == 0 {
		return nil, nil
	}

	switch discovererType {
	case DiscovererNetListeners:
		return json.Marshal(transformNetListeners(cfg))
	case DiscovererDocker:
		return json.Marshal(transformDocker(cfg))
	case DiscovererK8s:
		return json.Marshal(transformK8s(cfg))
	case DiscovererSNMP:
		return json.Marshal(transformSNMP(cfg))
	default:
		// Fallback: return minimal config
		return json.Marshal(map[string]string{"name": cfg.Name})
	}
}

func transformNetListeners(cfg pipeline.Config) DyncfgNetListenersConfig {
	disc := cfg.Discover[0]
	nl := disc.NetListeners

	result := DyncfgNetListenersConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	if nl.Interval != nil {
		result.Interval = nl.Interval.String()
	}
	if nl.Timeout.Duration() != 0 {
		result.Timeout = nl.Timeout.String()
	}

	return result
}

func transformDocker(cfg pipeline.Config) DyncfgDockerConfig {
	disc := cfg.Discover[0]
	d := disc.Docker

	result := DyncfgDockerConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Address:  d.Address,
		Services: extractServices(cfg),
	}

	if d.Timeout.Duration() != 0 {
		result.Timeout = d.Timeout.String()
	}

	return result
}

func transformK8s(cfg pipeline.Config) DyncfgK8sConfig {
	disc := cfg.Discover[0]

	result := DyncfgK8sConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	if len(disc.K8s) > 0 {
		k := disc.K8s[0]

		result.Role = k.Role
		result.Namespaces = k.Namespaces

		if k.Selector.Label != "" || k.Selector.Field != "" {
			result.Selector = &DyncfgK8sSelector{
				Label: k.Selector.Label,
				Field: k.Selector.Field,
			}
		}

		if k.Pod.LocalMode {
			result.Pod = &DyncfgK8sPodOptions{
				LocalMode: true,
			}
		}
	}

	return result
}

func transformSNMP(cfg pipeline.Config) DyncfgSNMPConfig {
	disc := cfg.Discover[0]
	s := disc.SNMP

	result := DyncfgSNMPConfig{
		Name:                    cfg.Name,
		Disabled:                cfg.Disabled,
		ParallelScansPerNetwork: s.ParallelScansPerNetwork,
		Services:                extractServices(cfg),
	}

	if s.RescanInterval != nil {
		result.RescanInterval = s.RescanInterval.String()
	}
	if s.Timeout.Duration() != 0 {
		result.Timeout = s.Timeout.String()
	}
	if s.DeviceCacheTTL != nil {
		result.DeviceCacheTTL = s.DeviceCacheTTL.String()
	}

	// Credentials
	for _, c := range s.Credentials {
		result.Credentials = append(result.Credentials, DyncfgSNMPCredential{
			Name:          c.Name,
			Version:       c.Version,
			Community:     c.Community,
			Username:      c.UserName,
			SecurityLevel: c.SecurityLevel,
			AuthProtocol:  c.AuthProtocol,
			AuthPassword:  c.AuthPassphrase,
			PrivProtocol:  c.PrivacyProtocol,
			PrivPassword:  c.PrivacyPassphrase,
		})
	}

	// Networks
	for _, n := range s.Networks {
		result.Networks = append(result.Networks, DyncfgSNMPNetwork{
			Subnet:     n.Subnet,
			Credential: n.Credential,
		})
	}

	return result
}

// extractServices extracts service rules from the pipeline config.
// It handles both the new "services" format and legacy "classify/compose" format.
func extractServices(cfg pipeline.Config) []DyncfgServiceRule {
	var services []DyncfgServiceRule

	if len(cfg.Services) > 0 {
		// New format
		for _, s := range cfg.Services {
			services = append(services, DyncfgServiceRule{
				ID:             s.ID,
				Match:          s.Match,
				ConfigTemplate: s.ConfigTemplate,
			})
		}
	} else if len(cfg.Classify) > 0 || len(cfg.Compose) > 0 {
		// Legacy format - convert to services
		converted, _ := pipeline.ConvertOldToServices(cfg.Classify, cfg.Compose)
		for _, s := range converted {
			services = append(services, DyncfgServiceRule{
				ID:             s.ID,
				Match:          s.Match,
				ConfigTemplate: s.ConfigTemplate,
			})
		}
	}

	return services
}
