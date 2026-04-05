// SPDX-License-Identifier: GPL-3.0-or-later

package discoveryproviders

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
)

func File() discovery.ProviderFactory {
	return discovery.NewProviderFactory("file", func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(ctx.ReadPaths)+len(ctx.Paths.CollectorsConfigWatchPath) == 0 {
			return nil, false, nil
		}

		d, err := file.NewDiscovery(file.Config{
			Registry: ctx.Registry,
			Read:     ctx.ReadPaths,
			Watch:    ctx.Paths.CollectorsConfigWatchPath,
		})
		if err != nil {
			return nil, false, err
		}
		return d, true, nil
	})
}

func Dummy() discovery.ProviderFactory {
	return discovery.NewProviderFactory("dummy", func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(ctx.DummyNames) == 0 {
			return nil, false, nil
		}

		d, err := dummy.NewDiscovery(dummy.Config{
			Registry: ctx.Registry,
			Names:    ctx.DummyNames,
		})
		if err != nil {
			return nil, false, err
		}
		return d, true, nil
	})
}

func SD(registry sd.Registry) discovery.ProviderFactory {
	return discovery.NewProviderFactory("sd", func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(ctx.Paths.ServiceDiscoveryConfigDir) == 0 {
			return nil, false, nil
		}

		d, err := sd.NewServiceDiscovery(sd.Config{
			ConfigDefaults: ctx.Registry,
			PluginName:     ctx.Identity.Name,
			RunModePolicy:  ctx.RunMode,
			Out:            ctx.Out,
			ConfDir:        ctx.Paths.ServiceDiscoveryConfigDir,
			FnReg:          ctx.FnReg,
			Discoverers:    registry,
		})
		if err != nil {
			return nil, false, err
		}
		return d, true, nil
	})
}
