// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
			DiscoveryEvery: web.Duration(time.Second * 30),
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
		UpdateEvery    int          `yaml:"update_every" json:"update_every"`
		DiscoveryEvery web.Duration `yaml:"discovery_every" json:"discovery_every"`
		Files          filesConfig  `yaml:"files" json:"files"`
		Dirs           dirsConfig   `yaml:"dirs" json:"dirs"`
	}
	filesConfig struct {
		Include []string `yaml:"include" json:"include"`
		Exclude []string `yaml:"exclude" json:"exclude"`
	}
	dirsConfig struct {
		Include        []string `yaml:"include" json:"include"`
		Exclude        []string `yaml:"exclude" json:"exclude"`
		CollectDirSize bool     `yaml:"collect_dir_size" json:"collect_dir_size"`
	}
)

type Filecheck struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	lastDiscoveryFiles time.Time
	curFiles           []string
	collectedFiles     map[string]bool

	lastDiscoveryDirs time.Time
	curDirs           []string
	collectedDirs     map[string]bool
}

func (fc *Filecheck) Configuration() any {
	return fc.Config
}

func (fc *Filecheck) Init() error {
	err := fc.validateConfig()
	if err != nil {
		fc.Errorf("error on validating config: %v", err)
		return err
	}

	charts, err := fc.initCharts()
	if err != nil {
		fc.Errorf("error on charts initialization: %v", err)
		return err
	}
	fc.charts = charts

	fc.Debugf("monitored files: %v", fc.Files.Include)
	fc.Debugf("monitored dirs: %v", fc.Dirs.Include)

	return nil
}

func (fc *Filecheck) Check() error {
	return nil
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

func (fc *Filecheck) Cleanup() {
}
