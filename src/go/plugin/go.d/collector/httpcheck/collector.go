// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("httpcheck", module.Creator{
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
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			AcceptedStatuses: []int{200},
		},

		acceptedStatuses: make(map[int]bool),
	}
}

type (
	Config struct {
		Vnode            string `yaml:"vnode,omitempty" json:"vnode"`
		UpdateEvery      int    `yaml:"update_every,omitempty" json:"update_every"`
		web.HTTPConfig   `yaml:",inline" json:""`
		AcceptedStatuses []int               `yaml:"status_accepted" json:"status_accepted"`
		ResponseMatch    string              `yaml:"response_match,omitempty" json:"response_match"`
		CookieFile       string              `yaml:"cookie_file,omitempty" json:"cookie_file"`
		HeaderMatch      []headerMatchConfig `yaml:"header_match,omitempty" json:"header_match"`
	}
	headerMatchConfig struct {
		Exclude bool   `yaml:"exclude" json:"exclude"`
		Key     string `yaml:"key" json:"key"`
		Value   string `yaml:"value" json:"value"`
	}
)

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	acceptedStatuses  map[int]bool
	reResponse        *regexp.Regexp
	headerMatch       []headerMatch
	cookieFileModTime time.Time

	metrics metrics
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	c.charts = c.initCharts()

	httpClient, err := c.initHTTPClient()
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	c.httpClient = httpClient

	re, err := c.initResponseMatchRegexp()
	if err != nil {
		return fmt.Errorf("init response match regexp: %v", err)
	}
	c.reResponse = re

	hm, err := c.initHeaderMatch()
	if err != nil {
		return fmt.Errorf("init header match: %v", err)
	}
	c.headerMatch = hm

	for _, v := range c.AcceptedStatuses {
		c.acceptedStatuses[v] = true
	}

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using HTTP timeout %s", c.Timeout.Duration())
	c.Debugf("using accepted HTTPConfig statuses %v", c.AcceptedStatuses)
	if c.reResponse != nil {
		c.Debugf("using response match regexp %s", c.reResponse)
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
