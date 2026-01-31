// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
)

// parseDyncfgPayload parses a dyncfg JSON payload into a pipeline.Config.
func parseDyncfgPayload(payload []byte, discovererType string, configDefaults confgroup.Registry) (pipeline.Config, error) {
	switch discovererType {
	case DiscovererNetListeners:
		return parseNetListenersConfig(payload, configDefaults)
	case DiscovererDocker:
		return parseDockerConfig(payload, configDefaults)
	case DiscovererK8s:
		return parseK8sConfig(payload, configDefaults)
	case DiscovererSNMP:
		return parseSNMPConfig(payload, configDefaults)
	default:
		return pipeline.Config{}, fmt.Errorf("unknown discoverer type: %s", discovererType)
	}
}

func parseNetListenersConfig(payload []byte, configDefaults confgroup.Registry) (pipeline.Config, error) {
	var dc DyncfgNetListenersConfig
	if err := json.Unmarshal(payload, &dc); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal net_listeners config: %w", err)
	}

	nlCfg := netlistensd.Config{
		Interval: dc.Interval,
		Timeout:  dc.Timeout,
	}

	return pipeline.Config{
		ConfigDefaults: configDefaults,
		Disabled:       dc.Disabled,
		Name:           dc.Name,
		Discover: []pipeline.DiscoveryConfig{{
			Discoverer:   DiscovererNetListeners,
			NetListeners: nlCfg,
		}},
		Services: convertServices(dc.Services),
	}, nil
}

func parseDockerConfig(payload []byte, configDefaults confgroup.Registry) (pipeline.Config, error) {
	var dc DyncfgDockerConfig
	if err := json.Unmarshal(payload, &dc); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal docker config: %w", err)
	}

	dCfg := dockersd.Config{
		Address: dc.Address,
		Timeout: dc.Timeout,
	}

	return pipeline.Config{
		ConfigDefaults: configDefaults,
		Disabled:       dc.Disabled,
		Name:           dc.Name,
		Discover: []pipeline.DiscoveryConfig{{
			Discoverer: DiscovererDocker,
			Docker:     dCfg,
		}},
		Services: convertServices(dc.Services),
	}, nil
}

func parseK8sConfig(payload []byte, configDefaults confgroup.Registry) (pipeline.Config, error) {
	var dc DyncfgK8sConfig
	if err := json.Unmarshal(payload, &dc); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal k8s config: %w", err)
	}

	k8sCfg := k8ssd.Config{
		Role:       dc.Role,
		Namespaces: dc.Namespaces,
	}

	if dc.Selector != nil {
		k8sCfg.Selector.Label = dc.Selector.Label
		k8sCfg.Selector.Field = dc.Selector.Field
	}

	if dc.Pod != nil {
		k8sCfg.Pod.LocalMode = dc.Pod.LocalMode
	}

	return pipeline.Config{
		ConfigDefaults: configDefaults,
		Disabled:       dc.Disabled,
		Name:           dc.Name,
		Discover: []pipeline.DiscoveryConfig{{
			Discoverer: DiscovererK8s,
			K8s:        []k8ssd.Config{k8sCfg},
		}},
		Services: convertServices(dc.Services),
	}, nil
}

func parseSNMPConfig(payload []byte, configDefaults confgroup.Registry) (pipeline.Config, error) {
	var dc DyncfgSNMPConfig
	if err := json.Unmarshal(payload, &dc); err != nil {
		return pipeline.Config{}, fmt.Errorf("unmarshal snmp config: %w", err)
	}

	snmpCfg := snmpsd.Config{
		RescanInterval:          dc.RescanInterval,
		Timeout:                 dc.Timeout,
		DeviceCacheTTL:          dc.DeviceCacheTTL,
		ParallelScansPerNetwork: dc.ParallelScansPerNetwork,
	}

	for _, c := range dc.Credentials {
		snmpCfg.Credentials = append(snmpCfg.Credentials, snmpsd.CredentialConfig{
			Name:              c.Name,
			Version:           c.Version,
			Community:         c.Community,
			UserName:          c.Username,
			SecurityLevel:     c.SecurityLevel,
			AuthProtocol:      c.AuthProtocol,
			AuthPassphrase:    c.AuthPassword,
			PrivacyProtocol:   c.PrivProtocol,
			PrivacyPassphrase: c.PrivPassword,
		})
	}

	for _, n := range dc.Networks {
		snmpCfg.Networks = append(snmpCfg.Networks, snmpsd.NetworkConfig{
			Subnet:     n.Subnet,
			Credential: n.Credential,
		})
	}

	return pipeline.Config{
		ConfigDefaults: configDefaults,
		Disabled:       dc.Disabled,
		Name:           dc.Name,
		Discover: []pipeline.DiscoveryConfig{{
			Discoverer: DiscovererSNMP,
			SNMP:       snmpCfg,
		}},
		Services: convertServices(dc.Services),
	}, nil
}

func convertServices(services []DyncfgServiceRule) []pipeline.ServiceRuleConfig {
	var result []pipeline.ServiceRuleConfig
	for _, s := range services {
		result = append(result, pipeline.ServiceRuleConfig{
			ID:             s.ID,
			Match:          s.Match,
			ConfigTemplate: s.ConfigTemplate,
		})
	}
	return result
}

// pipelineKey returns a unique key for a dyncfg pipeline.
// Format: "dyncfg:{discovererType}:{name}"
func pipelineKey(discovererType, name string) string {
	return fmt.Sprintf("dyncfg:%s:%s", discovererType, name)
}
