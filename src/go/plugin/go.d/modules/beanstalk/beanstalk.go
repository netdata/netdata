// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
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
			Address:      "127.0.0.1:11300",
			Timeout:      web.Duration(time.Second * 5),
			TubeSelector: "*",
		},

		charts:             &module.Charts{},
		tubeSr:             matcher.TRUE(),
		discoverTubesEvery: time.Minute * 1,
		seenTubes:          make(map[string]bool),
		newBeanstalkConn:   newBeanstalkConn,
	}
}

type Config struct {
	UpdateEvery  int          `yaml:"update_every,omitempty" json:"update_every"`
	Address      string       `yaml:"address" json:"address"`
	Timeout      web.Duration `yaml:"timeout" json:"timeout"`
	TubeSelector string       `yaml:"tube_selector" json:"tube_selector"`
}

type (
	Beanstalk struct {
		module.Base
		Config `yaml:",inline" json:""`

		newBeanstalkConn func(Config) beanstalkConn
		conn             beanstalkConn

		charts *module.Charts

		discoverTubesEvery    time.Duration
		lastDiscoverTubesTime time.Time
		discoveredTubes       []string

		tubeSr matcher.Matcher

		seenTubes map[string]bool
	}

	beanstalkConn interface {
		connect() error
		disconnect() error
		queryStats() (*beanstalkdStats, error)
		queryListTubes() ([]string, error)
		queryStatsTube(string) (*tubeStats, error)
	}
)

func (b *Beanstalk) Configuration() any {
	return b.Config
}

func (b *Beanstalk) Init() error {
	if err := b.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sr, err := b.initTubeSelector()
	if err != nil {
		return fmt.Errorf("failed to init tube selector: %v", err)
	}
	b.tubeSr = sr

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
		if err := b.conn.disconnect(); err != nil {
			b.Warningf("error on disconnect: %s", err)
		}
		b.conn = nil
	}
}
