// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("energid", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Energid struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts
}

func New() *Energid {
	return &Energid{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:9796",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},
	}
}

func (e *Energid) Init() bool {
	err := e.validateConfig()
	if err != nil {
		e.Errorf("config validation: %v", err)
		return false
	}

	client, err := e.initHTTPClient()
	if err != nil {
		e.Errorf("init HTTP client: %v", err)
		return false
	}
	e.httpClient = client

	cs, err := e.initCharts()
	if err != nil {
		e.Errorf("init charts: %v", err)
		return false
	}
	e.charts = cs

	return true
}

func (e *Energid) Check() bool {
	return len(e.Collect()) > 0
}

func (e *Energid) Charts() *module.Charts {
	return e.charts
}

func (e *Energid) Collect() map[string]int64 {
	ms, err := e.collect()
	if err != nil {
		e.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}

	return ms
}

func (e *Energid) Cleanup() {
	if e.httpClient == nil {
		return
	}
	e.httpClient.CloseIdleConnections()
}
