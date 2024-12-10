// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("fail2ban", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:        &module.Charts{},
		discoverEvery: time.Minute * 5,
		seenJails:     make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec fail2banClientCli

	discoverEvery    time.Duration
	lastDiscoverTime time.Time
	forceDiscover    bool
	jails            []string

	seenJails map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	f2bClientExec, err := c.initFail2banClientCliExec()
	if err != nil {
		return fmt.Errorf("fail2ban-client exec initialization: %v", err)
	}
	c.exec = f2bClientExec

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

func (c *Collector) Cleanup(context.Context) {}
