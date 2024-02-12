// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

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
	module.Register("powerdns_recursor", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Recursor {
	return &Recursor{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8081",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Recursor struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts
}

func (r *Recursor) Init() bool {
	err := r.validateConfig()
	if err != nil {
		r.Errorf("config validation: %v", err)
		return false
	}

	client, err := r.initHTTPClient()
	if err != nil {
		r.Errorf("init HTTP client: %v", err)
		return false
	}
	r.httpClient = client

	cs, err := r.initCharts()
	if err != nil {
		r.Errorf("init charts: %v", err)
		return false
	}
	r.charts = cs

	return true
}

func (r *Recursor) Check() bool {
	return len(r.Collect()) > 0
}

func (r *Recursor) Charts() *module.Charts {
	return r.charts
}

func (r *Recursor) Collect() map[string]int64 {
	ms, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (r *Recursor) Cleanup() {
	if r.httpClient == nil {
		return
	}
	r.httpClient.CloseIdleConnections()
}
