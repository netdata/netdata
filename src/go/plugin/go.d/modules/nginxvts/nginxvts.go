// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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
		Config: func() any { return &Config{} },
	})
}

func New() *NginxVTS {
	return &NginxVTS{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://localhost/status/format/json",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type NginxVTS struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (vts *NginxVTS) Configuration() any {
	return vts.Config
}

func (vts *NginxVTS) Cleanup() {
	if vts.httpClient == nil {
		return
	}
	vts.httpClient.CloseIdleConnections()
}

func (vts *NginxVTS) Init() error {
	err := vts.validateConfig()
	if err != nil {
		vts.Errorf("check configuration: %v", err)
		return err
	}

	httpClient, err := vts.initHTTPClient()
	if err != nil {
		vts.Errorf("init HTTPConfig client: %v", err)
	}
	vts.httpClient = httpClient

	charts, err := vts.initCharts()
	if err != nil {
		vts.Errorf("init charts: %v", err)
		return err
	}
	vts.charts = charts

	return nil
}

func (vts *NginxVTS) Check() error {
	mx, err := vts.collect()
	if err != nil {
		vts.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
