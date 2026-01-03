// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/blang/semver/v4"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pgbouncer", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second),
			DSN:     "postgres://postgres:postgres@127.0.0.1:6432/pgbouncer",
		},
		charts:               globalCharts.Copy(),
		recheckSettingsEvery: time.Minute * 5,
		metrics: &metrics{
			dbs: make(map[string]*dbMetrics),
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

	version              *semver.Version
	recheckSettingsTime  time.Time
	recheckSettingsEvery time.Duration
	maxClientConn        int64

	metrics *metrics
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

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
		c.Warningf("cleanup: error on closing the PgBouncer database [%s]: %v", c.DSN, err)
	}
	c.db = nil
}
