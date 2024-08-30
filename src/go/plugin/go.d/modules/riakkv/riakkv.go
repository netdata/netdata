// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	_ "embed"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("riakkv", module.Creator{
		Create: func() module.Module { return New() },
		// Riak updates the metrics on the /stats endpoint every 1 second.
		// If we use 1 here, it means we might get weird jitter in the graph,
		// so the default is set to 2 seconds to prevent that.
		Defaults: module.Defaults{
			UpdateEvery: 2,
		},
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
	})
}

func New() *RiakKv {
	return &RiakKv{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					// https://docs.riak.com/riak/kv/2.2.3/developing/api/http/status.1.html
					URL: "http://127.0.0.1:8098/stats",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		once:   &sync.Once{},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type RiakKv struct {
	module.Base
	Config `yaml:",inline" json:""`

	once   *sync.Once
	charts *module.Charts

	httpClient *http.Client
}

func (r *RiakKv) Configuration() any {
	return r.Config
}

func (r *RiakKv) Init() error {
	if r.URL == "" {
		r.Errorf("url required but not set")
		return errors.New("url not set")
	}

	httpClient, err := web.NewHTTPClient(r.Client)
	if err != nil {
		r.Errorf("init HTTP client: %v", err)
		return err
	}
	r.httpClient = httpClient

	r.Debugf("using URL %s", r.URL)
	r.Debugf("using timeout: %s", r.Timeout)

	return nil
}

func (r *RiakKv) Check() error {
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

func (r *RiakKv) Charts() *module.Charts {
	return r.charts
}

func (r *RiakKv) Collect() map[string]int64 {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (r *RiakKv) Cleanup() {
	if r.httpClient != nil {
		r.httpClient.CloseIdleConnections()
	}
}
