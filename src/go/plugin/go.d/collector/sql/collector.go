// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("sql", module.Creator{
		Create:          func() module.Module { return New() },
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Driver:          "mysql",
			DSN:             "root:my-secret-pw@tcp(10.10.10.20:3306)/",
			Timeout:         confopt.Duration(time.Second * 5),
			ConnMaxLifetime: confopt.Duration(time.Minute * 10),
		},
		charts:     &module.Charts{},
		seenCharts: make(map[string]bool),
		skipValues: make(map[string]bool),
	}
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db *sql.DB

	seenCharts map[string]bool
	skipValues map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Init(context.Context) error {
	return c.validateConfig()
}

func (c *Collector) Check(ctx context.Context) error {
	mx, err := c.collect(ctx)
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx, err := c.collect(ctx)
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}
