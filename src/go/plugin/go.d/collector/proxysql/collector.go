// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("proxysql", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:     "stats:stats@tcp(127.0.0.1:6032)/",
			Timeout: confopt.Duration(time.Second),
		},

		charts: baseCharts.Copy(),
		once:   &sync.Once{},
		cache: &cache{
			commands:   make(map[string]*commandCache),
			users:      make(map[string]*userCache),
			backends:   make(map[string]*backendCache),
			hostgroups: make(map[string]*hostgroupCache),
		},
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	DSN                string           `yaml:"dsn" json:"dsn"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db *sql.DB

	once  *sync.Once
	cache *cache
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.DSN == "" {
		return errors.New("dsn not set")
	}

	c.Debugf("using DSN [%s]", c.DSN)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
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
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		c.Errorf("cleanup: error on closing the ProxySQL instance [%s]: %v", c.DSN, err)
	}
	c.db = nil
}
