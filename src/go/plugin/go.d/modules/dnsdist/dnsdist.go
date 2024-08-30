// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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
		Config: func() any { return &Config{} },
	})
}

func New() *DNSdist {
	return &DNSdist{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8083",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type DNSdist struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (d *DNSdist) Configuration() any {
	return d.Config
}

func (d *DNSdist) Init() error {
	err := d.validateConfig()
	if err != nil {
		d.Errorf("config validation: %v", err)
		return err
	}

	client, err := d.initHTTPClient()
	if err != nil {
		d.Errorf("init HTTP client: %v", err)
		return err
	}
	d.httpClient = client

	cs, err := d.initCharts()
	if err != nil {
		d.Errorf("init charts: %v", err)
		return err
	}
	d.charts = cs

	return nil
}

func (d *DNSdist) Check() error {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
