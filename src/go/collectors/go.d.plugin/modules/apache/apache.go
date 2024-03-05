// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	_ "embed"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts: &module.Charts{},
		once:   &sync.Once{},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Apache struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	once *sync.Once
}

func (a *Apache) Configuration() any {
	return a.Config
}

func (a *Apache) Init() error {
	if err := a.validateConfig(); err != nil {
		a.Errorf("config validation: %v", err)
		return err
	}

	httpClient, err := a.initHTTPClient()
	if err != nil {
		a.Errorf("init HTTP client: %v", err)
		return err
	}
	a.httpClient = httpClient

	a.Debugf("using URL %s", a.URL)
	a.Debugf("using timeout: %s", a.Timeout)

	return nil
}

func (a *Apache) Check() error {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
