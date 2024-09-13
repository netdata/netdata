// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("apcupsd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Apcupsd {
	return &Apcupsd{
		Config: Config{
			Address: "127.0.0.1:3551",
			Timeout: confopt.Duration(time.Second * 3),
		},
		newConn: newUpsdConn,
		charts:  charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Apcupsd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	conn    apcupsdConn
	newConn func(Config) apcupsdConn
}

func (a *Apcupsd) Configuration() any {
	return a.Config
}

func (a *Apcupsd) Init() error {
	if a.Address == "" {
		a.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (a *Apcupsd) Check() error {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (a *Apcupsd) Charts() *module.Charts {
	return a.charts
}

func (a *Apcupsd) Collect() map[string]int64 {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (a *Apcupsd) Cleanup() {
	if a.conn != nil {
		if err := a.conn.disconnect(); err != nil {
			a.Warningf("error on disconnect: %v", err)
		}
		a.conn = nil
	}
}
