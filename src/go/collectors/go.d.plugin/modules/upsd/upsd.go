// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("upsd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Upsd {
	return &Upsd{
		Config: Config{
			Address: "127.0.0.1:3493",
			Timeout: web.Duration(time.Second * 2),
		},
		newUpsdConn: newUpsdConn,
		charts:      &module.Charts{},
		upsUnits:    make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Username    string       `yaml:"username" json:"username"`
	Password    string       `yaml:"password" json:"password"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
}

type (
	Upsd struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		conn        upsdConn
		newUpsdConn func(Config) upsdConn

		upsUnits map[string]bool
	}

	upsdConn interface {
		connect() error
		disconnect() error
		authenticate(string, string) error
		upsUnits() ([]upsUnit, error)
	}
)

func (u *Upsd) Configuration() any {
	return u.Config
}

func (u *Upsd) Init() error {
	if u.Address == "" {
		u.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (u *Upsd) Check() error {
	mx, err := u.collect()
	if err != nil {
		u.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (u *Upsd) Charts() *module.Charts {
	return u.charts
}

func (u *Upsd) Collect() map[string]int64 {
	mx, err := u.collect()
	if err != nil {
		u.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (u *Upsd) Cleanup() {
	if u.conn == nil {
		return
	}
	if err := u.conn.disconnect(); err != nil {
		u.Warningf("error on disconnect: %v", err)
	}
	u.conn = nil
}
