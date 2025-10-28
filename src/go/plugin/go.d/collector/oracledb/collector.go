// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("oracledb", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout:         confopt.Duration(time.Second * 2),
			charts:          globalCharts.Copy(),
			seenTablespaces: make(map[string]bool),
			seenWaitClasses: make(map[string]bool),
		},
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `json:"dsn" yaml:"dsn"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	charts *module.Charts

	publicDSN string // with hidden username/password

	seenTablespaces map[string]bool
	seenWaitClasses map[string]bool
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	db *sql.DB
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	dsn, err := c.validateDSN()
	if err != nil {
		return fmt.Errorf("invalid oracle DSN: %w", err)
	}

	c.publicDSN = dsn

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return fmt.Errorf("failed to collect metrics [%s]: %w", c.publicDSN, err)
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(fmt.Sprintf("failed to collect metrics [%s]: %s", c.publicDSN, err))
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.Errorf("cleanup: error on closing connection [%s]: %v", c.publicDSN, err)
		}
		c.db = nil
	}
}
