// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
)

func (d *ServiceDiscovery) discovererRegistry() Registry {
	return d.discoverers
}

func (d *ServiceDiscovery) newDiscoverersFromRegistry(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
	desc, ok := d.discovererRegistry().Get(payload.Type())
	if !ok {
		return nil, fmt.Errorf("unsupported discoverer type %q", payload.Type())
	}

	cfg, err := desc.ParseJSONConfig(payload.Config)
	if err != nil {
		return nil, fmt.Errorf("parse %q config: %w", payload.Type(), err)
	}

	return desc.NewDiscoverers(cfg, source)
}

func (d *ServiceDiscovery) hasDiscovererType(typ string) bool {
	_, ok := d.discovererRegistry().Get(typ)
	return ok
}
