// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

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
					Timeout: web.Duration(time.Second),
				},
			},
		},
		collectedBuckets: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
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
		cb.Errorf("init HTTP client: %v", err)
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
