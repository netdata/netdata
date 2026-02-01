// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
)

// transformPipelineConfigToDyncfg converts a pipeline.Config to a dyncfg-compatible JSON format.
// This is used when exposing file-based configs through dyncfg so the UI can display them correctly.
func transformPipelineConfigToDyncfg(cfg pipeline.Config, discovererType string) ([]byte, error) {
	if cfg.Discoverer.Empty() {
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
	result := DyncfgNetListenersConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	if nl := cfg.Discoverer.NetListeners; nl != nil {
		result.Discoverer.NetListeners = DyncfgNetListenersOptions{
			Interval: nl.Interval,
			Timeout:  nl.Timeout,
		}
	}

	return result
}

func transformDocker(cfg pipeline.Config) DyncfgDockerConfig {
	result := DyncfgDockerConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	if d := cfg.Discoverer.Docker; d != nil {
		result.Discoverer.Docker = DyncfgDockerOptions{
			Address: d.Address,
			Timeout: d.Timeout,
		}
	}

	return result
}

func transformK8s(cfg pipeline.Config) DyncfgK8sConfig {
	result := DyncfgK8sConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	for _, k := range cfg.Discoverer.K8s {
		opts := DyncfgK8sOptions{
			Role:       k.Role,
			Namespaces: k.Namespaces,
		}

		if k.Selector.Label != "" || k.Selector.Field != "" {
			opts.Selector = &DyncfgK8sSelector{
				Label: k.Selector.Label,
				Field: k.Selector.Field,
			}
		}

		if k.Pod.LocalMode {
			opts.Pod = &DyncfgK8sPodOptions{
				LocalMode: true,
			}
		}

		result.Discoverer.K8s = append(result.Discoverer.K8s, opts)
	}

	return result
}

func transformSNMP(cfg pipeline.Config) DyncfgSNMPConfig {
	result := DyncfgSNMPConfig{
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Services: extractServices(cfg),
	}

	if s := cfg.Discoverer.SNMP; s != nil {
		result.Discoverer.SNMP = DyncfgSNMPOptions{
			RescanInterval:          s.RescanInterval,
			Timeout:                 s.Timeout,
			DeviceCacheTTL:          s.DeviceCacheTTL,
			ParallelScansPerNetwork: s.ParallelScansPerNetwork,
		}

		// Credentials
		for _, c := range s.Credentials {
			result.Discoverer.SNMP.Credentials = append(result.Discoverer.SNMP.Credentials, DyncfgSNMPCredential{
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
			result.Discoverer.SNMP.Networks = append(result.Discoverer.SNMP.Networks, DyncfgSNMPNetwork{
				Subnet:     n.Subnet,
				Credential: n.Credential,
			})
		}
	}

	return result
}

// extractServices extracts service rules from the pipeline config.
func extractServices(cfg pipeline.Config) []DyncfgServiceRule {
	var services []DyncfgServiceRule

	for _, s := range cfg.Services {
		services = append(services, DyncfgServiceRule{
			ID:             s.ID,
			Match:          s.Match,
			ConfigTemplate: s.ConfigTemplate,
		})
	}

	return services
}
