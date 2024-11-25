// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("rethinkdb", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Rethinkdb {
	return &Rethinkdb{
		Config: Config{
			Address: "127.0.0.1:28015",
			Timeout: confopt.Duration(time.Second * 1),
		},

		charts:      clusterCharts.Copy(),
		newConn:     newRethinkdbConn,
		seenServers: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Username    string           `yaml:"username,omitempty" json:"username"`
	Password    string           `yaml:"password,omitempty" json:"password"`
}

type (
	Rethinkdb struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		newConn func(cfg Config) (rdbConn, error)
		rdb     rdbConn

		seenServers map[string]bool
	}
)

func (r *Rethinkdb) Configuration() any {
	return r.Config
}

func (r *Rethinkdb) Init() error {
	if r.Address == "" {
		return errors.New("config: address is not set")
	}
	return nil
}

func (r *Rethinkdb) Check() error {
	mx, err := r.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (r *Rethinkdb) Charts() *module.Charts {
	return r.charts
}

func (r *Rethinkdb) Collect() map[string]int64 {
	ms, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (r *Rethinkdb) Cleanup() {
	if r.rdb != nil {
		if err := r.rdb.close(); err != nil {
			r.Warningf("cleanup: error on closing client [%s]: %v", r.Address, err)
		}
		r.rdb = nil
	}
}
