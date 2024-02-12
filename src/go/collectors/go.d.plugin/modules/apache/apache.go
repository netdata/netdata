// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	_ "embed"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("apache", module.Creator{
		Create:          func() module.Module { return New() },
		JobConfigSchema: configSchema,
	})
}

func New() *Apache {
	return &Apache{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1/server-status?auto",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 2},
				},
			},
		},
		charts: &module.Charts{},
		once:   &sync.Once{},
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Apache struct {
	module.Base

	Config `yaml:",inline"`

	charts *module.Charts

	httpClient *http.Client
	once       *sync.Once
}

func (a *Apache) Init() bool {
	if err := a.verifyConfig(); err != nil {
		a.Errorf("config validation: %v", err)
		return false
	}

	httpClient, err := a.initHTTPClient()
	if err != nil {
		a.Errorf("init HTTP client: %v", err)
		return false
	}
	a.httpClient = httpClient

	a.Debugf("using URL %s", a.URL)
	a.Debugf("using timeout: %s", a.Timeout.Duration)
	return true
}

func (a *Apache) Check() bool {
	return len(a.Collect()) > 0
}

func (a *Apache) Charts() *module.Charts {
	return a.charts
}

func (a *Apache) Collect() map[string]int64 {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (a *Apache) Cleanup() {
	if a.httpClient != nil {
		a.httpClient.CloseIdleConnections()
	}
}
