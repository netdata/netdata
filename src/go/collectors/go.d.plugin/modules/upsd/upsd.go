// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func init() {
	module.Register("upsd", module.Creator{
		Create: func() module.Module { return New() },
	})
}

func New() *Upsd {
	return &Upsd{
		Config: Config{
			Address: "127.0.0.1:3493",
			Timeout: web.Duration{Duration: time.Second * 2},
		},
		newUpsdConn: newUpsdConn,
		charts:      &module.Charts{},
		upsUnits:    make(map[string]bool),
	}
}

type Config struct {
	Address  string       `yaml:"address"`
	Username string       `yaml:"username"`
	Password string       `yaml:"password"`
	Timeout  web.Duration `yaml:"timeout"`
}

type (
	Upsd struct {
		module.Base

		Config `yaml:",inline"`

		charts *module.Charts

		newUpsdConn func(Config) upsdConn
		conn        upsdConn

		upsUnits map[string]bool
	}

	upsdConn interface {
		connect() error
		disconnect() error
		authenticate(string, string) error
		upsUnits() ([]upsUnit, error)
	}
)

func (u *Upsd) Init() bool {
	if u.Address == "" {
		u.Error("config: 'address' not set")
		return false
	}

	return true
}

func (u *Upsd) Check() bool {
	return len(u.Collect()) > 0
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
