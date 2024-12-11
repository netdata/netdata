// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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

func New() *Collector {
	return &Collector{
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

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	ff, err := c.initFilesFilter()
	if err != nil {
		return fmt.Errorf("files filter initialization: %v", err)
	}
	c.filesFilter = ff

	df, err := c.initDirsFilter()
	if err != nil {
		return fmt.Errorf("dirs filter initialization: %v", err)
	}
	c.dirsFilter = df

	c.Debugf("monitored files: %v", c.Files.Include)
	c.Debugf("monitored dirs: %v", c.Dirs.Include)

	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {}
