// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
					Timeout: web.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Recursor struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (r *Recursor) Configuration() any {
	return r.Config
}

func (r *Recursor) Init() error {
	err := r.validateConfig()
	if err != nil {
		r.Errorf("config validation: %v", err)
		return err
	}

	client, err := r.initHTTPClient()
	if err != nil {
		r.Errorf("init HTTP client: %v", err)
		return err
	}
	r.httpClient = client

	cs, err := r.initCharts()
	if err != nil {
		r.Errorf("init charts: %v", err)
		return err
	}
	r.charts = cs

	return nil
}

func (r *Recursor) Check() error {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
