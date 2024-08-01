// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

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
	module.Register("beanstalk", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Beanstalk {
	return &Beanstalk{
		Config: Config{
			Address: "127.0.0.1:11300",
			Timeout: web.Duration(time.Second * 10),
		},

		charts:           &module.Charts{},
		tubes:            make(map[string]bool),
		newBeanstalkConn: newBeanstalkConn,
		once:             false,
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
}

type (
	Beanstalk struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		tubes map[string]bool

		newBeanstalkConn func(Config) beanstalkConn
		conn             beanstalkConn
		once             bool
	}

	beanstalkConn interface {
		connect() error
		disconnect()
		queryStats() ([]byte, error)
		listTubes() ([]byte, error)
		statsTube(string) ([]byte, error)
	}
)

func (b *Beanstalk) Configuration() any {
	return b.Config
}

func (b *Beanstalk) Init() error {
	if b.Address == "" {
		b.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (b *Beanstalk) Check() error {
	mx, err := b.collect()
	if err != nil {
		b.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (b *Beanstalk) Charts() *module.Charts {
	return b.charts
}

func (b *Beanstalk) Collect() map[string]int64 {
	mx, err := b.collect()
	if err != nil {
		b.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (b *Beanstalk) Cleanup() {
	if b.conn != nil {
		b.conn.disconnect()
		b.conn = nil
	}
}
