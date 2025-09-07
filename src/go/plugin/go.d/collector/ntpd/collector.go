// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ntpd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address:      "127.0.0.1:123",
			Timeout:      confopt.Duration(time.Second),
			CollectPeers: false,
		},
		charts:         systemCharts.Copy(),
		newClient:      newNTPClient,
		findPeersEvery: time.Minute * 3,
		peerAddr:       make(map[string]bool),
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	CollectPeers       confopt.FlexBool `yaml:"collect_peers" json:"collect_peers"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	client    ntpConn
	newClient func(c Config) (ntpConn, error)

	findPeersTime    time.Time
	findPeersEvery   time.Duration
	peerAddr         map[string]bool
	peerIDs          []uint16
	peerIPAddrFilter *iprange.Pool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.Address == "" {
		return errors.New("config: 'address' can not be empty")
	}

	txt := "0.0.0.0 127.0.0.0/8"
	r, err := iprange.ParseRanges(txt)
	if err != nil {
		return fmt.Errorf("error on parsing ip range '%s': %v", txt, err)
	}

	c.peerIPAddrFilter = iprange.NewPool(r...)

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
	if c.client != nil {
		c.client.close()
		c.client = nil
	}
}
