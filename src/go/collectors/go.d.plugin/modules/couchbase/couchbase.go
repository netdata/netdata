// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
	})
}

func New() *Couchbase {
	return &Couchbase{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8091",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
		},
		collectedBuckets: make(map[string]bool),
	}
}

type (
	Config struct {
		web.HTTP `yaml:",inline"`
	}
	Couchbase struct {
		module.Base
		Config `yaml:",inline"`

		httpClient       *http.Client
		charts           *module.Charts
		collectedBuckets map[string]bool
	}
)

func (cb *Couchbase) Cleanup() {
	if cb.httpClient == nil {
		return
	}
	cb.httpClient.CloseIdleConnections()
}

func (cb *Couchbase) Init() bool {
	err := cb.validateConfig()
	if err != nil {
		cb.Errorf("check configuration: %v", err)
		return false
	}

	httpClient, err := cb.initHTTPClient()
	if err != nil {
		cb.Errorf("init HTTP client: %v", err)
		return false
	}
	cb.httpClient = httpClient

	charts, err := cb.initCharts()
	if err != nil {
		cb.Errorf("init charts: %v", err)
		return false
	}

	cb.charts = charts
	return true
}

func (cb *Couchbase) Check() bool {
	return len(cb.Collect()) > 0
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
