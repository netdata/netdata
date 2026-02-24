// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/snmpsd"
)

const (
	discovererNetListeners = "net_listeners"
	discovererDocker       = "docker"
	discovererK8s          = "k8s"
	discovererSNMP         = "snmp"
)

func Registry() sd.Registry {
	descs := []sd.Descriptor{
		sd.NewDescriptor(
			discovererNetListeners,
			schemaNetListeners,
			parseJSONConfig[netlistensd.Config],
			newNetListenersDiscoverers,
		),
		sd.NewDescriptor(
			discovererK8s,
			schemaK8s,
			parseJSONConfig[[]k8ssd.Config],
			newK8sDiscoverers,
		),
		sd.NewDescriptor(
			discovererSNMP,
			schemaSNMP,
			parseJSONConfig[snmpsd.Config],
			newSNMPDiscoverers,
		),
	}
	if !hostinfo.IsInsideK8sCluster() {
		descs = append(descs, sd.NewDescriptor(
			discovererDocker,
			schemaDocker,
			parseJSONConfig[dockersd.Config],
			newDockerDiscoverers,
		))
	}
	return sd.NewRegistry(descs...)
}

func parseJSONConfig[T any](raw json.RawMessage) (T, error) {
	var cfg T
	if err := json.Unmarshal(raw, &cfg); err != nil {
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

func newK8sDiscoverers(cfgs []k8ssd.Config, source string) ([]model.Discoverer, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("empty %q discoverer config", discovererK8s)
	}

	discs := make([]model.Discoverer, 0, len(cfgs))
	for _, cfg := range cfgs {
		cfg.Source = source
		d, err := k8ssd.NewDiscoverer(cfg)
		if err != nil {
			return nil, err
		}
		discs = append(discs, d)
	}

	return discs, nil
}

func newSNMPDiscoverers(cfg snmpsd.Config, source string) ([]model.Discoverer, error) {
	cfg.Source = source
	d, err := snmpsd.NewDiscoverer(cfg)
	if err != nil {
		return nil, err
	}
	return []model.Discoverer{d}, nil
}
