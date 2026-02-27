// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("sql", collectorapi.Creator{
		Create:          func() collectorapi.CollectorV1 { return New() },
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
		JobMethods:      sqlJobMethods,
		MethodHandler:   sqlMethodHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Driver:  "mysql",
			Timeout: confopt.Duration(time.Second * 5),
		},
		charts:     &collectorapi.Charts{},
		seenCharts: make(map[string]bool),
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	dbMu     sync.RWMutex
	db       *sql.DB
	dbCtx    context.Context
	dbCancel context.CancelFunc

	seenCharts map[string]bool

	funcTable *funcTable
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Charts() *collectorapi.Charts {
	if c.Config.FunctionOnly {
		return nil
	}
	return c.charts
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}

	c.funcTable = newFuncTable(c)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.db == nil {
		if err := c.openConnection(ctx); err != nil {
			return err
		}
		// Create cancellable context for function queries
		c.dbCtx, c.dbCancel = context.WithCancel(context.Background())
	}

	if c.Config.FunctionOnly {
		return nil
	}

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
	if c.Config.FunctionOnly {
		return nil
	}

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
	// Cancel context first to signal in-flight queries to abort
	if c.dbCancel != nil {
		c.dbCancel()
	}

	// Acquire write lock - should be quick since queries are aborting
	c.dbMu.Lock()
	defer c.dbMu.Unlock()

	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}
