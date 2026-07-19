// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"
	"io"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type (
	PlatformPolicy struct {
		IsInsideK8s bool
	}
	PluginIdentity struct {
		Name string
	}
	PathsConfig struct {
		PluginConfigDir           multipath.MultiPath
		CollectorsConfigDir       multipath.MultiPath
		CollectorsConfigWatchPath []string
		ServiceDiscoveryConfigDir multipath.MultiPath
		VarLibDir                 string
	}
	BuildContext struct {
		Policy     PlatformPolicy
		RunMode    policy.RunModePolicy
		Identity   PluginIdentity
		Out        io.Writer
		Paths      PathsConfig
		Registry   confgroup.Registry
		ReadPaths  []string
		DummyNames []string
		FnReg      functions.Registry
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
