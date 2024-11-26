// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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
			Timeout:      confopt.Duration(time.Second * 1),
			TubeSelector: "*",
		},

		charts:             statsCharts.Copy(),
		newConn:            newBeanstalkConn,
		discoverTubesEvery: time.Minute * 1,
		tubeSr:             matcher.TRUE(),
		seenTubes:          make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery  int              `yaml:"update_every,omitempty" json:"update_every"`
	Address      string           `yaml:"address" json:"address"`
	Timeout      confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	TubeSelector string           `yaml:"tube_selector,omitempty" json:"tube_selector"`
}

type Beanstalk struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config, *logger.Logger) beanstalkConn
	conn    beanstalkConn

	discoverTubesEvery    time.Duration
	lastDiscoverTubesTime time.Time
	discoveredTubes       []string
	tubeSr                matcher.Matcher
	seenTubes             map[string]bool
}

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
