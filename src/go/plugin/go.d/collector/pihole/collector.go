// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pihole", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},
			SetupVarsPath: "/etc/pihole/setupVars.conf",
		},
		checkVersion:           true,
		charts:                 baseCharts.Copy(),
		addQueriesTypesOnce:    &sync.Once{},
		addFwsDestinationsOnce: &sync.Once{},
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	SetupVarsPath  string `yaml:"setup_vars_path" json:"setup_vars_path"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                 *module.Charts
	addQueriesTypesOnce    *sync.Once
	addFwsDestinationsOnce *sync.Once

	httpClient *http.Client

	checkVersion bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	httpClient, err := c.initHTTPClient()
	if err != nil {
		return fmt.Errorf("init http client: %v", err)
	}
	c.httpClient = httpClient

	c.Password = c.getWebPassword()
	if c.Password == "" {
		c.Warning("no web password, not all metrics available")
	} else {
		c.Debugf("web password: %s", c.Password)
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
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
