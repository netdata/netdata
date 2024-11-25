// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package litespeed

import (
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("litespeed", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10, // The .rtreport files are generated per worker, and updated every 10 seconds.
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Litespeed {
	return &Litespeed{
		Config: Config{
			//ReportsDir: "/tmp/lshttpd/",
			ReportsDir: "/opt/litespeed",
		},
		checkDir: true,
		charts:   charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	ReportsDir  string `yaml:"reports_dir" json:"reports_dir"`
}

type Litespeed struct {
	module.Base
	Config `yaml:",inline" json:""`

	checkDir bool

	charts *module.Charts
}

func (l *Litespeed) Configuration() any {
	return l.Config
}

func (l *Litespeed) Init() error {
	if l.ReportsDir == "" {
		return errors.New("reports_dir is required")
	}
	return nil
}

func (l *Litespeed) Check() error {
	mx, err := l.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (l *Litespeed) Charts() *module.Charts {
	return l.charts
}

func (l *Litespeed) Collect() map[string]int64 {
	mx, err := l.collect()

	if err != nil {
		l.Error(err)
		return nil
	}

	return mx
}

func (l *Litespeed) Cleanup() {}
