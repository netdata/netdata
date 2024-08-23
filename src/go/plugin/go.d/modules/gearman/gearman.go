// SPDX-License-Identifier: GPL-3.0-or-later

package gearman

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("gearman", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Gearman {
	return &Gearman{
		Config: Config{
			Address: "127.0.0.1:4730",
			Timeout: web.Duration(time.Second * 1),
		},
		newConn:           newGearmanConn,
		charts:            summaryCharts.Copy(),
		seenTasks:         make(map[string]bool),
		seenPriorityTasks: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
}

type Gearman struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config) gearmanConn
	conn    gearmanConn

	seenTasks         map[string]bool
	seenPriorityTasks map[string]bool
}

func (g *Gearman) Configuration() any {
	return g.Config
}

func (g *Gearman) Init() error {
	if g.Address == "" {
		g.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (g *Gearman) Check() error {
	mx, err := g.collect()
	if err != nil {
		g.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (g *Gearman) Charts() *module.Charts {
	return g.charts
}

func (g *Gearman) Collect() map[string]int64 {
	mx, err := g.collect()
	if err != nil {
		g.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (g *Gearman) Cleanup() {
	if g.conn != nil {
		g.conn.disconnect()
		g.conn = nil
	}
}
