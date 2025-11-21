// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	cfssllog "github.com/cloudflare/cfssl/log"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	cfssllog.Level = cfssllog.LevelFatal
	module.Register("x509check", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 60,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout:        confopt.Duration(time.Second * 2),
			CheckFullChain: false,
		},

		charts:    &module.Charts{},
		seenCerts: make(map[string]bool),
	}
}

type Config struct {
	Vnode            string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery      int              `yaml:"update_every,omitempty" json:"update_every"`
	Source           string           `yaml:"source" json:"source"`
	Timeout          confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	CheckFullChain   bool             `yaml:"check_full_chain" json:"check_full_chain"`
	CheckRevocation  bool             `yaml:"check_revocation_status" json:"check_revocation_status"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prov provider

	seenCerts map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prov, err := c.initProvider()
	if err != nil {
		return fmt.Errorf("certificate provider init: %v", err)
	}
	c.prov = prov

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
