// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	_ "embed"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
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
		Config: func() any { return &Config{} },
	})
}

func New() *Filecheck {
	return &Filecheck{
		Config: Config{
			DiscoveryEvery: confopt.Duration(time.Minute * 1),
			Files:          filesConfig{},
			Dirs:           dirsConfig{CollectDirSize: false},
		},
		charts:    &module.Charts{},
		seenFiles: newSeenItems(),
		seenDirs:  newSeenItems(),
	}
}

type (
	Config struct {
		UpdateEvery    int              `yaml:"update_every,omitempty" json:"update_every"`
		DiscoveryEvery confopt.Duration `yaml:"discovery_every,omitempty" json:"discovery_every"`
		Files          filesConfig      `yaml:"files" json:"files"`
		Dirs           dirsConfig       `yaml:"dirs" json:"dirs"`
	}
	filesConfig struct {
		Include []string `yaml:"include" json:"include"`
		Exclude []string `yaml:"exclude,omitempty" json:"exclude"`
	}
	dirsConfig struct {
		Include        []string `yaml:"include" json:"include"`
		Exclude        []string `yaml:"exclude,omitempty" json:"exclude"`
		CollectDirSize bool     `yaml:"collect_dir_size" json:"collect_dir_size"`
	}
)

type Filecheck struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	filesFilter       matcher.Matcher
	lastDiscFilesTime time.Time
	curFiles          []string
	seenFiles         *seenItems

	dirsFilter       matcher.Matcher
	lastDiscDirsTime time.Time
	curDirs          []string
	seenDirs         *seenItems
}

func (f *Filecheck) Configuration() any {
	return f.Config
}

func (f *Filecheck) Init() error {
	err := f.validateConfig()
	if err != nil {
		f.Errorf("config validation: %v", err)
		return err
	}

	ff, err := f.initFilesFilter()
	if err != nil {
		f.Errorf("files filter initialization: %v", err)
		return err
	}
	f.filesFilter = ff

	df, err := f.initDirsFilter()
	if err != nil {
		f.Errorf("dirs filter initialization: %v", err)
		return err
	}
	f.dirsFilter = df

	f.Debugf("monitored files: %v", f.Files.Include)
	f.Debugf("monitored dirs: %v", f.Dirs.Include)

	return nil
}

func (f *Filecheck) Check() error {
	return nil
}

func (f *Filecheck) Charts() *module.Charts {
	return f.charts
}

func (f *Filecheck) Collect() map[string]int64 {
	mx, err := f.collect()
	if err != nil {
		f.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (f *Filecheck) Cleanup() {}
