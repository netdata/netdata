// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

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
	module.Register("dnsdist", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
	})
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type DNSdist struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts
}

func New() *DNSdist {
	return &DNSdist{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8083",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},
	}
}

func (d *DNSdist) Init() bool {
	err := d.validateConfig()
	if err != nil {
		d.Errorf("config validation: %v", err)
		return false
	}

	client, err := d.initHTTPClient()
	if err != nil {
		d.Errorf("init HTTP client: %v", err)
		return false
	}
	d.httpClient = client

	cs, err := d.initCharts()
	if err != nil {
		d.Errorf("init charts: %v", err)
		return false
	}
	d.charts = cs

	return true
}

func (d *DNSdist) Check() bool {
	return len(d.Collect()) > 0
}

func (d *DNSdist) Charts() *module.Charts {
	return d.charts
}

func (d *DNSdist) Collect() map[string]int64 {
	ms, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}

	return ms
}

func (d *DNSdist) Cleanup() {
	if d.httpClient == nil {
		return
	}

	d.httpClient.CloseIdleConnections()
}
