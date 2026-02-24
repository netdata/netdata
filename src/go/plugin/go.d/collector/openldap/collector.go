// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("openldap", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			URL:     "ldap://127.0.0.1:389",
			Timeout: confopt.Duration(time.Second * 2),
		},

		newConn: newLdapConn,

		charts: charts.Copy(),
	}

}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	URL                string           `yaml:"url" json:"url"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Username           string           `yaml:"username" json:"username"`
	Password           string           `yaml:"password" json:"password"`
	tlscfg.TLSConfig   `yaml:",inline" json:""`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	conn    ldapConn
	newConn func(Config) ldapConn
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.URL == "" {
		return errors.New("empty LDAP server url")
	}
	if c.Username == "" {
		return errors.New("empty LDAP username")
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

func (c *Collector) Charts() *collectorapi.Charts {
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
			c.Warningf("error disconnecting ldap client: %v", err)
		}
		c.conn = nil
	}
}
