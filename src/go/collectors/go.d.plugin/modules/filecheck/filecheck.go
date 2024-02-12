// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("filecheck", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Filecheck {
	return &Filecheck{
		Config: Config{
			DiscoveryEvery: web.Duration{Duration: time.Second * 30},
			Files:          filesConfig{},
			Dirs: dirsConfig{
				CollectDirSize: true,
			},
		},
		collectedFiles: make(map[string]bool),
		collectedDirs:  make(map[string]bool),
	}
}

type (
	Config struct {
		DiscoveryEvery web.Duration `yaml:"discovery_every"`
		Files          filesConfig  `yaml:"files"`
		Dirs           dirsConfig   `yaml:"dirs"`
	}
	filesConfig struct {
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	}
	dirsConfig struct {
		Include        []string `yaml:"include"`
		Exclude        []string `yaml:"exclude"`
		CollectDirSize bool     `yaml:"collect_dir_size"`
	}
)

type Filecheck struct {
	module.Base
	Config `yaml:",inline"`

	lastDiscoveryFiles time.Time
	curFiles           []string
	collectedFiles     map[string]bool

	lastDiscoveryDirs time.Time
	curDirs           []string
	collectedDirs     map[string]bool

	charts *module.Charts
}

func (Filecheck) Cleanup() {
}

func (fc *Filecheck) Init() bool {
	err := fc.validateConfig()
	if err != nil {
		fc.Errorf("error on validating config: %v", err)
		return false
	}

	charts, err := fc.initCharts()
	if err != nil {
		fc.Errorf("error on charts initialization: %v", err)
		return false
	}
	fc.charts = charts

	fc.Debugf("monitored files: %v", fc.Files.Include)
	fc.Debugf("monitored dirs: %v", fc.Dirs.Include)
	return true
}

func (fc Filecheck) Check() bool {
	return true
}

func (fc *Filecheck) Charts() *module.Charts {
	return fc.charts
}

func (fc *Filecheck) Collect() map[string]int64 {
	ms, err := fc.collect()
	if err != nil {
		fc.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}
