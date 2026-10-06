// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// The Agent requires an enabled discovery source. With no files configured,
// this plugin deliberately discovers no default jobs and waits for DynCfg.
func manualDiscovery() discovery.ProviderFactory {
	return discovery.NewProviderFactory("manual", func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(ctx.ReadPaths)+len(ctx.Paths.CollectorsConfigWatchPath) > 0 {
			return nil, false, nil
		}
		return manualSource{}, true, nil
	})
}

type manualSource struct{}

func (manualSource) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	defer close(out)
	<-ctx.Done()
}
