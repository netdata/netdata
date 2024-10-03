// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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

func New() *OracleDB {
	return &OracleDB{
		Config: Config{
			Timeout:         confopt.Duration(time.Second * 2),
			charts:          globalCharts.Copy(),
			seenTablespaces: make(map[string]bool),
			seenWaitClasses: make(map[string]bool),
		},
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string           `json:"dsn" yaml:"dsn"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	charts *module.Charts

	publicDSN string // with hidden username/password

	seenTablespaces map[string]bool
	seenWaitClasses map[string]bool
}

type OracleDB struct {
	module.Base
	Config `yaml:",inline" json:""`

	db *sql.DB
}

func (o *OracleDB) Configuration() any {
	return o.Config
}

func (o *OracleDB) Init() error {
	dsn, err := o.validateDSN()
	if err != nil {
		return fmt.Errorf("invalid oracle DSN: %w", err)
	}

	o.publicDSN = dsn

	return nil
}

func (o *OracleDB) Check() error {
	mx, err := o.collect()
	if err != nil {
		return fmt.Errorf("failed to collect metrics [%s]: %w", o.publicDSN, err)
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (o *OracleDB) Charts() *module.Charts {
	return o.charts
}

func (o *OracleDB) Collect() map[string]int64 {
	mx, err := o.collect()
	if err != nil {
		o.Error(fmt.Sprintf("failed to collect metrics [%s]: %s", o.publicDSN, err))
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (o *OracleDB) Cleanup() {
	if o.db != nil {
		if err := o.db.Close(); err != nil {
			o.Errorf("cleanup: error on closing connection [%s]: %v", o.publicDSN, err)
		}
		o.db = nil
	}
}
