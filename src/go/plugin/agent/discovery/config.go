// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type (
	PluginIdentity struct {
		Name string
	}
	PathsConfig struct {
		CollectorsConfigWatchPath []string
		ServiceDiscoveryConfigDir multipath.MultiPath
	}
	BuildContext struct {
		RunMode      policy.RunModePolicy
		Identity     PluginIdentity
		DyncfgOutput dyncfg.Output
		Paths        PathsConfig
		Registry     confgroup.Registry
		ReadPaths    []string
		DummyNames   []string
		FnReg        functions.Registry
	}
)

type PipelineConfig struct {
	BuildContext BuildContext
	Providers    *ProviderCatalog
}

func validatePipelineConfig(cfg PipelineConfig) error {
	if len(cfg.BuildContext.Registry) == 0 {
		return errors.New("empty config registry")
	}
	if cfg.Providers == nil || cfg.Providers.Len() == 0 {
		return errors.New("provider catalog not set")
	}
	return nil
}
