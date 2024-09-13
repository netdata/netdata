// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("couchbase", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Couchbase {
	return &Couchbase{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8091",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		collectedBuckets: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Couchbase struct {
	module.Base
	Config `yaml:",inline" json:""`

	httpClient *http.Client
	charts     *module.Charts

	collectedBuckets map[string]bool
}

func (cb *Couchbase) Configuration() any {
	return cb.Config
}

func (cb *Couchbase) Init() error {
	err := cb.validateConfig()
	if err != nil {
		cb.Errorf("check configuration: %v", err)
		return err
	}

	httpClient, err := cb.initHTTPClient()
	if err != nil {
		cb.Errorf("init HTTPConfig client: %v", err)
		return err
	}
	cb.httpClient = httpClient

	charts, err := cb.initCharts()
	if err != nil {
		cb.Errorf("init charts: %v", err)
		return err
	}
	cb.charts = charts

	return nil
}

func (cb *Couchbase) Check() error {
	mx, err := cb.collect()
	if err != nil {
		cb.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (cb *Couchbase) Charts() *Charts {
	return cb.charts
}

func (cb *Couchbase) Collect() map[string]int64 {
	mx, err := cb.collect()
	if err != nil {
		cb.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (cb *Couchbase) Cleanup() {
	if cb.httpClient == nil {
		return
	}
	cb.httpClient.CloseIdleConnections()
}
