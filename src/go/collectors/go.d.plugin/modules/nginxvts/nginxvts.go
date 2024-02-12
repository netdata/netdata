// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

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
	module.Register("nginxvts", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *NginxVTS {
	return &NginxVTS{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://localhost/status/format/json",
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

type NginxVTS struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts
}

func (vts *NginxVTS) Cleanup() {
	if vts.httpClient == nil {
		return
	}
	vts.httpClient.CloseIdleConnections()
}

func (vts *NginxVTS) Init() bool {
	err := vts.validateConfig()
	if err != nil {
		vts.Errorf("check configuration: %v", err)
		return false
	}

	httpClient, err := vts.initHTTPClient()
	if err != nil {
		vts.Errorf("init HTTP client: %v", err)
	}
	vts.httpClient = httpClient

	charts, err := vts.initCharts()
	if err != nil {
		vts.Errorf("init charts: %v", err)
		return false
	}
	vts.charts = charts

	return true
}

func (vts *NginxVTS) Check() bool {
	return len(vts.Collect()) > 0
}

func (vts *NginxVTS) Charts() *module.Charts {
	return vts.charts
}

func (vts *NginxVTS) Collect() map[string]int64 {
	mx, err := vts.collect()
	if err != nil {
		vts.Error(err)
		return nil
	}
	if len(mx) == 0 {
		return nil
	}
	return mx
}
