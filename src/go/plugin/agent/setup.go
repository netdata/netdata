// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"io"
	"os"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

func (a *Agent) loadPluginConfig() config {
	a.Info("loading config file")

	if len(a.ConfigDir) == 0 {
		a.Info("config dir not provided, will use defaults")
		return defaultConfig()
	}

	cfgPath := a.Name + ".conf"
	a.Debugf("looking for '%s' in %v", cfgPath, a.ConfigDir)

	path, err := a.ConfigDir.Find(cfgPath)
	if err != nil || path == "" {
		a.Warning("couldn't find config, will use defaults")
		return defaultConfig()
	}
	a.Infof("found '%s", path)

	cfg := defaultConfig()
	if err := loadYAML(&cfg, path); err != nil {
		a.Warningf("couldn't load config '%s': %v, will use defaults", path, err)
		return defaultConfig()
	}
	a.Info("config successfully loaded")
	return cfg
}

func (a *Agent) loadEnabledModules(cfg config) collectorapi.Registry {
	a.Info("loading modules")

	all := a.RunModule == "all" || a.RunModule == ""
	enabled := collectorapi.Registry{}

	for name, creator := range a.ModuleRegistry {
		if !all && a.RunModule != name {
			continue
		}
		if all {
			if !cfg.isExplicitlyEnabled(name) && creator.Disabled {
				a.Infof("'%s' module disabled by default, should be explicitly enabled in the config", name)
				continue
			}
			if !cfg.isImplicitlyEnabled(name) {
				a.Infof("'%s' module disabled in the config file", name)
				continue
			}
		}
		enabled[name] = creator
	}

	a.Infof("enabled/registered modules: %d/%d", len(enabled), len(a.ModuleRegistry))

	return enabled
}

func (a *Agent) buildDiscoveryConf(enabled collectorapi.Registry, fnReg functions.Registry) discovery.Config {
	a.Info("building discovery config")

	reg := confgroup.Registry{}
	for name, creator := range enabled {
		reg.Register(name, confgroup.Default{
			MinUpdateEvery:     a.MinUpdateEvery,
			UpdateEvery:        creator.UpdateEvery,
			AutoDetectionRetry: creator.AutoDetectionRetry,
			Priority:           creator.Priority,
		})
	}

	var readPaths, dummyPaths []string

	watchPaths := a.CollectorsConfigWatchPath
	sdConfDir := a.ServiceDiscoveryConfigDir
	if a.DisableServiceDiscovery {
		dummyPaths = nil
		sdConfDir = nil
	}

	cfg := discovery.Config{
		Registry: reg,
		BuildContext: discovery.BuildContext{
			Policy: discovery.PlatformPolicy{
				IsInsideK8s: a.IsInsideK8s,
			},
			RunMode: a.runModePolicy,
			Identity: discovery.PluginIdentity{
				Name: a.Name,
			},
			Out: a.Out,
			Paths: discovery.PathsConfig{
				PluginConfigDir:           a.ConfigDir,
				CollectorsConfigDir:       a.CollectorsConfDir,
				CollectorsConfigWatchPath: watchPaths,
				ServiceDiscoveryConfigDir: sdConfDir,
				VarLibDir:                 a.VarLibDir,
			},
			Registry:   reg,
			ReadPaths:  readPaths,
			DummyNames: dummyPaths,
			FnReg:      fnReg,
		},
		Providers: append([]discovery.ProviderFactory(nil), a.DiscoveryProviders...),
	}

	if len(a.CollectorsConfDir) == 0 {
		if a.IsInsideK8s {
			cfg.Providers = nil
			return cfg
		}
		a.Info("modules conf dir not provided, will use default config for all enabled modules")
		for name := range enabled {
			dummyPaths = append(dummyPaths, name)
		}
		watchPaths = nil
		cfg.BuildContext.Paths.CollectorsConfigWatchPath = watchPaths
		cfg.BuildContext.DummyNames = dummyPaths
		return cfg
	}

	for name := range enabled {
		cfgName := name + ".conf"
		a.Debugf("looking for '%s' in %v", cfgName, a.CollectorsConfDir)

		path, err := a.CollectorsConfDir.Find(cfgName)
		if a.IsInsideK8s {
			if err != nil {
				a.Infof("not found '%s', won't use default (reading stock configs is disabled in k8s)", cfgName)
				continue
			} else if isStockConfig(path) {
				a.Infof("found '%s', but won't load it (reading stock configs is disabled in k8s)", cfgName)
				continue
			}
		}
		if err != nil {
			a.Infof("couldn't find '%s' module config, will use default config", name)
			dummyPaths = append(dummyPaths, name)
		} else {
			a.Debugf("found '%s", path)
			readPaths = append(readPaths, path)
		}
	}

	a.Infof("dummy/read/watch paths: %d/%d/%d", len(dummyPaths), len(readPaths), len(a.CollectorsConfigWatchPath))
	cfg.BuildContext.Paths.CollectorsConfigWatchPath = watchPaths
	cfg.BuildContext.Paths.ServiceDiscoveryConfigDir = sdConfDir
	cfg.BuildContext.ReadPaths = readPaths
	cfg.BuildContext.DummyNames = dummyPaths

	return cfg
}

func (a *Agent) setupVnodeRegistry() map[string]*vnodes.VirtualNode {
	a.Debugf("looking for 'vnodes/' in %v", a.ConfigDir)
	if len(a.ConfigDir) == 0 {
		return nil
	}

	dirPath, err := a.ConfigDir.Find("vnodes/")
	if err != nil || dirPath == "" {
		return nil
	}

	reg := vnodes.Load(dirPath)
	a.Infof("found '%s' (%d vhosts)", dirPath, len(reg))

	return reg
}

func loadYAML(conf any, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if err = yaml.NewDecoder(f).Decode(conf); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}

func isStockConfig(path string) bool {
	return pluginconfig.IsStock(path)
}
