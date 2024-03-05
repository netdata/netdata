// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

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
	module.Register("rabbitmq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *RabbitMQ {
	return &RabbitMQ{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL:      "http://localhost:15672",
					Username: "guest",
					Password: "guest",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
			CollectQueues: false,
		},
		charts: baseCharts.Copy(),
		vhosts: make(map[string]bool),
		queues: make(map[string]queueCache),
	}
}

type Config struct {
	web.HTTP      `yaml:",inline" json:""`
	UpdateEvery   int  `yaml:"update_every" json:"update_every"`
	CollectQueues bool `yaml:"collect_queues_metrics" json:"collect_queues_metrics"`
}

type (
	RabbitMQ struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		httpClient *http.Client

		nodeName string
		vhosts   map[string]bool
		queues   map[string]queueCache
	}
	queueCache struct {
		name, vhost string
	}
)

func (r *RabbitMQ) Configuration() any {
	return r.Config
}

func (r *RabbitMQ) Init() error {
	if r.URL == "" {
		r.Error("'url' can not be empty")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(r.Client)
	if err != nil {
		r.Errorf("init HTTP client: %v", err)
		return err
	}
	r.httpClient = client

	r.Debugf("using URL %s", r.URL)
	r.Debugf("using timeout: %s", r.Timeout)

	return nil
}

func (r *RabbitMQ) Check() error {
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
