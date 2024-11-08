// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("rabbitmq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *RabbitMQ {
	return &RabbitMQ{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL:      "http://localhost:15672",
					Username: "guest",
					Password: "guest",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			CollectQueues: false,
		},

		charts: &module.Charts{},
		cache:  newCache(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	CollectQueues  bool `yaml:"collect_queues_metrics" json:"collect_queues_metrics"`
}

type (
	RabbitMQ struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		httpClient *http.Client

		clusterName string
		clusterId   string
		cache       *cache
	}
)

func (r *RabbitMQ) Configuration() any {
	return r.Config
}

func (r *RabbitMQ) Init() error {
	if r.URL == "" {
		return errors.New("config: url not set")
	}

	client, err := web.NewHTTPClient(r.ClientConfig)
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	r.httpClient = client

	r.Debugf("using URL %s", r.URL)
	r.Debugf("using timeout: %s", r.Timeout)

	return nil
}

func (r *RabbitMQ) Check() error {
	mx, err := r.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (r *RabbitMQ) Charts() *module.Charts {
	return r.charts
}

func (r *RabbitMQ) Collect() map[string]int64 {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (r *RabbitMQ) Cleanup() {
	if r.httpClient != nil {
		r.httpClient.CloseIdleConnections()
	}
}
