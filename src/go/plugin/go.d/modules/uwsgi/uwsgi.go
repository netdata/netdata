// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("uwsgi", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Uwsgi {
	return &Uwsgi{
		Config: Config{
			Address: "127.0.0.1:1717",
			Timeout: web.Duration(time.Second * 1),
		},
		newConn:     newUwsgiConn,
		charts:      charts.Copy(),
		seenWorkers: make(map[int]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
}

type Uwsgi struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config) uwsgiConn

	seenWorkers map[int]bool
}

func (u *Uwsgi) Configuration() any {
	return u.Config
}

func (u *Uwsgi) Init() error {
	if u.Address == "" {
		u.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (u *Uwsgi) Check() error {
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

func (u *Uwsgi) Charts() *module.Charts {
	return u.charts
}

func (u *Uwsgi) Collect() map[string]int64 {
	mx, err := u.collect()
	if err != nil {
		u.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (u *Uwsgi) Cleanup() {}
