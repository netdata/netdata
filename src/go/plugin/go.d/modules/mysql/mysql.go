// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"database/sql"
	_ "embed"
	"errors"
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

func New() *MySQL {
	return &MySQL{
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
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string           `yaml:"dsn" json:"dsn"`
	MyCNF       string           `yaml:"my.cnf,omitempty" json:"my.cnf"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type MySQL struct {
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

func (m *MySQL) Configuration() any {
	return m.Config
}

func (m *MySQL) Init() error {
	if m.MyCNF != "" {
		dsn, err := dsnFromFile(m.MyCNF)
		if err != nil {
			m.Error(err)
			return err
		}
		m.DSN = dsn
	}

	if m.DSN == "" {
		m.Error("dsn not set")
		return errors.New("dsn not set")
	}

	cfg, err := mysql.ParseDSN(m.DSN)
	if err != nil {
		m.Errorf("error on parsing DSN: %v", err)
		return err
	}

	cfg.Passwd = strings.Repeat("*", len(cfg.Passwd))
	m.safeDSN = cfg.FormatDSN()

	m.Debugf("using DSN [%s]", m.DSN)

	return nil
}

func (m *MySQL) Check() error {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (m *MySQL) Charts() *module.Charts {
	return m.charts
}

func (m *MySQL) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (m *MySQL) Cleanup() {
	if m.db == nil {
		return
	}
	if err := m.db.Close(); err != nil {
		m.Errorf("cleanup: error on closing the mysql database [%s]: %v", m.safeDSN, err)
	}
	m.db = nil
}
