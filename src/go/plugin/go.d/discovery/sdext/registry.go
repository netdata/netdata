// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	sharedsd "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/dockersd"
	k8ssd2 "github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/netlistensd"
	snmpsd2 "github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/snmpsd"
	"gopkg.in/yaml.v2"
)

const (
	discovererNetListeners = "net_listeners"
	discovererDocker       = "docker"
	discovererK8s          = "k8s"
	discovererSNMP         = "snmp"
)

func Registry() sharedsd.Registry {
	descs := []sharedsd.Descriptor{
		sharedsd.NewDescriptor(
			discovererNetListeners,
			schemaNetListeners,
			parseJSONConfig[netlistensd.Config],
			parseYAMLConfig[netlistensd.Config],
			newNetListenersDiscoverers,
		),
		sharedsd.NewDescriptor(
			discovererK8s,
			schemaK8s,
			parseJSONConfig[[]k8ssd2.Config],
			parseYAMLConfig[[]k8ssd2.Config],
			newK8sDiscoverers,
		),
		sharedsd.NewDescriptor(
			discovererSNMP,
			schemaSNMP,
			parseJSONConfig[snmpsd2.Config],
			parseYAMLConfig[snmpsd2.Config],
			newSNMPDiscoverers,
		),
	}
	if !hostinfo.IsInsideK8sCluster() {
		descs = append(descs, sharedsd.NewDescriptor(
			discovererDocker,
			schemaDocker,
			parseJSONConfig[dockersd.Config],
			parseYAMLConfig[dockersd.Config],
			newDockerDiscoverers,
		))
	}
	return sharedsd.NewRegistry(descs...)
}

func parseJSONConfig[T any](raw json.RawMessage) (T, error) {
	var cfg T
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func parseYAMLConfig[T any](raw []byte) (T, error) {
	var cfg T
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func newNetListenersDiscoverers(cfg netlistensd.Config, source string) ([]model.Discoverer, error) {
	cfg.Source = source
	d, err := netlistensd.NewDiscoverer(cfg)
	if err != nil {
		return nil, err
	}
	return []model.Discoverer{d}, nil
}

func newDockerDiscoverers(cfg dockersd.Config, source string) ([]model.Discoverer, error) {
	cfg.Source = source
	d, err := dockersd.NewDiscoverer(cfg)
	if err != nil {
		return nil, err
	}
	return []model.Discoverer{d}, nil
}

func newK8sDiscoverers(cfgs []k8ssd2.Config, source string) ([]model.Discoverer, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("empty %q discoverer config", discovererK8s)
	}

	discs := make([]model.Discoverer, 0, len(cfgs))
	for _, cfg := range cfgs {
		cfg.Source = source
		d, err := k8ssd2.NewDiscoverer(cfg)
		if err != nil {
			return nil, err
		}
		discs = append(discs, d)
	}

	return discs, nil
}

func newSNMPDiscoverers(cfg snmpsd2.Config, source string) ([]model.Discoverer, error) {
	cfg.Source = source
	d, err := snmpsd2.NewDiscoverer(cfg)
	if err != nil {
		return nil, err
	}
	return []model.Discoverer{d}, nil
}
