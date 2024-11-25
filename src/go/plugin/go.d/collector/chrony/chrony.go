// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("chrony", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Chrony {
	return &Chrony{
		Config: Config{
			Address: "127.0.0.1:323",
			Timeout: confopt.Duration(time.Second),
		},
		charts:                   charts.Copy(),
		addServerStatsChartsOnce: &sync.Once{},
		newConn:                  newChronyConn,
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Chrony struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                   *module.Charts
	addServerStatsChartsOnce *sync.Once

	exec chronyBinary

	conn    chronyConn
	newConn func(c Config) (chronyConn, error)
}

func (c *Chrony) Configuration() any {
	return c.Config
}

func (c *Chrony) Init() error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	var err error
	if c.exec, err = c.initChronycBinary(); err != nil {
		c.Warningf("chronyc binary init failed: %v (serverstats metrics collection is disabled)", err)
	}

	return nil
}

func (c *Chrony) Check() error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (c *Chrony) Charts() *module.Charts {
	return c.charts
}

func (c *Chrony) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Chrony) Cleanup() {
	if c.conn != nil {
		c.conn.close()
		c.conn = nil
	}
}
