// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"io"
	"os"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/hostinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

	"github.com/goccy/go-yaml"
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

func (a *Agent) loadEnabledModules(cfg config) module.Registry {
	a.Info("loading modules")

	all := a.RunModule == "all" || a.RunModule == ""
	enabled := module.Registry{}

	for name, creator := range a.ModuleRegistry {
		if !all && a.RunModule != name {
			continue
		}
		if all {
			// Known issue: go.d/logind high CPU usage on Alma Linux8 (https://github.com/netdata/netdata/issues/15930)
			if !cfg.isExplicitlyEnabled(name) && (creator.Disabled || name == "logind" && hostinfo.SystemdVersion == 239) {
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

func (a *Agent) buildDiscoveryConf(enabled module.Registry) discovery.Config {
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

	if len(a.CollectorsConfDir) == 0 {
		if hostinfo.IsInsideK8sCluster() {
			return discovery.Config{Registry: reg}
		}
		a.Info("modules conf dir not provided, will use default config for all enabled modules")
		for name := range enabled {
			dummyPaths = append(dummyPaths, name)
		}
		return discovery.Config{
			Registry: reg,
			Dummy:    dummy.Config{Names: dummyPaths},
		}
	}

	for name := range enabled {
		cfgName := name + ".conf"
		a.Debugf("looking for '%s' in %v", cfgName, a.CollectorsConfDir)

		path, err := a.CollectorsConfDir.Find(cfgName)
		if hostinfo.IsInsideK8sCluster() {
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

	return discovery.Config{
		Registry: reg,
		File: file.Config{
			Read:  readPaths,
			Watch: a.CollectorsConfigWatchPath,
		},
		Dummy: dummy.Config{
			Names: dummyPaths,
		},
		SD: sd.Config{
			ConfDir: a.ServiceDiscoveryConfigDir,
		},
	}
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

var (
	envNDStockConfigDir = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
)

func isStockConfig(path string) bool {
	if envNDStockConfigDir == "" {
		return false
	}
	return strings.HasPrefix(path, envNDStockConfigDir)
}
