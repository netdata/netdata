// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("mysql", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:     "root@tcp(localhost:3306)/",
			Timeout: confopt.Duration(time.Second),
		},

		charts:                         baseCharts.Copy(),
		addInnoDBOSLogOnce:             &sync.Once{},
		addBinlogOnce:                  &sync.Once{},
		addMyISAMOnce:                  &sync.Once{},
		addInnodbDeadlocksOnce:         &sync.Once{},
		addGaleraOnce:                  &sync.Once{},
		addQCacheOnce:                  &sync.Once{},
		addTableOpenCacheOverflowsOnce: &sync.Once{},
		doDisableSessionQueryLog:       true,
		doSlaveStatus:                  true,
		doUserStatistics:               true,
		collectedReplConns:             make(map[string]bool),
		collectedUsers:                 make(map[string]bool),

		recheckGlobalVarsEvery: time.Minute * 10,
	}
}

type Config struct {
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string           `yaml:"dsn" json:"dsn"`
	MyCNF       string           `yaml:"my.cnf,omitempty" json:"my.cnf"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                         *module.Charts
	addInnoDBOSLogOnce             *sync.Once
	addBinlogOnce                  *sync.Once
	addMyISAMOnce                  *sync.Once
	addInnodbDeadlocksOnce         *sync.Once
	addGaleraOnce                  *sync.Once
	addQCacheOnce                  *sync.Once
	addTableOpenCacheOverflowsOnce *sync.Once

	db *sql.DB

	safeDSN   string
	version   *semver.Version
	isMariaDB bool
	isPercona bool

	doDisableSessionQueryLog bool

	doSlaveStatus      bool
	collectedReplConns map[string]bool
	doUserStatistics   bool
	collectedUsers     map[string]bool

	recheckGlobalVarsTime    time.Time
	recheckGlobalVarsEvery   time.Duration
	varMaxConns              int64
	varTableOpenCache        int64
	varDisabledStorageEngine string
	varLogBin                string
	varPerformanceSchema     string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.MyCNF != "" {
		dsn, err := dsnFromFile(c.MyCNF)
		if err != nil {
			return err
		}
		c.DSN = dsn
	}

	if c.DSN == "" {
		return errors.New("config: dsn not set")
	}

	cfg, err := mysql.ParseDSN(c.DSN)
	if err != nil {
		return fmt.Errorf("error on parsing DSN: %v", err)
	}

	cfg.Passwd = strings.Repeat("x", len(cfg.Passwd))
	c.safeDSN = cfg.FormatDSN()

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
		c.Errorf("cleanup: error on closing the mysql database [%s]: %v", c.safeDSN, err)
	}
	c.db = nil
}
