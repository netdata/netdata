// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("unbound", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address:    "127.0.0.1:8953",
			ConfPath:   "/etc/unbound/unbound.conf",
			Timeout:    confopt.Duration(time.Second),
			Cumulative: false,
			UseTLS:     true,
			TLSConfig: tlscfg.TLSConfig{
				TLSCert:            "/etc/unbound/unbound_control.pem",
				TLSKey:             "/etc/unbound/unbound_control.key",
				InsecureSkipVerify: true,
			},
		},
		curCache: newCollectCache(),
		cache:    newCollectCache(),
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	ConfPath           string           `yaml:"conf_path,omitempty" json:"conf_path"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Cumulative         confopt.FlexBool `yaml:"cumulative_stats" json:"cumulative_stats"`
	UseTLS             confopt.FlexBool `yaml:"use_tls,omitempty" json:"use_tls"`
	tlscfg.TLSConfig   `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	client socket.Client

	cache            collectCache
	curCache         collectCache
	prevCacheMiss    float64 // needed for cumulative mode
	extChartsCreated bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if enabled := c.initConfig(); !enabled {
		return errors.New("remote control is disabled in the configuration file")
	}

	if err := c.initClient(); err != nil {
		return fmt.Errorf("creating client: %v", err)
	}

	c.charts = charts(c.Cumulative.Bool())

	c.Debugf("using address: %s, cumulative: %v, use_tls: %v, timeout: %s", c.Address, c.Cumulative, c.UseTLS, c.Timeout)
	if c.UseTLS {
		c.Debugf("using tls_skip_verify: %v, tls_key: %s, tls_cert: %s", c.InsecureSkipVerify, c.TLSKey, c.TLSCert)
	}

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
		_ = c.client.Disconnect()
	}
}
