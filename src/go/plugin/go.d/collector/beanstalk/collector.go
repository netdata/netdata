// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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

func New() *Collector {
	return &Collector{
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
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	TubeSelector       string           `yaml:"tube_selector,omitempty" json:"tube_selector"`
}

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sr, err := c.initTubeSelector()
	if err != nil {
		return fmt.Errorf("failed to init tube selector: %v", err)
	}
	c.tubeSr = sr

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.conn != nil {
		if err := c.conn.disconnect(); err != nil {
			c.Warningf("error on disconnect: %s", err)
		}
		c.conn = nil
	}
}
